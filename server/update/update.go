// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package update updates songs in datastore.
package update

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/server/query"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/log"
)

var errUnmodified = errors.New("object wasn't modified")

const reindexBatchSize = 1000

// AddPlay adds a play report to the song identified by id in datastore.
func AddPlay(ctx context.Context, id int64, startTime time.Time, ip string) error {
	err := updateExistingSong(ctx, id, func(ctx context.Context, s *db.Song) error {
		songKey := datastore.NewKey(ctx, db.SongKind, "", id, nil)
		existingKeys, err := datastore.NewQuery(db.PlayKind).Ancestor(songKey).KeysOnly().
			Filter("StartTime =", startTime).Filter("IpAddress =", ip).GetAll(ctx, nil)
		if err != nil {
			return fmt.Errorf("querying for existing play failed: %v", err)
		} else if len(existingKeys) > 0 {
			log.Debugf(ctx, "Already have play for song %v starting at %v from %v", id, startTime, ip)
			return errUnmodified
		}

		s.UpdatePlayStats(startTime)

		newKey := datastore.NewIncompleteKey(ctx, db.PlayKind, songKey)
		if _, err = datastore.Put(ctx, newKey, &db.Play{ // must pass pointer
			StartTime: startTime,
			IPAddress: ip,
		}); err != nil {
			return fmt.Errorf("putting play failed: %v", err)
		}
		return nil
	}, 0, true)
	if err != nil {
		return err
	}
	return query.FlushCacheForUpdate(ctx, query.PlaysUpdate)
}

// SetRatingAndTags updates the rating and tags of the song identified by id in datastore.
// The rating is only updated if hasRating is true, and tags are not updated if tags is nil.
// If delay is nonzero, the server will wait before writing to datastore.
func SetRatingAndTags(ctx context.Context, id int64, hasRating bool, rating int,
	tags []string, delay time.Duration) error {
	var ut query.UpdateTypes
	err := updateExistingSong(ctx, id, func(ctx context.Context, s *db.Song) error {
		if hasRating && rating != s.Rating {
			s.SetRating(rating)
			ut |= query.RatingUpdate
		}
		if tags != nil {
			oldTags := s.Tags
			s.Tags = tags
			s.Clean() // sort and dedupe
			if !stringSlicesMatch(oldTags, s.Tags) {
				ut |= query.TagsUpdate
			}
		}
		if ut == 0 {
			return errUnmodified
		}
		s.LastModifiedTime = time.Now()
		return nil
	}, delay, true)

	if err != nil {
		return err
	}
	if ut != 0 {
		return query.FlushCacheForUpdate(ctx, ut)
	}
	return nil
}

// UserDataPolicy indicates what UpdateOrInsertSong should do with existing user data
// (e.g. ratings, tags, plays) when updating a song.
type UserDataPolicy int

const (
	// PreserveUserData indicates that the current user data should not be changed.
	PreserveUserData UserDataPolicy = iota
	// ReplaceUserData indicates that user data should be replaced by data from the imported song.
	ReplaceUserData
)

// UpdateKeyType indicates the key that UpdateOrInsertSong should use to determine which
// song to update.
type UpdateKeyType int

const (
	// UpdateBySHA1 indicates that the Song.SHA1 field should be used.
	// This identifies songs by their audio data, so they will be tracked across renames or moves.
	UpdateBySHA1 UpdateKeyType = iota
	// UpdateByFilename indicates that the Song.Filename field should be used.
	// This is useful when a file's audio data has been deliberately updated.
	UpdateByFilename
)

