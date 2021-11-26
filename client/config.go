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

// Config holds configuration details shared across client binaries.
type Config struct {
	// ServerURL contains the App Engine server URL.
	ServerURL string `json:"serverUrl"`
	// Username contains an HTTP basic auth username.
	Username string `json:"username"`
	// Password contains an HTTP basic auth password.
	Password string `json:"password"`
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
