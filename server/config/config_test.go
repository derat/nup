// Copyright 2023 Daniel Erat.
// All rights reserved.

package config

import (
	"net/http"
	"reflect"
	"testing"
)

func TestGetUser(t *testing.T) {
	cfg := Config{
		Users: []User{
			{Username: "user", Password: "upass"},
			{Username: "admin", Password: "apass", Admin: true},
			{Username: "guest", Password: "gpass", Guest: true},
		},
	}

	// Authentication is tested in more detail in server/http_test.go.
	for _, tc := range []struct {
		user, pass string
		name       string
		utype      UserType
	}{
		{"user", "upass", "user", NormalUser},
		{"admin", "apass", "admin", AdminUser},
		{"guest", "gpass", "guest", GuestUser},
		{"user", "", "user", 0},
		{"user", "bogus", "user", 0},
		{"user", "apass", "user", 0},
		{"", "upass", "", 0},
		{"", "", "", 0},
	} {
		if name, utype := cfg.GetUser(makeReq(t, tc.user, tc.pass)); name != tc.name || utype != tc.utype {
			t.Errorf("GetUser for %q/%q returned %q and %v; want %q and %v",
				tc.user, tc.pass, name, utype, tc.name, tc.utype)
		}
	}
}

func TestGetPresets(t *testing.T) {
	defPresets := []SearchPreset{{Name: "default"}}
	guestPresets := []SearchPreset{{Name: "guest"}}
	cfg := Config{
		Users: []User{
			{Username: "user", Password: "upass"},
			{Username: "guest", Password: "gpass", Guest: true, Presets: guestPresets},
		},
		Presets: defPresets,
	}

	for _, tc := range []struct {
		user, pass string
		want       []SearchPreset
	}{
		{"user", "upass", defPresets},
		{"guest", "gpass", guestPresets},
		{"user", "", nil},
		{"", "upass", nil},
		{"", "", nil},
	} {
		if got := cfg.GetPresets(makeReq(t, tc.user, tc.pass)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("GetPresets for %q/%q returned %v; want %v", tc.user, tc.pass, got, tc.want)
		}
	}
}

// makeReq returns an *http.Request with the supplied HTTP basic auth credentials.
func makeReq(t *testing.T, user, pass string) *http.Request {
	req, err := http.NewRequest("GET", "https://example.org", nil)
	if err != nil {
		t.Fatal("NewRequest failed: ", err)
	}
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	return req
}
