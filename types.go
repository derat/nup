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

	// Lowercase versions used for searching and sorting.
	ArtistLower string `json:"-"`
	TitleLower  string `json:"-"`
	AlbumLower  string `json:"-"`

	// Words from ArtistLower, TitleLower, and AlbumLower used for searching.
	Keywords []string `json:"-"`

	// Opaque ID uniquely identifying the album (generally, a MusicBrainz
	// release ID taken from a "MusicBrainz Album Id" ID3v2 tag).
	AlbumId string `json:"albumId,omitempty"`

	// Opaque ID uniquely identifying the recording (generally, the MusicBrainz
	// ID corresponding to the MusicBrainz recording entity, taken from a UFID
	// ID3v2 tag). Only used to find cover art for non-album tracks.
	RecordingId string `datastore:"-" json:"-"`

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

type SongOrErr struct {
	*Song
	Err error
}

// PlayDump is used when dumping data.
type PlayDump struct {
	// Song entity's key ID from Datastore.
	SongId string `json:"songId"`

	// Play information.
	Play Play `json:"play"`
}

type PlayArray []Play

func (a PlayArray) Len() int           { return len(a) }
func (a PlayArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a PlayArray) Less(i, j int) bool { return a[i].StartTime.Before(a[j].StartTime) }

// ClientConfig holds configuration details shared across client binaries.
type ClientConfig struct {
	ServerUrl string
	Username  string
	Password  string
}

type BasicAuthInfo struct {
	Username string
	Password string
}

// ClientConfig holds the App Engine server's configuration.
type ServerConfig struct {
	// Email addresses of Google users allowed to use the server.
	GoogleUsers []string

	// Credentials of accounts using HTTP basic authentication.
	BasicAuthUsers []BasicAuthInfo

	// Names of the Cloud Storage buckets where song and cover files are stored.
	SongBucket  string
	CoverBucket string

	// Base URLs where song and cover files are stored.
	// Exactly one of *Bucket and *BaseUrl must be set.
	SongBaseUrl  string
	CoverBaseUrl string

	// Should songs, query results, and tags be cached?
	CacheSongs   bool
	CacheQueries bool
	CacheTags    bool

	// Should datastore (rather than memcache) be used for caching query results and tags?
	UseDatastoreForCache bool

	// Should failure be reported for all user data updates (ratings, tags, plays)?
	// Ignored for non-development servers.
	ForceUpdateFailures bool
}
