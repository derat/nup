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
	"os"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/user"
)

const (
	DatastoreKind    = "Config"
	DatastoreKeyName = "active"
)

// SavedConfig is used to store a JSON-marshaled Config in Datastore.
type SavedConfig struct {
	JSON string `datastore:"json"`
}

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
	// TODO: This should probably be called MinStars instead to make it clear
	// that it's different from the Song.Rating field with its values in [0.0, 1.0].
	// Or maybe just use stars everywhere? ¯\_(ツ)_/¯
	MinRating int `json:"minRating"`
	// Unrated specifies that only unrated songs should be returned.
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
	// OrderByLastPlayed specifies that songs should be ordered by the last time
	// they were played (in ascending order).
	OrderByLastPlayed bool `json:"orderByLastPlayed"`
	// MaxPlays specifies the maximum number of times that each song has been played.
	// -1 specifies that there is no restriction on the number of plays.
	MaxPlays int `json:"maxPlays"`
	// FirstTrack specifies that only albums' first tracks should be returned.
	FirstTrack bool `json:"firstTrack"`
	// Shuffle specifies that the returned songs should be shuffled.
	Shuffle bool `json:"shuffle"`
	// Play specifies that returned songs should be played automatically.
	// The current playlist is replaced.
	Play bool `json:"play"`
}

func (p *SearchPreset) UnmarshalJSON(data []byte) error {
	// Set defaults for unspecified fields.
	*p = SearchPreset{MaxPlays: -1}

	// Use json.Decoder rather than json.Unmarshal so we can reject unknown fields.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	type searchPreset *SearchPreset // avoid stack overflow
	return dec.Decode(searchPreset(p))
}

// Config holds the App Engine server's configuration.
type Config struct {
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
	// This is used for testing.
	// Exactly one of SongBucket and SongBaseURL must be set.
	SongBaseURL string `json:"songBaseUrl"`
	// CoverBaseURL contains the slash-terminated URL under which album cover images are stored.
	// This is used for testing.
	// Exactly one of CoverBucket and CoverBaseURL must be set.
	CoverBaseURL string `json:"coverBaseUrl"`

	// Presets describes search presets for the web interface.
	Presets []SearchPreset `json:"presets"`
}

// Parse unmarshals jsonData, validates it, and returns the resulting config.
func Parse(jsonData []byte) (*Config, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(jsonData))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}

	cleanBaseURL(&cfg.SongBaseURL)
	haveSongBucket := len(cfg.SongBucket) > 0
	haveSongURL := len(cfg.SongBaseURL) > 0
	if (haveSongBucket && haveSongURL) || !(haveSongBucket || haveSongURL) {
		return nil, errors.New("exactly one of SongBucket and SongBaseURL must be set")
	}

	cleanBaseURL(&cfg.CoverBaseURL)
	haveCoverBucket := len(cfg.CoverBucket) > 0
	haveCoverURL := len(cfg.CoverBaseURL) > 0
	if (haveCoverBucket && haveCoverURL) || !(haveCoverBucket || haveCoverURL) {
		return nil, errors.New("exactly one of CoverBucket and CoverBaseURL must be set")
	}

	return &cfg, nil
}

// Load attempts to load the server's config from various locations.
// ctx must be an App Engine context.
func Load(ctx context.Context) (*Config, error) {
	b, err := func() ([]byte, error) {
		// Tests can override the config file via a NUP_CONFIG environment variable.
		if b := []byte(os.Getenv("NUP_CONFIG")); len(b) != 0 {
			return b, nil
		}
		// Try to get the JSON data from Datastore by default.
		var saved SavedConfig
		key := datastore.NewKey(ctx, DatastoreKind, DatastoreKeyName, 0, nil)
		if err := datastore.Get(ctx, key, &saved); err != nil {
			return nil, err
		}
		return []byte(saved.JSON), nil
	}()

	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Auth checks that r is authorized in cfg via either HTTP basic authentication or Google
// authentication. A username or email address that can be used in logging is returned if found,
// even if the the request is unauthorized. If allowCron is true, requests that were initiated via
// cron are also permitted.
func (cfg *Config) Auth(r *http.Request, allowCron bool) (ok bool, username string) {
	// https://cloud.google.com/appengine/docs/standard/go/scheduling-jobs-with-cron-yaml#validating_cron_requests
	if allowCron && r.Header.Get("X-Appengine-Cron") == "true" {
		return true, "cron"
	}

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
