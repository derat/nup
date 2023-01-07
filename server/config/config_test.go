// Copyright 2023 Daniel Erat.
// All rights reserved.

package config

import (
	"net/http"
	"reflect"
	"testing"
)

func TestGetUserType(t *testing.T) {
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
		utype      UserType
		name       string
	}{
		{"user", "upass", NormalUser, "user"},
		{"admin", "apass", AdminUser, "admin"},
		{"guest", "gpass", GuestUser, "guest"},
		{"user", "", 0, "user"},
		{"user", "bogus", 0, "user"},
		{"user", "apass", 0, "user"},
		{"", "upass", 0, ""},
		{"", "", 0, ""},
	} {
		if utype, name := cfg.GetUserType(makeReq(t, tc.user, tc.pass)); utype != tc.utype || name != tc.name {
			t.Errorf("GetUserType for %q/%q returned %v and %q; want %v and %q",
				tc.user, tc.pass, utype, name, tc.utype, tc.name)
		}
	}
}

func TestGetUser(t *testing.T) {
	cfg := Config{
		Users: []User{
			{Username: "user", Password: "upass", ExcludedTags: []string{"foo"}},
			{Username: "guest", Password: "gpass", Guest: true},
		},
	}

	for _, tc := range []struct {
		user, pass string
		want       *User
	}{
		{"user", "upass", &User{Username: "user", ExcludedTags: []string{"foo"}}},
		{"guest", "gpass", &User{Username: "guest", Guest: true}},
		{"user", "", nil},
		{"", "upass", nil},
		{"", "", nil},
	} {
		if user, name := cfg.GetUser(makeReq(t, tc.user, tc.pass)); !reflect.DeepEqual(user, tc.want) || name != tc.user {
			t.Errorf("GetUser for %q/%q returned %v and %q; want %v and %q", tc.user, tc.pass, user, name, tc.want, tc.user)
		}
	}

	if cfg.Users[0].Password != "upass" || cfg.Users[1].Password != "gpass" {
		t.Error("Original passwords were modified")
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
