package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"

	"erat.org/nup"
)

const (
	metadataUpdate = 1
	ratingUpdate   = 2
	tagsUpdate     = 4
	playUpdate     = 8
)

func sortedStringSlicesMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func copySongFileFields(dest, src *nup.Song) {
	dest.Sha1 = src.Sha1
	dest.Filename = src.Filename
	dest.CoverFilename = src.CoverFilename
	dest.Artist = src.Artist
	dest.Title = src.Title
	dest.Album = src.Album
	dest.AlbumId = src.AlbumId
	dest.Track = src.Track
	dest.Disc = src.Disc
	dest.Length = src.Length

	dest.ArtistLower = strings.ToLower(src.Artist)
	dest.TitleLower = strings.ToLower(src.Title)
	dest.AlbumLower = strings.ToLower(src.Album)

	keywords := make(map[string]bool)
	for _, s := range []string{dest.ArtistLower, dest.TitleLower, dest.AlbumLower} {
		for _, w := range strings.FieldsFunc(s, func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsNumber(c) }) {
			keywords[w] = true
		}
	}
	dest.Keywords = make([]string, len(keywords))
	i := 0
	for w := range keywords {
		dest.Keywords[i] = w
		i++
	}
}

func copySongUserFields(dest, src *nup.Song) {
	dest.Rating = src.Rating
	dest.FirstStartTime = src.FirstStartTime
	dest.LastStartTime = src.LastStartTime
	dest.NumPlays = src.NumPlays
	dest.Tags = src.Tags
	sort.Strings(dest.Tags)
}

func buildSongPlayStats(s *nup.Song) {
	s.NumPlays = 0
	s.FirstStartTime = time.Time{}
	s.LastStartTime = time.Time{}

	for _, p := range s.Plays {
		s.NumPlays++
		if s.FirstStartTime.IsZero() || p.StartTime.Before(s.FirstStartTime) {
			s.FirstStartTime = p.StartTime
		}
		if s.LastStartTime.IsZero() || p.StartTime.After(s.LastStartTime) {
			s.LastStartTime = p.StartTime
		}
	}
}

func replacePlays(ctx context.Context, songKey *datastore.Key, plays []nup.Play) error {
	playKeys, err := datastore.NewQuery(playKind).Ancestor(songKey).KeysOnly().GetAll(ctx, nil)
	if err != nil {
		return err
	}
	if err = datastore.DeleteMulti(ctx, playKeys); err != nil {
		return err
	}

	playKeys = make([]*datastore.Key, len(plays))
	for i := range plays {
		playKeys[i] = datastore.NewIncompleteKey(ctx, playKind, songKey)
	}
	if _, err = datastore.PutMulti(ctx, playKeys, plays); err != nil {
		return err
	}
	return nil
}

func updateExistingSong(ctx context.Context, id int64,
	f func(context.Context, *nup.Song) error, updateDelay time.Duration) error {
	if err := flushSongFromCache(ctx, id); err != nil {
		return fmt.Errorf("Not updating song %v due to cache eviction error: %v", id, err)
	}

	if updateDelay > 0 {
		time.Sleep(updateDelay)
	}

	cfg := getConfig(ctx)
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		key := datastore.NewKey(ctx, songKind, "", id, nil)
		song := &nup.Song{}
		if err := datastore.Get(ctx, key, song); err != nil {
			return err
		}
		if err := f(ctx, song); err != nil {
			if err == ErrUnmodified {
				log.Debugf(ctx, "Song %v wasn't changed", id)
				return nil
			}
			return err
		}
		if _, err := datastore.Put(ctx, key, song); err != nil {
			return err
		}
		log.Debugf(ctx, "Updated song %v", id)

		if cfg.CacheSongs {
			if err := writeSongsToCache(ctx, []int64{id}, []nup.Song{*song}, true); err != nil {
				return err
			}
		}
		return nil
	}, nil)
}

func addPlay(ctx context.Context, id int64, startTime time.Time, ip string) error {
	err := updateExistingSong(ctx, id, func(ctx context.Context, s *nup.Song) error {
		songKey := datastore.NewKey(ctx, songKind, "", id, nil)
		existingKeys, err := datastore.NewQuery(playKind).Ancestor(songKey).KeysOnly().Filter("StartTime =", startTime).Filter("IpAddress =", ip).GetAll(ctx, nil)
		if err != nil {
			return fmt.Errorf("Querying for existing play failed: %v", err)
		} else if len(existingKeys) > 0 {
			log.Debugf(ctx, "Already have play for song %v starting at %v from %v", id, startTime, ip)
			return ErrUnmodified
		}

		s.NumPlays++
		if s.FirstStartTime.IsZero() || startTime.Before(s.FirstStartTime) {
			s.FirstStartTime = startTime
		}
		if s.LastStartTime.IsZero() || startTime.After(s.LastStartTime) {
			s.LastStartTime = startTime
		}

		newKey := datastore.NewIncompleteKey(ctx, playKind, songKey)
		if _, err = datastore.Put(ctx, newKey, &nup.Play{startTime, ip}); err != nil {
			return fmt.Errorf("Putting play failed: %v", err)
		}
		return nil
	}, time.Duration(0))
	if err != nil {
		return err
	}
	return flushDataFromCacheForUpdate(ctx, playUpdate)
}

