// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package cloudutil provides common server-related functionality.
package cloudutil

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
)

const (
	// TestUsername and TestPassword are accepted for basic HTTP authentication
	// by development servers.
	TestUsername = "testuser"
	TestPassword = "testpass"
)

func ServerURL(baseURL, path string) (*url.URL, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.Path += path
	return u, nil
}

func ReadJSON(path string, out interface{}) error {
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
