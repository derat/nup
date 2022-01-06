// Copyright 2021 Daniel Erat.
// All rights reserved.

// Package client continues functionality shared across client binaries.
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// Config holds configuration details for the nup client executable.
type Config struct {
	// ServerURL contains the App Engine server URL.
	ServerURL string `json:"serverUrl"`
	// Username contains an HTTP basic auth username.
	Username string `json:"username"`
	// Password contains an HTTP basic auth password.
	Password string `json:"password"`

	// CoverDir is the base directory containing cover art.
	CoverDir string `json:"coverDir"`
	// MusicDir is the base directory containing song files.
	MusicDir string `json:"musicDir"`
	// LastUpdateInfoFile is the path to a JSON file storing info about the last update.
	// The file will be created if it does not already exist.
	LastUpdateInfoFile string `json:"lastUpdateInfoFile"`
	// ComputeGain indicates whether the mp3gain program should be used to compute per-song
	// and per-album gain information so that volume can be normalized during playback.
	ComputeGain bool `json:"computeGain"`
	// ArtistRewrites is a map from original ID3 tag artist names to replacement names that should
	// be used for updates. This can be used to fix incorrectly-tagged files without needing to
	// reupload them.
	ArtistRewrites map[string]string `json:"artistRewrites"`
}

// LoadConfig loads a JSON-marshaled Config from the file at p.
// dst must be either a *Config or a pointer to a struct that embeds Config.
func LoadConfig(p string, dst interface{}) error {
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	if err = d.Decode(dst); err != nil {
		return err
	}

	// Go doesn't let us cast dst to (possibly-embedded) Config, so use this
	// dumb hack to check that the ServerURL field is set properly.
	type clientConfig interface{ checkServerURL() error }
	cfg, ok := dst.(clientConfig)
	if !ok {
		return errors.New("invalid config type")
	} else if err := cfg.checkServerURL(); err != nil {
		return err
	}

	return nil
}

// GetURL appends path to ServerURL. Query params should not be included.
func (cfg *Config) GetURL(path string) *url.URL {
	u, _ := url.Parse(cfg.ServerURL) // checked in LoadConfig()
	if u.Path == "" {
		u.Path = "/"
	}
	u.Path = filepath.Join(u.Path, path)
	return u
}

// checkServerURL returns an error if cfg.ServerURL is unset or malformed.
func (cfg *Config) checkServerURL() error {
	if cfg.ServerURL == "" {
		return errors.New("serverUrl not set")
	}
	if _, err := url.Parse(cfg.ServerURL); err != nil {
		return fmt.Errorf("bad serverUrl %q: %v", cfg.ServerURL, err)
	}
	return nil
}