// UpdateOrInsertSong stores the supplied song in datastore.
// If delay is nonzero, the server will wait before writing to datastore.
func UpdateOrInsertSong(ctx context.Context, updated *db.Song,
	dataPolicy UserDataPolicy, keyType UpdateKeyType, delay time.Duration) error {
	base := datastore.NewQuery(db.SongKind).KeysOnly()
	queryKeys, err := base.Filter("Sha1 =", updated.SHA1).GetAll(ctx, nil)
	if err != nil {
		return fmt.Errorf("querying for SHA1 %v failed: %v", updated.SHA1, err)
	} else if len(queryKeys) > 1 {
		return fmt.Errorf("found %v songs with SHA1 %v", len(queryKeys), updated.SHA1)
	}

	if keyType == UpdateByFilename {
		var oldKey *datastore.Key
		if len(queryKeys) > 0 {
			oldKey = queryKeys[0]
		}
		if queryKeys, err = base.Filter("Filename =", updated.Filename).GetAll(ctx, nil); err != nil {
			return fmt.Errorf("querying for %q failed: %v", updated.Filename, err)
		} else if len(queryKeys) > 1 {
			return fmt.Errorf("found %v songs with filename %q", len(queryKeys), updated.Filename)
		} else if oldKey != nil && (len(queryKeys) == 0 || queryKeys[0].IntID() != oldKey.IntID()) {
			// If the song's SHA1 is already present in the database with a different filename,
			// avoid inserting or updating another entity to have the same SHA1.
			return fmt.Errorf("existing song %v already has SHA1 %v", oldKey.IntID(), updated.SHA1)
		}
	}

	replace := dataPolicy == ReplaceUserData
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var key *datastore.Key
		var song db.Song
		if len(queryKeys) == 1 {
			key = queryKeys[0]
			log.Debugf(ctx, "Updating song %v with SHA1 %v and filename %q",
				key.IntID(), updated.SHA1, updated.Filename)
			if !replace {
				// If we're preserving the existing user data, we need to load it first.
				if err := datastore.Get(ctx, key, &song); err != nil {
					return fmt.Errorf("getting song %v failed: %v", key.IntID(), err)
				}
			}
		} else {
			log.Debugf(ctx, "Inserting song with SHA1 %v and filename %q",
				updated.SHA1, updated.Filename)
			key = datastore.NewIncompleteKey(ctx, db.SongKind, nil)
		}

		if err := song.Update(updated, replace); err != nil {
			return err
		}
		if replace {
			song.RebuildPlayStats(updated.Plays)
		}
		song.LastModifiedTime = time.Now()

		time.Sleep(delay)
		key, err = datastore.Put(ctx, key, &song) // must pass pointer
		if err != nil {
			return fmt.Errorf("putting %v failed: %v", key.IntID(), err)
		}
		log.Debugf(ctx, "Put song %v", key.IntID())

		if replace {
			if err := replacePlays(ctx, key, updated.Plays); err != nil {
				return err
			}
		}
		return nil
	}, nil)
}

// DeleteSong deletes the song identified by id from datastore.
func DeleteSong(ctx context.Context, id int64) error {
	if err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		songKey := datastore.NewKey(ctx, db.SongKind, "", id, nil)
		song := db.Song{}
		if err := datastore.Get(ctx, songKey, &song); err != nil {
			return fmt.Errorf("getting song %v failed: %v", id, err)
		}
		plays := make([]db.Play, 0)
		playKeys, err := datastore.NewQuery(db.PlayKind).Ancestor(songKey).GetAll(ctx, &plays)
		if err != nil {
			return fmt.Errorf("getting plays for song %v failed: %v", id, err)
		}

		// Delete the old song and plays.
		if err = datastore.Delete(ctx, songKey); err != nil {
			return fmt.Errorf("deleting song %v failed: %v", id, err)
		}
		if err = datastore.DeleteMulti(ctx, playKeys); err != nil {
			return fmt.Errorf("deleting %v play(s) for song %v failed: %v", len(playKeys), id, err)
		}

		// Put the deleted song and plays.
		song.LastModifiedTime = time.Now()
		delSongKey := datastore.NewKey(ctx, db.DeletedSongKind, "", id, nil)
		if _, err := datastore.Put(ctx, delSongKey, &song); err != nil { // must pass pointer
			return fmt.Errorf("putting deleted song %v failed: %v", id, err)
		}
		delPlayKeys := make([]*datastore.Key, len(plays))
		for i := range plays {
			delPlayKeys[i] = datastore.NewIncompleteKey(ctx, db.DeletedPlayKind, delSongKey)
		}
		if _, err = datastore.PutMulti(ctx, delPlayKeys, plays); err != nil {
			return fmt.Errorf("putting %v deleted play(s) for song %v failed: %v", len(plays), id, err)
		}

		return nil
	}, &datastore.TransactionOptions{XG: true}); err != nil {
		return err
	}

	return query.FlushCacheForUpdate(ctx, query.MetadataUpdate)
}

