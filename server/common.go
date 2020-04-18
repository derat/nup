package main

import (
	"errors"
	"strconv"
	"time"

	"github.com/derat/nup/cloudutil"
	"github.com/derat/nup/types"
)

const (
	// Datastore kinds of various objects.
	playKind        = "Play"
	songKind        = "Song"
	deletedPlayKind = "DeletedPlay"
	deletedSongKind = "DeletedSong"
)

var errUnmodified = errors.New("object wasn't modified")

func prepareSongForClient(s *types.Song, id int64, cfg *types.ServerConfig, client cloudutil.ClientType) {
	// Set fields that are only present in search results (i.e. not in Datastore).
	s.SongID = strconv.FormatInt(id, 10)

	// Turn the bare music and cover filenames into URLs.
	getURL := func(filename, bucket, baseURL string) string {
		if len(filename) == 0 {
			return ""
		}
		if len(bucket) > 0 {
			return cloudutil.CloudStorageURL(bucket, filename, client)
		}
		return baseURL + filename
	}
	s.URL = getURL(s.Filename, cfg.SongBucket, cfg.SongBaseURL)
	s.CoverURL = getURL(s.CoverFilename, cfg.CoverBucket, cfg.CoverBaseURL)

	// Create an empty tags slice so that clients don't need to check for null.
	if s.Tags == nil {
		s.Tags = make([]string, 0)
	}

	// Clear fields that are passed for updates (and hence not excluded from JSON)
	// but that aren't needed in search results.
	s.SHA1 = ""
	s.Filename = ""
	s.Plays = s.Plays[:0]
	// Preserve CoverFilename so clients can pass it to /cover.
}

func getMsecSinceTime(t time.Time) int64 {
	return time.Now().Sub(t).Nanoseconds() / int64(time.Millisecond/time.Nanosecond)
}
