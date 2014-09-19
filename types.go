package nup

import (
	"time"
)

// Play represents one playback of a Song.
type Play struct {
	// Time at which playback started.
	StartTime time.Time `json:"t"`

	// Client playing the song.
	IpAddress string `json:"ip"`
}

// Song represents an audio file.
type Song struct {
	// SHA1 hash of the audio portion of the file.
	Sha1 string `json:"sha1,omitempty"`

	// Song entity's key ID from Datastore. Only set in search results.
	SongId string `datastore:"-" json:"songId,omitempty"`

	// Relative path from the base of the music directory.
	// Must be escaped for Cloud Storage when constructing Url.
	Filename string `json:"filename,omitempty"`

	// Relative path from the base of the covers directory.
	// Must be escaped for Cloud Storage when constructing CoverUrl.
	CoverFilename string `datastore:",noindex" json:"coverFilename,omitempty"`

	// URL of the song and cover art. Only set in search results.
	Url      string `datastore:"-" json:"url,omitempty"`
	CoverUrl string `datastore:"-" json:"coverUrl,omitempty"`

	// Canonical versions used for display.
	Artist string `datastore:",noindex" json:"artist"`
	Title  string `datastore:",noindex" json:"title"`
	Album  string `datastore:",noindex" json:"album"`

	// Lowercase versions used for searching.
	ArtistLower string `json:"-"`
	TitleLower  string `json:"-"`
	AlbumLower  string `json:"-"`

	// Words from ArtistLower, TitleLower, and AlbumLower used for searching.
	Keywords []string `json:"-"`

	// Track and disc number or 0 if unset.
	Track int `json:"track"`
	Disc  int `json:"disc"`

	// Length in seconds.
	Length float64 `json:"length"`

	// Rating in the range [0.0, 1.0] or -1 if unrated.
	Rating float64 `json:"rating"`

	// First and last time the song was played.
	FirstStartTime time.Time `json:"-"`
	LastStartTime  time.Time `json:"-"`

	// Number of times the song has been played.
	NumPlays int `json:"-"`

	// The song's playback history.
	// Only used for importing data -- in Datastore, Play is a descendant of Song.
	Plays []Play `datastore:"-" json:"plays,omitempty"`

	// Tags assigned to the song by the user.
	Tags []string `json:"tags"`

	// Last time the song was modified.
	LastModifiedTime time.Time `json:"-"`
}

// ClientConfig holds configuration details shared across client binaries.
type ClientConfig struct {
	ServerUrl string
	Username  string
	Password  string
}
