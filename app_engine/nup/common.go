package appengine

import (
	"errors"
	"strconv"
	"time"

	"erat.org/nup"
	"erat.org/nup/lib"
)

const (
	// Datastore kinds of various objects.
	playKind        = "Play"
	songKind        = "Song"
	deletedPlayKind = "DeletedPlay"
	deletedSongKind = "DeletedSong"
)

var (
	ErrUnmodified = errors.New("Object wasn't modified")
)

func prepareSongForClient(s *nup.Song, id int64, cfg *nup.ServerConfig, client lib.ClientType) {
	// Set fields that are only present in search results (i.e. not in Datastore).
	s.SongId = strconv.FormatInt(id, 10)

	// Turn the bare music and cover filenames into URLs.
	getUrl := func(filename, bucket, baseUrl string) string {
		if len(filename) == 0 {
			return ""
		}
		if len(bucket) > 0 {
			return lib.GetCloudStorageUrl(bucket, filename, client)
		}
		return baseUrl + filename
	}
	s.Url = getUrl(s.Filename, cfg.SongBucket, cfg.SongBaseUrl)
	s.CoverUrl = getUrl(s.CoverFilename, cfg.CoverBucket, cfg.CoverBaseUrl)

	// Create an empty tags slice so that clients don't need to check for null.
	if s.Tags == nil {
		s.Tags = make([]string, 0)
	}

	// Clear fields that are passed for updates (and hence not excluded from JSON)
	// but that aren't needed in search results.
	s.Sha1 = ""
	s.Filename = ""
	s.CoverFilename = ""
	s.Plays = s.Plays[:0]
}

func getMsecSinceTime(t time.Time) int64 {
	return time.Now().Sub(t).Nanoseconds() / int64(time.Millisecond/time.Nanosecond)
}
