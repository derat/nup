package appengine

import (
	"appengine"
	"appengine/datastore"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"erat.org/nup"
)

var (
	ErrSongUnchanged = errors.New("Song wasn't modified")
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

func replacePlays(c appengine.Context, songKey *datastore.Key, plays []nup.Play) error {
	playKeys, err := datastore.NewQuery(playKind).Ancestor(songKey).KeysOnly().GetAll(c, nil)
	if err != nil {
		return err
	}
	if err = datastore.DeleteMulti(c, playKeys); err != nil {
		return err
	}

	playKeys = make([]*datastore.Key, len(plays))
	for i := range plays {
		playKeys[i] = datastore.NewIncompleteKey(c, playKind, songKey)
	}
	if _, err = datastore.PutMulti(c, playKeys, plays); err != nil {
		return err
	}
	return nil
}

func updateExistingSong(c appengine.Context, id int64, f func(appengine.Context, *nup.Song) error) error {
	return datastore.RunInTransaction(c, func(c appengine.Context) error {
		key := datastore.NewKey(c, songKind, "", id, nil)
		song := &nup.Song{}
		if err := datastore.Get(c, key, song); err != nil {
			return err
		}
		if err := f(c, song); err != nil {
			if err == ErrSongUnchanged {
				c.Debugf("Song %v wasn't changed", id)
			}
			return err
		}
		song.LastModifiedTime = time.Now()
		if _, err := datastore.Put(c, key, song); err != nil {
			return err
		}
		c.Debugf("Updated song %v", id)
		return nil
	}, nil)
}

func addPlay(c appengine.Context, id int64, startTime time.Time, ip string) error {
	return updateExistingSong(c, id, func(c appengine.Context, s *nup.Song) error {
		songKey := datastore.NewKey(c, songKind, "", id, nil)
		existingKeys, err := datastore.NewQuery(playKind).Ancestor(songKey).KeysOnly().Filter("StartTime =", startTime).Filter("IpAddress =", ip).GetAll(c, nil)
		if err != nil {
			return fmt.Errorf("Querying for existing play failed: %v", err)
		} else if len(existingKeys) > 0 {
			return fmt.Errorf("Already have play for song %v starting at %v from %v", id, startTime, ip)
		}

		s.NumPlays++
		if s.FirstStartTime.IsZero() || startTime.Before(s.FirstStartTime) {
			s.FirstStartTime = startTime
		}
		if s.LastStartTime.IsZero() || startTime.After(s.LastStartTime) {
			s.LastStartTime = startTime
		}

		newKey := datastore.NewIncompleteKey(c, playKind, songKey)
		if _, err = datastore.Put(c, newKey, &nup.Play{startTime, ip}); err != nil {
			return fmt.Errorf("Putting play failed: %v", err)
		}
		return nil
	})
}

func updateRatingAndTags(c appengine.Context, id int64, hasRating bool, rating float64, tags []string) error {
	if err := updateExistingSong(c, id, func(c appengine.Context, s *nup.Song) error {
		var updated bool
		if hasRating && rating != s.Rating {
			s.Rating = rating
			updated = true
		}
		if tags != nil {
			sort.Strings(tags)
			if !sortedStringSlicesMatch(tags, s.Tags) {
				s.Tags = tags
				updated = true
			}
		}
		if updated {
			return nil
		} else {
			return ErrSongUnchanged
		}
	}); err != nil && err != ErrSongUnchanged {
		return err
	}
	return nil
}

func updateOrInsertSong(c appengine.Context, updatedSong *nup.Song, replaceUserData bool) error {
	sha1 := updatedSong.Sha1
	queryKeys, err := datastore.NewQuery(songKind).KeysOnly().Filter("Sha1 =", sha1).GetAll(c, nil)
	if err != nil {
		return fmt.Errorf("Querying for SHA1 %v failed: %v", sha1, err)
	}
	if len(queryKeys) > 1 {
		return fmt.Errorf("Found %v songs with SHA1 %v; expected 0 or 1", sha1)
	}

	return datastore.RunInTransaction(c, func(c appengine.Context) error {
		var key *datastore.Key
		song := &nup.Song{}
		if len(queryKeys) == 1 {
			c.Debugf("Updating %v with SHA1 %v", updatedSong.Filename, sha1)
			key = queryKeys[0]
			if !replaceUserData {
				if err := datastore.Get(c, key, song); err != nil {
					return fmt.Errorf("Getting %v failed: %v", sha1, key.IntID(), err)
				}
			}
		} else {
			c.Debugf("Inserting %v with SHA1 %v", updatedSong.Filename, sha1)
			key = datastore.NewIncompleteKey(c, songKind, nil)
			song.Rating = -1.0
		}

		copySongFileFields(song, updatedSong)
		if replaceUserData {
			buildSongPlayStats(updatedSong)
			copySongUserFields(song, updatedSong)
		}
		song.LastModifiedTime = time.Now()
		key, err = datastore.Put(c, key, song)
		if err != nil {
			return fmt.Errorf("Putting %v failed: %v", key.IntID(), err)
		}
		c.Debugf("Put %v with key %v", songKind, key.IntID())

		if replaceUserData {
			if err = replacePlays(c, key, updatedSong.Plays); err != nil {
				return err
			}
		}
		return nil
	}, nil)
}

func clearData(c appengine.Context) error {
	// Can't be too careful.
	if !appengine.IsDevAppServer() {
		return fmt.Errorf("Can't clear data on non-dev server")
	}

	c.Debugf("Clearing all data")
	for _, kind := range []string{songKind, playKind} {
		keys, err := datastore.NewQuery(kind).KeysOnly().GetAll(c, nil)
		if err != nil {
			return fmt.Errorf("Getting all %v keys failed: %v", kind, err)
		}
		if err = datastore.DeleteMulti(c, keys); err != nil {
			return fmt.Errorf("Deleting all %v entities failed: %v", kind, err)
		}
	}
	return nil
}
