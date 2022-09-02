// Copyright 2022 Daniel Erat.
// All rights reserved.

package db

import "time"

const (
	// StatsKind is the Stats struct's Datastore kind.
	StatsKind = "Stats"
	// StatsKeyName is the Stats struct's key name for both Datastore and memcache.
	StatsKeyName = "stats"
)

// Stats summarizes information from the database.
type Stats struct {
	// Songs is the total number of songs in the database.
	Songs int `json:"songs"`
	// Albums is the total number of albums in the database.
	// This is computed by counting distinct Song.AlbumID values.
	Albums int `json:"albums"`
	// TotalSec is the total duration in seconds of all songs.
	TotalSec float64 `json:"totalSec"`
	// Ratings maps from a rating in [1, 5] (or 0 for unrated) to number of songs with that rating.
	Ratings map[int]int `json:"ratings"`
	// SongDecades maps from the year at the beginning of a decade (e.g. 1990) to the number of
	// songs in the database with a Date field in the decade. 0 is used for songs with unset dates.
	SongDecades map[int]int `json:"songDecades"`
	// Tags maps from tag to number of songs with that tag.
	Tags map[string]int `json:"tags"`
	// Years maps from year (e.g. 2020) to stats about plays in that year.
	Years map[int]PlayStats `json:"years"`
	// UpdateTime is the time at which these stats were generated.
	UpdateTime time.Time `json:"updateTime"`
}

// NewStats returns a new Stats struct with all fields initialized to 0.
func NewStats() *Stats {
	return &Stats{
		Ratings:     make(map[int]int),
		SongDecades: make(map[int]int),
		Tags:        make(map[string]int),
		Years:       make(map[int]PlayStats),
	}
}

// PlayStats summarizes plays in a time interval.
type PlayStats struct {
	// Plays is the number of plays.
	Plays int `json:"plays"`
	// TotalSec is the total duration in seconds of played songs.
	TotalSec float64 `json:"totalSec"`
	// FirstPlays is the number of songs that were first played in the interval.
	FirstPlays int `json:"firstPlays"`
	// LastPlays is the number of songs that were last played in the interval.
	LastPlays int `json:"lastPlays"`
}
