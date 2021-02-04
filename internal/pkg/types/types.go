// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package types defines shared types.
package types

import (
	"time"
)

// Play represents one playback of a Song.
type Play struct {
	// StartTime is the time at which playback started.
	StartTime time.Time `json:"t"`
	// IPAddress is the IPv4 or IPv6 address of the client playing the song.
	IPAddress string `datastore:"IpAddress" json:"ip"`
}

// Song represents an audio file.
type Song struct {
	// SHA1 is a hash of the audio portion of the file.
	SHA1 string `datastore:"Sha1" json:"sha1,omitempty"`

	// SongID is the Song entity's key ID from Datastore. Only set in search results.
	SongID string `datastore:"-" json:"songId,omitempty"`

	// Filename is a relative path from the base of the music directory.
	// Must be escaped for Cloud Storage when constructing URL.
	Filename string `json:"filename,omitempty"`

	// CoverFilename is a relative path from the base of the covers directory.
	// Must be escaped for Cloud Storage when constructing CoverURL.
	// Clients can pass this to the server's /cover endpoint to get a scaled
	// copy of the cover.
	CoverFilename string `datastore:",noindex" json:"coverFilename,omitempty"`

	// URL is the song's URL. Only set in search results.
	URL string `datastore:"-" json:"url,omitempty"`
	// CoverURL is the cover image's URL. Only set in search results.
	CoverURL string `datastore:"-" json:"coverUrl,omitempty"`

	// Canonical versions used for display.
	Artist string `datastore:",noindex" json:"artist"`
	Title  string `datastore:",noindex" json:"title"`
	Album  string `datastore:",noindex" json:"album"`

	// Lowercase versions used for searching and sorting.
	ArtistLower string `json:"-"`
	TitleLower  string `json:"-"`
	AlbumLower  string `json:"-"`

	// Keywords contains words from ArtistLower, TitleLower, and AlbumLower used for searching.
	Keywords []string `json:"-"`

	// AlbumID is an opaque ID uniquely identifying the album
	// (generally, a MusicBrainz release ID taken from a "MusicBrainz Album Id" ID3v2 tag).
	AlbumID string `datastore:"AlbumId" json:"albumId,omitempty"`

	// RecordingID is an opaque ID uniquely identifying the recording
	// (generally, the MusicBrainz ID corresponding to the MusicBrainz recording entity,
	// taken from a UFID ID3v2 tag). Only used to find cover art for non-album tracks.
	RecordingID string `datastore:"-" json:"-"`

	// Track is the song's track number, or 0 if unset.
	Track int `json:"track"`
	// Disc is the song's disc number, or 0 if unset.
	Disc int `json:"disc"`

	// Length is the song's duration in seconds.
	Length float64 `json:"length"`

	// Rating is the song's rating in the range [0.0, 1.0], or -1 if unrated.
	Rating float64 `json:"rating"`

	// FirstStartTime is the first time the song was played.
	FirstStartTime time.Time `json:"-"`
	// LastStartTime is the last time the song was played.
	LastStartTime time.Time `json:"-"`

	// NumPlays is the number of times the song has been played.
	NumPlays int `json:"-"`

	// Plays contains the song's playback history.
	// Only used for importing data -- in Datastore, Play is a descendant of Song.
	Plays []Play `datastore:"-" json:"plays,omitempty"`

	// Tags contains tags assigned to the song by the user.
	Tags []string `json:"tags"`

	// LastModifiedTime is the time that the song was modified.
	LastModifiedTime time.Time `json:"-"`
}

type SongOrErr struct {
	*Song
	Err error
}

// PlayDump is used when dumping data.
type PlayDump struct {
	// Song entity's key ID from Datastore.
	SongID string `json:"songId"`

	// Play information.
	Play Play `json:"play"`
}

type PlayArray []Play

func (a PlayArray) Len() int           { return len(a) }
func (a PlayArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a PlayArray) Less(i, j int) bool { return a[i].StartTime.Before(a[j].StartTime) }

// ClientConfig holds configuration details shared across client binaries.
type ClientConfig struct {
	// ServerURL contains the App Engine server URL.
	ServerURL string `json:"serverUrl"`
	// Username contains an HTTP basic auth username.
	Username string `json:"username"`
	// Password contains an HTTP basic auth password.
	Password string `json:"password"`
}

// BasicAuthInfo contains information used for validating HTTP basic authentication.
type BasicAuthInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ServerConfig holds the App Engine server's configuration.
type ServerConfig struct {
	// GoogleUsers contains email addresses of Google accounts allowed to access
	// the web interface.
	GoogleUsers []string `json:"googleUsers"`

	// BasicAuthUsers contains for accounts using HTTP basic authentication
	// (i.e. command-line tools or the Android client).
	BasicAuthUsers []BasicAuthInfo `json:"basicAuthUsers"`

	// SongBucket contains the name of the Google Cloud Storage bucket holding song files.
	SongBucket string `json:"songBucket"`
	// CoverBucket contains the name of the Google Cloud Storage bucket holding album cover images.
	CoverBucket string `json:"coverBucket"`

	// SongBaseURL contains the slash-terminated URL under which song files are stored.
	// Exactly one of SongBucket and SongBaseURL must be set.
	SongBaseURL string `json:"songBaseUrl"`
	// CoverBaseURL contains the slash-terminated URL under which album cover images are stored.
	// Exactly one of CoverBucket and CoverBaseURL must be set.
	CoverBaseURL string `json:"coverBaseUrl"`

	// ForceUpdateFailures is set by tests to indicate that failure be reported
	// for all user data updates (ratings, tags, plays). Ignored for non-development servers.
	ForceUpdateFailures bool `json:"forceUpdateFailures"`
}
