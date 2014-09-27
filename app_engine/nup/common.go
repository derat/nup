package appengine

import (
	"fmt"
	"strconv"
	"time"

	"erat.org/nup"
)

type clientType int

const (
	webClient clientType = iota
	androidClient
)

func prepareSongForClient(s *nup.Song, id int64, client clientType) {
	// Set fields that are only present in search results (i.e. not in Datastore).
	s.SongId = strconv.FormatInt(id, 10)

	getUrl := func(bucketName, filePath string) string {
		switch client {
		case webClient:
			return fmt.Sprintf("https://storage.cloud.google.com/%s/%s", bucketName, nup.EncodePathForCloudStorage(filePath))
		case androidClient:
			return fmt.Sprintf("https://%s.storage.googleapis.com/%s", bucketName, nup.EncodePathForCloudStorage(filePath))
		default:
			return ""
		}
	}
	if len(s.Filename) > 0 {
		s.Url = getUrl(cfg.SongBucket, s.Filename)
	}
	if len(s.CoverFilename) > 0 {
		s.CoverUrl = getUrl(cfg.CoverBucket, s.CoverFilename)
	}

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
