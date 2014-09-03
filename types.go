package nup

import (
	"time"
)

// Play represents one playback of a Song.
type Play struct {
	// Time at which playback started.
	StartTime time.Time

	// Client playing the song.
	IpAddress string
}

// Song represents an audio file.
type Song struct {
	// SHA1 hash of the audio portion of the file.
	Sha1 string

	// Relative path from the base of the music directory.
	// Must be escaped for Cloud Storage.
	Filename string

	// Canonical versions used for display.
	Artist string `datastore:",noindex"`
	Title  string `datastore:",noindex"`
	Album  string `datastore:",noindex"`

	// Lowercase versions used for searching.
	ArtistLower string `json:"-"`
	TitleLower  string `json:"-"`
	AlbumLower  string `json:"-"`

	// Words from ArtistLower, TitleLower, and AlbumLower used for searching.
	Keywords []string `json:"-"`

	// Track and disc number or 0 if unset.
	Track int `json:",omitempty"`
	Disc  int `json:",omitempty"`

	// Length in milliseconds.
	LengthMs int64

	// Last time the song was modified.
	LastModifiedTime time.Time `json:"-"`

	// Rating in the range [0.0, 1.0] or -1 if unrated.
	Rating float32

	// First and last time the song was played.
	FirstStartTime time.Time `json:",omitempty"`
	LastStartTime  time.Time `json:",omitempty"`

	// Number of times the song has been played.
	NumPlays int `json:",omitempty"`

	// The song's playback history.
	// Only used for importing data -- in Datastore, Play is a descendant of Song.
	Plays []Play `json:",omitempty",datastore:",noindex"`

	// Tags assigned to the song.
	Tags []string `json:",omitempty"`
}
