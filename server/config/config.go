// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package config contains types and constants related to server configuration.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/user"
)

// Singleton parsed by LoadConfig().
var baseCfg *Config

// BasicAuthInfo contains information used for validating HTTP basic authentication.
type BasicAuthInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// SearchPreset specifies a search preset to display in the web interface.
type SearchPreset struct {
	// Name contains a human-readable name to display in the interface.
	Name string `json:"name"`
	// Tags contains a space-separated tag expression, e.g. "guitar -banjo".
	Tags string `json:"tags"`
	// MinRating contains a minimum rating as number of stars in [1, 5].
	// 0 is equivalent to 1, i.e. any rating is accepted.
	MinRating int `json:"minRating"`
	// Unrated indicates that only unrated songs should be returned.
	Unrated bool `json:"unrated"`
	// FirstPlayed limits results to songs first played within the given interval:
	//   0 - no restriction
	//   1 - last day
	//   2 - last week
	//   3 - last month
	//   4 - last three months
	//   5 - last six months
	//   6 - last year
	//   7 - last three years
	//   8 - last five years
	FirstPlayed int `json:"firstPlayed"`
	// LastPlayed limits results to songs last played before the given interval.
	// See FirstPlayed for values.
	LastPlayed int `json:"lastPlayed"`
	// FirstTrack indicates that only albums' first tracks should be returned.
	FirstTrack bool `json:"firstTrack"`
	// Shuffle indicates that the returned songs should be shuffled.
	Shuffle bool `json:"shuffle"`
	// Play indicates that returned songs should be played automatically.
	// The current playlist is replaced.
	Play bool `json:"play"`
}

// Config holds the App Engine server's configuration.
type Config struct {
	// ProjectID is the Google Cloud project ID.
	ProjectID string `json:"projectId"`

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
	Presets []SearchPreset `json:"presets"`
}

// Auth checks that r is authorized in cfg via either HTTP basic authentication or Google
// authentication. A username or email address that can be used in logging is returned if found,
// even if the the request is unauthorized.
func (cfg *Config) Auth(r *http.Request) (ok bool, username string) {
	if username, password, ok := r.BasicAuth(); ok {
		for _, u := range cfg.BasicAuthUsers {
			if username == u.Username && password == u.Password {
				return true, username
			}
		}
		return false, username
	}

	if u := user.Current(appengine.NewContext(r)); u != nil {
		for _, e := range cfg.GoogleUsers {
			if u.Email == e {
				return true, u.Email
			}
		}
		return false, u.Email
	}

	return false, ""
}

// cleanBaseURL appends a trailing slash to u if not already present.
// Does nothing if u is empty.
func cleanBaseURL(u *string) {
	if len(*u) > 0 && (*u)[len(*u)-1] != '/' {
		*u += "/"
	}
}

// LoadConfig unmarshals jsonData, validates it, and sets baseCfg.
// It should be called once at the start of main().
func LoadConfig(jsonData []byte) error {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(jsonData))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return err
	}

	cleanBaseURL(&cfg.SongBaseURL)
	haveSongBucket := len(cfg.SongBucket) > 0
	haveSongURL := len(cfg.SongBaseURL) > 0
	if (haveSongBucket && haveSongURL) || !(haveSongBucket || haveSongURL) {
		return errors.New("exactly one of SongBucket and SongBaseURL must be set")
	}

	cleanBaseURL(&cfg.CoverBaseURL)
	haveCoverBucket := len(cfg.CoverBucket) > 0
	haveCoverURL := len(cfg.CoverBaseURL) > 0
	if (haveCoverBucket && haveCoverURL) || !(haveCoverBucket || haveCoverURL) {
		return errors.New("exactly one of CoverBucket and CoverBaseURL must be set")
	}

	baseCfg = &cfg
	return nil
}

// GetConfig returns the currently-in-use Config.
func GetConfig(ctx context.Context) (*Config, error) {
	if baseCfg == nil {
		return nil, errors.New("LoadConfig not called")
	}
	return baseCfg, nil
}
