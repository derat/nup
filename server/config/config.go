// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package config contains types and constants related to server configuration.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	JSON string `datastore:"json,noindex"`
}

// User contains information about a user allowed to access the server.
type User struct {
	// Email contains an email address for Google authentication.
	Email string `json:"email"`

	// Username contains a username for HTTP basic auth.
	Username string `json:"username"`
	// Password contains a password for HTTP basic auth.
	Password string `json:"password"`

	// Admin is true if this user should have elevated permissions.
	// This should only be set for the HTTP basic auth account used by the nup command-line executable.
	Admin bool `json:"admin"`
	// Guest is true if this user should have reduced permissions.
	// This can be set for HTTP basic auth accounts used by the Android app.
	Guest bool `json:"guest"`

	// Presets contains custom search presets for this user.
	// If empty, Config.Presets will be used instead.
	Presets []SearchPreset `json:"presets"`
}

// Type returns u's type.
func (u *User) Type() UserType {
	switch {
	case u.Guest:
		return GuestUser
	case u.Admin:
		return AdminUser
	default:
		return NormalUser
	}
}

// UserType describes the level of access granted to a user.
// 0 represents a non-user.
type UserType uint32

const (
	// NormalUser indicates a non-admin, non-guest user.
	NormalUser UserType = 1 << iota
	// AdminUser indicates a user with its Admin field set to true.
	AdminUser
	// GuestUser indicates a user with its Guest field set to true.
	GuestUser
	// CronUser indicates a request issued by App Engine cron jobs.
	CronUser
)

// SearchPreset specifies a search preset to display.
type SearchPreset struct {
	// Name contains a human-readable name describing the preset.
	Name string `json:"name"`
	// Tags contains a space-separated tag expression, e.g. "guitar -banjo".
	Tags string `json:"tags"`
	// MinRating contains a minimum rating as number of stars in [1, 5].
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
	// Users contains information about users who can access the server.
	Users []User `json:"users"`

	// SongBucket contains the name of the Google Cloud Storage bucket holding song files.
	SongBucket string `json:"songBucket,omitempty"`
	// CoverBucket contains the name of the Google Cloud Storage bucket holding album cover images.
	CoverBucket string `json:"coverBucket,omitempty"`

	// SongBaseURL contains the slash-terminated URL under which song files are stored.
	// This is used for testing.
	// Exactly one of SongBucket and SongBaseURL must be set.
	SongBaseURL string `json:"songBaseUrl,omitempty"`
	// CoverBaseURL contains the slash-terminated URL under which album cover images are stored.
	// This is used for testing.
	// Exactly one of CoverBucket and CoverBaseURL must be set.
	CoverBaseURL string `json:"coverBaseUrl,omitempty"`

	// Presets contains default search presets.
	Presets []SearchPreset `json:"presets"`

	// Minify describes whether the server should minify JavaScript, HTML, and CSS code
	// and bundle all JavaScript code into a single file. Defaults to true if unset.
	Minify *bool `json:"minify"`

	// MaxGuestSongRequestsPerHour contains the maximum rate at which each guest
	// user can send requests to the /song endpoint. Unlimited if 0 or negative.
	MaxGuestSongRequestsPerHour int `json:"maxGuestSongRequestsPerHour,omitempty"`
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

	var admin bool
	for i, u := range cfg.Users {
		switch {
		case (u.Email != "") == (u.Username != ""):
			return nil, fmt.Errorf("user %d has email %q and username %q; exactly one should be set", i, u.Email, u.Username)
		case u.Email != "" && u.Password != "":
			return nil, fmt.Errorf("user %d has email %q but non-empty password", i, u.Email)
		case u.Username != "" && u.Password == "":
			return nil, fmt.Errorf("user %d has username %q but empty password", i, u.Username)
		case u.Admin && u.Guest:
			return nil, fmt.Errorf("user %d is both admin and guest", i)
		}
		if u.Admin {
			admin = true
		}
	}
	if !admin {
		return nil, errors.New("no admin user")
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

// GetUser returns information about the user who sent r.
// If the request was unauthenticated or the user is not listed in cfg.Users, utype is 0.
// A username or email address that can be used in logging is returned if possible,
// even if the the request is not from a known user.
func (cfg *Config) GetUser(r *http.Request) (name string, utype UserType) {
	// https://cloud.google.com/appengine/docs/standard/go/scheduling-jobs-with-cron-yaml#validating_cron_requests
	if r.Header.Get("X-Appengine-Cron") == "true" {
		return "cron", CronUser
	}
	if username, password, ok := r.BasicAuth(); ok {
		for _, u := range cfg.Users {
			if username == u.Username && password == u.Password {
				return username, u.Type()
			}
		}
		return username, 0
	}
	if gu := user.Current(appengine.NewContext(r)); gu != nil {
		for _, u := range cfg.Users {
			if gu.Email == u.Email {
				return gu.Email, u.Type()
			}
		}
		return gu.Email, 0
	}
	return "", 0
}

// GetPresets returns search presets for the user who issued r.
func (cfg *Config) GetPresets(r *http.Request) []SearchPreset {
	// Make sure that this is from a known user.
	name, utype := cfg.GetUser(r)
	if utype&(GuestUser|NormalUser|AdminUser) == 0 {
		return nil
	}
	// Check if the user has custom presets defined.
	for _, u := range cfg.Users {
		if u.Email == name || u.Username == name {
			if len(u.Presets) > 0 {
				return u.Presets
			}
			break
		}
	}
	// Fall back to the default presets.
	return cfg.Presets
}

// cleanBaseURL appends a trailing slash to u if not already present.
// Does nothing if u is empty.
func cleanBaseURL(u *string) {
	if len(*u) > 0 && (*u)[len(*u)-1] != '/' {
		*u += "/"
	}
}
