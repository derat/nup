// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package config contains types and constants related to server configuration.
package config

import (
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
	// TestUsername and TestPassword are accepted for basic HTTP authentication by development servers.
	TestUsername = "testuser"
	TestPassword = "testpass"

	// WebDriverCookie is set by web tests to skip authentication.
	WebDriverCookie = "webdriver"

	// Config file path relative to base directory.
	configPath = "config.json"

	// Datastore kind and ID for storing the server config for testing.
	configKind  = "ServerConfig"
	configKeyID = "config"
)

// Singleton loaded from disk by LoadConfig().
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

	// ForceUpdateFailures is set by tests to indicate that failure be reported
	// for all user data updates (ratings, tags, plays). Ignored for non-development servers.
	ForceUpdateFailures bool `json:"forceUpdateFailures"`
}

// HasAllowedGoogleAuth checks whether ctx contains credentials for an authorized Google user.
func (cfg *Config) HasAllowedGoogleAuth(ctx context.Context) (email string, allowed bool) {
	u := user.Current(ctx)
	if u == nil {
		return "", false
	}

	for _, e := range cfg.GoogleUsers {
		if u.Email == e {
			return u.Email, true
		}
	}
	return u.Email, false
}

// HasAllowedBasicAuth checks whether r is authorized via HTTP basic
// authentication with a username and password. If basic auth was used in r,
// the username return value is set regardless of the user is actually allowed or not.
func (cfg *Config) HasAllowedBasicAuth(r *http.Request) (username string, allowed bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", false
	}
	for _, u := range cfg.BasicAuthUsers {
		if username == u.Username && password == u.Password {
			return username, true
		}
	}
	return username, false
}

func (cfg *Config) AddTestUser() {
	cfg.BasicAuthUsers = append(cfg.BasicAuthUsers, BasicAuthInfo{
		Username: TestUsername,
		Password: TestPassword,
	})
}

// RequestHasWebDriverCookie returns true if r contains a special cookie set by browser
// tests that use WebDriver.
func RequestHasWebDriverCookie(r *http.Request) bool {
	if _, err := r.Cookie(WebDriverCookie); err != nil {
		return false
	}
	return true
}

// cleanBaseURL appends a trailing slash to u if not already present.
// Does nothing if u is empty.
func cleanBaseURL(u *string) {
	if len(*u) > 0 && (*u)[len(*u)-1] != '/' {
		*u += "/"
	}
}

// LoadConfig loads the server configuration from disk.
// It should be called once at the start of main().
func LoadConfig() error {
	f, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var cfg Config
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return err
	}
	baseCfg = &cfg

	cleanBaseURL(&baseCfg.SongBaseURL)
	haveSongBucket := len(baseCfg.SongBucket) > 0
	haveSongURL := len(baseCfg.SongBaseURL) > 0
	if (haveSongBucket && haveSongURL) || !(haveSongBucket || haveSongURL) {
		return errors.New("exactly one of SongBucket and SongBaseURL must be set")
	}

	cleanBaseURL(&baseCfg.CoverBaseURL)
	haveCoverBucket := len(baseCfg.CoverBucket) > 0
	haveCoverURL := len(baseCfg.CoverBaseURL) > 0
	if (haveCoverBucket && haveCoverURL) || !(haveCoverBucket || haveCoverURL) {
		return errors.New("exactly one of CoverBucket and CoverBaseURL must be set")
	}

	if appengine.IsDevAppServer() {
		baseCfg.AddTestUser()
	}
	return nil
}

func testConfigKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, configKind, configKeyID, 0, nil)
}

// GetConfig returns the currently-in-use Config.
func GetConfig(ctx context.Context) *Config {
	if appengine.IsDevAppServer() {
		var testCfg Config
		if err := datastore.Get(ctx, testConfigKey(ctx), &testCfg); err == nil {
			return &testCfg
		} else if err != datastore.ErrNoSuchEntity {
			panic(err)
		}
	}

	if baseCfg == nil {
		panic("LoadConfig() not called")
	}
	return baseCfg
}

func SaveTestConfig(ctx context.Context, cfg *Config) {
	if _, err := datastore.Put(ctx, testConfigKey(ctx), cfg); err != nil {
		panic(err)
	}
}

func ClearTestConfig(ctx context.Context) {
	if err := datastore.Delete(ctx, testConfigKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
		panic(err)
	}
}