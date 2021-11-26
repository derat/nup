// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/datastore"

	"github.com/derat/nup/server/auth"
)

const (
	// Config file path relative to base directory.
	configPath = "config.json"

	// Datastore kind and ID for storing the server config for testing.
	configKind  = "ServerConfig"
	configKeyID = "config"
)

// searchPreset specifies a search preset to display in the web interface.
type searchPreset struct {
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

// config holds the App Engine server's configuration.
type config struct {
	// GoogleUsers contains email addresses of Google accounts allowed to access
	// the web interface.
	GoogleUsers []string `json:"googleUsers"`

	// BasicAuthUsers contains for accounts using HTTP basic authentication
	// (i.e. command-line tools or the Android client).
	BasicAuthUsers []auth.BasicAuthInfo `json:"basicAuthUsers"`

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
	Presets []searchPreset `json:"presets"`

	// ForceUpdateFailures is set by tests to indicate that failure be reported
	// for all user data updates (ratings, tags, plays). Ignored for non-development servers.
	ForceUpdateFailures bool `json:"forceUpdateFailures"`
}

// Singleton loaded from disk by loadConfig().
var baseCfg *config

func addTestUserToConfig(cfg *config) {
	cfg.BasicAuthUsers = append(cfg.BasicAuthUsers, auth.BasicAuthInfo{
		Username: auth.TestUsername,
		Password: auth.TestPassword,
	})
}

// cleanBaseURL appends a trailing slash to u if not already present.
// Does nothing if u is empty.
func cleanBaseURL(u *string) {
	if len(*u) > 0 && (*u)[len(*u)-1] != '/' {
		*u += "/"
	}
}

// loadConfig loads the server configuration from disk.
// It should be called once at the start of main().
func loadConfig() error {
	f, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var cfg config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
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
		addTestUserToConfig(baseCfg)
	}
	return nil
}

func testConfigKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, configKind, configKeyID, 0, nil)
}

// getConfig returns the currently-in-use config.
func getConfig(ctx context.Context) *config {
	if appengine.IsDevAppServer() {
		var testCfg config
		if err := datastore.Get(ctx, testConfigKey(ctx), &testCfg); err == nil {
			return &testCfg
		} else if err != datastore.ErrNoSuchEntity {
			panic(err)
		}
	}

	if baseCfg == nil {
		panic("loadConfig() not called")
	}
	return baseCfg
}

func saveTestConfig(ctx context.Context, cfg *config) {
	if _, err := datastore.Put(ctx, testConfigKey(ctx), cfg); err != nil {
		panic(err)
	}
}

func clearTestConfig(ctx context.Context) {
	if err := datastore.Delete(ctx, testConfigKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
		panic(err)
	}
}