// ReindexSongs regenerates various fields for all songs in the database and updates songs that
// were changed. If nextCursor is non-empty, ReindexSongs should be called again to continue reindexing.
func ReindexSongs(ctx context.Context, cursor string) (nextCursor string, scanned, updated int, err error) {
	q := datastore.NewQuery(db.SongKind).KeysOnly()
	if len(cursor) > 0 {
		dc, err := datastore.DecodeCursor(cursor)
		if err != nil {
			return "", 0, 0, fmt.Errorf("decode cursor %q: %v", cursor, err)
		}
		q = q.Start(dc)
	}

	it := q.Run(ctx)
	var ids []int64
	for {
		if k, err := it.Next(nil); err == nil {
			ids = append(ids, k.IntID())
		} else if err == datastore.Done {
			break
		} else {
			return "", 0, 0, err
		}
		if len(ids) == reindexBatchSize {
			nc, err := it.Cursor()
			if err != nil {
				return "", 0, 0, fmt.Errorf("get cursor: %v", err)
			}
			nextCursor = nc.String()
			break
		}
	}

	for _, id := range ids {
		var update bool
		if err := updateExistingSong(ctx, id, func(ctx context.Context, s *db.Song) error {
			scanned++

			// Write the song back if we converted from a floating-point rating.
			if s.RatingWasFloat {
				update = true
				return nil
			}

			var up db.Song
			if err := up.Update(s, true /* copyUserData */); err != nil {
				return err
			}

			// The Keywords field is derived from ArtistLower, TitleLower, and AlbumLower,
			// so it will only change if one or more of those fields changed.
			if up.ArtistLower == s.ArtistLower &&
				up.TitleLower == s.TitleLower &&
				up.AlbumLower == s.AlbumLower &&
				up.RatingAtLeast1 == s.RatingAtLeast1 &&
				up.RatingAtLeast2 == s.RatingAtLeast2 &&
				up.RatingAtLeast3 == s.RatingAtLeast3 &&
				up.RatingAtLeast4 == s.RatingAtLeast4 &&
				reflect.DeepEqual(up.Tags, s.Tags) {
				return errUnmodified
			}

			// LastModifiedTime isn't updated since these fields aren't exposed to clients.
			s.ArtistLower = up.ArtistLower
			s.TitleLower = up.TitleLower
			s.AlbumLower = up.AlbumLower
			s.Keywords = up.Keywords
			s.RatingAtLeast1 = up.RatingAtLeast1
			s.RatingAtLeast2 = up.RatingAtLeast2
			s.RatingAtLeast3 = up.RatingAtLeast3
			s.RatingAtLeast4 = up.RatingAtLeast4
			s.Tags = up.Tags

			update = true
			return nil
		}, 0, false); err != nil {
			return "", scanned, updated, fmt.Errorf("song %d: %v", id, err)
		}
		if update {
			updated++
		}
	}
	log.Debugf(ctx, "Scanned %d songs for reindex, updated %d", scanned, updated)
	return nextCursor, scanned, updated, nil
}

// ClearData deletes all song and play objects from datastore.
// It's intended for testing and can only be called on dev servers.
func ClearData(ctx context.Context) error {
	// Can't be too careful.
	if !appengine.IsDevAppServer() {
		return errors.New("can't clear data on non-dev server")
	}

	log.Debugf(ctx, "Clearing all data")
	for _, kind := range []string{db.SongKind, db.PlayKind} {
		keys, err := datastore.NewQuery(kind).KeysOnly().GetAll(ctx, nil)
		if err != nil {
			return fmt.Errorf("getting all %v keys failed: %v", kind, err)
		}
		if err = datastore.DeleteMulti(ctx, keys); err != nil {
			return fmt.Errorf("deleting all %v entities failed: %v", kind, err)
		}
	}
	return nil
}

func stringSlicesMatch(a, b []string) bool {
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

func replacePlays(ctx context.Context, songKey *datastore.Key, plays []db.Play) error {
	playKeys, err := datastore.NewQuery(db.PlayKind).Ancestor(songKey).KeysOnly().GetAll(ctx, nil)
	if err != nil {
		return err
	}
	if err = datastore.DeleteMulti(ctx, playKeys); err != nil {
		return err
	}

	playKeys = make([]*datastore.Key, len(plays))
	for i := range plays {
		playKeys[i] = datastore.NewIncompleteKey(ctx, db.PlayKind, songKey)
	}
	if _, err = datastore.PutMulti(ctx, playKeys, plays); err != nil {
		return err
	}
	return nil
}

func updateExistingSong(ctx context.Context, id int64, f func(context.Context, *db.Song) error,
	delay time.Duration, shouldLog bool) error {
	time.Sleep(delay)

	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		key := datastore.NewKey(ctx, db.SongKind, "", id, nil)
		var song db.Song
		if err := datastore.Get(ctx, key, &song); err != nil {
			return err
		}
		if err := f(ctx, &song); err != nil {
			if err == errUnmodified {
				if shouldLog {
					log.Debugf(ctx, "Song %v wasn't changed", id)
				}
				return nil
			}
			return err
		}
		if _, err := datastore.Put(ctx, key, &song); err != nil { // must pass pointer
			return err
		}
		if shouldLog {
			log.Debugf(ctx, "Updated song %v", id)
		}
		return nil
	}, nil)
}
