// Copyright 2022 Daniel Erat.
// All rights reserved.

package db

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
	// TotalSec is the total duration in seconds of all songs.
	TotalSec float64 `json:"totalSec"`
	// Ratings maps from "%.2f" rating in [0.0, 1.0] (or -1.0 for unrated) to number of songs with that rating.
	Ratings map[string]int `json:"ratings"`
	// Tags maps from tag to number of songs with that tag.
	Tags map[string]int `json:"tags"`
	// Years maps from year (e.g. 2020) to stats about plays in that year.
	Years map[int]PlayStats `json:"years"`
}

// NewStats returns a new Stats struct with all fields initialized to 0.
func NewStats() *Stats {
	return &Stats{
		Ratings: make(map[string]int),
		Tags:    make(map[string]int),
		Years:   make(map[int]PlayStats),
	}
}

// PlayStats summarizes plays in a time interval.
type PlayStats struct {
	// Plays contains the number of plays.
	Plays int `json:"plays"`
	// TotalSec is the total duration in seconds of played songs.
	TotalSec float64 `json:"totalSec"`
}
