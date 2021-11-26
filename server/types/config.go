// Copyright 2021 Daniel Erat.
// All rights reserved.

package types

import (
	"encoding/json"
	"os"
)

const (
	// TestUsername and TestPassword are accepted for basic HTTP authentication
	// by development servers.
	TestUsername = "testuser"
	TestPassword = "testpass"
)

// BasicAuthInfo contains information used for validating HTTP basic authentication.
type BasicAuthInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Preset specifies a search preset to display in the web interface.
type Preset struct {
	// Name contains a human-readable name to display in the interface.
	Name string `json:"name"`
	// Tags contains a space-separated tag expression, e.g. "guitar -banjo".
	Tags string `json:"tags"`
	// MinRating contains a minimum rating as number of stars in [1, 5].
	// 0 is treated the same as 1.
	MinRating int `json:"minRating"`
	// Unrated indicates that only unrated songs should be returned.
	Unrated bool `json:"unrated"`
	// FirstPlayed contains a 0-based index into the first-played dropdown:
	// [empty], one day, one week, one month, three months, six months, one year, three years
	FirstPlayed int `json:"firstPlayed"`
	// LastPlayed contains a 0-based index into the last-played dropdown:
	// [empty], one day, one week, one month, three months, six months, one year, three years
	LastPlayed int `json:"lastPlayed"`
	// FirstTrack indicates that only albums' first tracks should be returned.
	FirstTrack bool `json:"firstTrack"`
	// Shuffle indicates that the returned songs should be shuffled.
	Shuffle bool `json:"shuffle"`
	// Play indicates that returned songs should be played automatically.
	Play bool `json:"play"`
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

	// Presets describes search presets for the web interface.
	Presets []Preset `json:"presets"`

	// ForceUpdateFailures is set by tests to indicate that failure be reported
	// for all user data updates (ratings, tags, plays). Ignored for non-development servers.
	ForceUpdateFailures bool `json:"forceUpdateFailures"`
}

// LoadServerConfig loads a JSON ServerConfig from the file at p.
func LoadServerConfig(p string) (*ServerConfig, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg ServerConfig
	err = json.NewDecoder(f).Decode(&cfg)
	return &cfg, err
}
