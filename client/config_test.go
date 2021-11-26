// Copyright 2021 Daniel Erat.
// All rights reserved.

package client

import "testing"

func TestConfig_ServerURL(t *testing.T) {
	for _, tc := range []struct{ server, path, want string }{
		{"https://www.example.com", "cmd", "https://www.example.com/cmd"},
		{"https://www.example.com", "/cmd", "https://www.example.com/cmd"},
		{"https://www.example.com/", "cmd", "https://www.example.com/cmd"},
		{"https://www.example.com/", "/cmd", "https://www.example.com/cmd"},
		{"https://www.example.com/base", "cmd", "https://www.example.com/base/cmd"},
		{"https://www.example.com/base", "/cmd", "https://www.example.com/base/cmd"},
		{"https://www.example.com/base/", "cmd", "https://www.example.com/base/cmd"},
		{"https://www.example.com/base/", "/cmd", "https://www.example.com/base/cmd"},
	} {
		cfg := Config{ServerURL: tc.server}
		if got := cfg.GetURL(tc.path); got.String() != tc.want {
			t.Errorf("ClientConfig{ServerURL: %q}.GetURL(%q) = %q; want %q",
				tc.server, tc.path, tc.want, got.String())
		}
	}
}
