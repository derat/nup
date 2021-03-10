// Copyright 2021 Daniel Erat.
// All rights reserved.

package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

const (
	// TestUsername and TestPassword are accepted for basic HTTP authentication
	// by development servers.
	TestUsername = "testuser"
	TestPassword = "testpass"
)

// ClientConfig holds configuration details shared across client binaries.
type ClientConfig struct {
	// ServerURL contains the App Engine server URL.
	ServerURL string `json:"serverUrl"`
	// Username contains an HTTP basic auth username.
	Username string `json:"username"`
	// Password contains an HTTP basic auth password.
	Password string `json:"password"`
}

// LoadClientConfig loads a JSON ClientConfig from the file at p.
// dst must be either a *ClientConfig or a pointer to a struct that embeds ClientConfig.
func LoadClientConfig(p string, dst interface{}) error {
	if err := loadJSON(p, dst); err != nil {
		return err
	}

	// Go doesn't let us cast dst to (possibly-embedded) ClientConfig, so use this
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
func (cfg *ClientConfig) GetURL(path string) *url.URL {
	u, _ := url.Parse(cfg.ServerURL) // checked in LoadClientConfig()
	if u.Path == "" {
		u.Path = "/"
	}
	u.Path = filepath.Join(u.Path, path)
	return u
}

// checkServerURL returns an error if cfg.ServerURL is unset or malformed.
func (cfg *ClientConfig) checkServerURL() error {
	if cfg.ServerURL == "" {
		return errors.New("serverUrl not set")
	}
	if _, err := url.Parse(cfg.ServerURL); err != nil {
		return fmt.Errorf("bad serverUrl %q: %v", cfg.ServerURL, err)
	}
	return nil
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

// LoadServerConfig loads a JSON ServerConfig from the file at p.
func LoadServerConfig(p string) (*ServerConfig, error) {
	var cfg ServerConfig
	if err := loadJSON(p, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// loadJSON opens the specified file and unmarshals it into out.
func loadJSON(path string, out interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	if err = d.Decode(out); err != nil {
		return err
	}
	return nil
}
