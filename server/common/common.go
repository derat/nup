// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package common contains code shared across multiple packages.
package common

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/derat/nup/internal/pkg/cloudutil"
	"github.com/derat/nup/internal/pkg/types"
)

const (
	// Datastore kinds of various objects.
	PlayKind        = "Play"
	SongKind        = "Song"
	DeletedPlayKind = "DeletedPlay"
	DeletedSongKind = "DeletedSong"
)

// UpdateTypes is a bitfield describing what was changed by an update.
type UpdateTypes uint8

const (
	MetadataUpdate UpdateTypes = 1 << iota // song metadata
	RatingUpdate
	TagsUpdate
	PlaysUpdate
)

var ErrUnmodified = errors.New("object wasn't modified")

// PrepareSongForClient sets and clears fields in s appropriately for client.
func PrepareSongForClient(s *types.Song, id int64, cfg *types.ServerConfig, client cloudutil.ClientType) {
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

	// Proxy MP3s for browsers to avoid cross-origin requests.
	if client == cloudutil.WebClient {
		s.URL = "/song_data?filename=" + url.QueryEscape(s.Filename)
	}

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

// SongQuery describes a query returning a list of Songs.
type SongQuery struct {
	Artist  string // Song.Artist
	Title   string // Song.Title
	Album   string // Song.Album
	AlbumID string // Song.AlbumID

	Keywords []string // Song.Keywords

	MinRating    float64 // Song.Rating
	HasMinRating bool    // MinRating is set
	Unrated      bool    // Song.Rating is -1

	MaxPlays    int64 // Song.NumPlays
	HasMaxPlays bool  // MaxPlays is set

	MinFirstStartTime time.Time // Song.FirstStartTime
	MaxLastStartTime  time.Time // Song.LastStartTime

	Track int64 // Song.Track
	Disc  int64 // Song.Disc

	Tags    []string // present in Song.Tags
	NotTags []string // not present in Song.Tags

	Shuffle bool // randomize results set/order
}

// Hash returns a unique string identifying q.
func (q *SongQuery) Hash() string {
	b, err := json.Marshal(q)
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal query: %v", err))
	}
	s := sha1.Sum(b)
	return hex.EncodeToString(s[:])
}

// CanCache returns true if the query's results can be safely cached.
func (q *SongQuery) CanCache() bool {
	return !q.HasMaxPlays && q.MinFirstStartTime.IsZero() && q.MaxLastStartTime.IsZero()
}

// ResultsInvalidated returns true if the updates described by ut would
// invalidate q's cached results.
func (q *SongQuery) ResultsInvalidated(ut UpdateTypes) bool {
	if (ut & MetadataUpdate) != 0 {
		return true
	}
	if (ut&RatingUpdate) != 0 && (q.HasMinRating || q.Unrated) {
		return true
	}
	if (ut&TagsUpdate) != 0 && (len(q.Tags) > 0 || len(q.NotTags) > 0) {
		return true
	}
	if (ut&PlaysUpdate) != 0 &&
		(q.HasMaxPlays || !q.MinFirstStartTime.IsZero() || !q.MaxLastStartTime.IsZero()) {
		return true
	}
	return false
}
