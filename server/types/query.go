// Copyright 2021 Daniel Erat.
// All rights reserved.

package types

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

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
	Disc  int64 // Song.Disc (may be 0 or 1 for single-disc albums)

	MaxDisc    int64 // Song.Disc
	HasMaxDisc bool  // MaxDisc is set

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

// UpdateTypes is a bitfield describing what was changed by an update.
type UpdateTypes uint8

const (
	MetadataUpdate UpdateTypes = 1 << iota // song metadata
	RatingUpdate
	TagsUpdate
	PlaysUpdate
)