func updateRatingAndTags(ctx context.Context, id int64, hasRating bool, rating float64, tags []string, updateDelay time.Duration) error {
	var updateType uint
	err := updateExistingSong(ctx, id, func(ctx context.Context, s *nup.Song) error {
		if hasRating && rating != s.Rating {
			s.Rating = rating
			updateType |= ratingUpdate
		}
		if tags != nil {
			sort.Strings(tags)
			seenTags := make(map[string]bool)
			uniqueTags := make([]string, 0, len(tags))
			for _, tag := range tags {
				if _, seen := seenTags[tag]; !seen {
					uniqueTags = append(uniqueTags, tag)
					seenTags[tag] = true
				}
			}
			if !sortedStringSlicesMatch(uniqueTags, s.Tags) {
				s.Tags = uniqueTags
				updateType |= tagsUpdate
			}
		}
		if updateType != 0 {
			s.LastModifiedTime = time.Now()
			return nil
		} else {
			return ErrUnmodified
		}
	}, updateDelay)

	if err != nil {
		return err
	}
	if updateType != 0 {
		return flushDataFromCacheForUpdate(ctx, updateType)
	}
	return nil
}

func updateOrInsertSong(ctx context.Context, updatedSong *nup.Song, replaceUserData bool, updateDelay time.Duration) error {
	sha1 := updatedSong.Sha1
	queryKeys, err := datastore.NewQuery(songKind).KeysOnly().Filter("Sha1 =", sha1).GetAll(ctx, nil)
	if err != nil {
		return fmt.Errorf("Querying for SHA1 %v failed: %v", sha1, err)
	}
	if len(queryKeys) > 1 {
		return fmt.Errorf("Found %v songs with SHA1 %v; expected 0 or 1", len(queryKeys), sha1)
	}

	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var key *datastore.Key
		song := &nup.Song{}
		if len(queryKeys) == 1 {
			log.Debugf(ctx, "Updating %v with SHA1 %v", updatedSong.Filename, sha1)
			key = queryKeys[0]
			if err := flushSongFromCache(ctx, key.IntID()); err != nil {
				return fmt.Errorf("Not updating song %v due to cache eviction error: %v", key.IntID(), err)
			}
			if !replaceUserData {
				if err := datastore.Get(ctx, key, song); err != nil {
					return fmt.Errorf("Getting %v with key %v failed: %v", sha1, key.IntID(), err)
				}
			}
		} else {
			log.Debugf(ctx, "Inserting %v with SHA1 %v", updatedSong.Filename, sha1)
			key = datastore.NewIncompleteKey(ctx, songKind, nil)
			song.Rating = -1.0
		}

		copySongFileFields(song, updatedSong)
		if replaceUserData {
			buildSongPlayStats(updatedSong)
			copySongUserFields(song, updatedSong)
		}
		song.LastModifiedTime = time.Now()

		if updateDelay > 0 {
			time.Sleep(updateDelay)
		}
		key, err = datastore.Put(ctx, key, song)
		if err != nil {
			return fmt.Errorf("Putting %v failed: %v", key.IntID(), err)
		}
		log.Debugf(ctx, "Put %v with key %v", songKind, key.IntID())

		if replaceUserData {
			if err = replacePlays(ctx, key, updatedSong.Plays); err != nil {
				return err
			}
		}
		if err := writeSongsToCache(ctx, []int64{key.IntID()}, []nup.Song{*song}, true); err != nil {
			return err
		}
		return nil
	}, nil)
}

func deleteSong(ctx context.Context, id int64) error {
	if err := flushSongFromCache(ctx, id); err != nil {
		return fmt.Errorf("Not deleting song %v due to cache eviction error: %v", id, err)
	}

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		songKey := datastore.NewKey(ctx, songKind, "", id, nil)
		song := nup.Song{}
		if err := datastore.Get(ctx, songKey, &song); err != nil {
			return fmt.Errorf("Getting song %v failed: %v", id, err)
		}
		plays := make([]nup.Play, 0)
		playKeys, err := datastore.NewQuery(playKind).Ancestor(songKey).GetAll(ctx, &plays)
		if err != nil {
			return fmt.Errorf("Getting plays for song %v failed: %v", id, err)
		}

		// Delete the old song and plays.
		if err = datastore.Delete(ctx, songKey); err != nil {
			return fmt.Errorf("Deleting song %v failed: %v", id, err)
		}
		if err = datastore.DeleteMulti(ctx, playKeys); err != nil {
			return fmt.Errorf("Deleting %v play(s) for song %v failed: %v", len(playKeys), id, err)
		}

		// Put the deleted song and plays.
		song.LastModifiedTime = time.Now()
		delSongKey := datastore.NewKey(ctx, deletedSongKind, "", id, nil)
		if _, err := datastore.Put(ctx, delSongKey, &song); err != nil {
			return fmt.Errorf("Putting deleted song %v failed: %v", id, err)
		}
		delPlayKeys := make([]*datastore.Key, len(plays))
		for i := range plays {
			delPlayKeys[i] = datastore.NewIncompleteKey(ctx, deletedPlayKind, delSongKey)
		}
		if _, err = datastore.PutMulti(ctx, delPlayKeys, plays); err != nil {
			return fmt.Errorf("Putting %v deleted play(s) for song %v failed: %v", len(plays), id, err)
		}

		return nil
	}, &datastore.TransactionOptions{XG: true})
	if err != nil {
		return err
	}

	return flushDataFromCacheForUpdate(ctx, metadataUpdate)
}

func clearData(ctx context.Context) error {
	// Can't be too careful.
	if !appengine.IsDevAppServer() {
		return fmt.Errorf("Can't clear data on non-dev server")
	}

	log.Debugf(ctx, "Clearing all data")
	for _, kind := range []string{songKind, playKind} {
		keys, err := datastore.NewQuery(kind).KeysOnly().GetAll(ctx, nil)
		if err != nil {
			return fmt.Errorf("Getting all %v keys failed: %v", kind, err)
		}
		if err = datastore.DeleteMulti(ctx, keys); err != nil {
			return fmt.Errorf("Deleting all %v entities failed: %v", kind, err)
		}
	}
	return nil
}
