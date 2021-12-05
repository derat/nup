// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/derat/nup/server/config"
	"google.golang.org/appengine/v2/aetest"
	"google.golang.org/appengine/v2/user"
)

// This test also exercises a lot of code from the config page, but the aetest package is slow
// (4+ seconds to start dev_appserver.py) so I'm minimizing the number of places I use it.
func TestAddHandler(t *testing.T) {
	const (
		email1   = "user1@example.org"
		email2   = "user2@example.org"
		badEmail = "bad@example.org"

		user1 = "user1"
		pass1 = "pass1"

		user2 = "user2"
		pass2 = "pass2"

		badUser = "baduser"
		badPass = "badpass"
	)

	// Start dev_appserver.py.
	inst, err := aetest.NewInstance(&aetest.Options{SuppressDevAppServerLog: false})
	if err != nil {
		t.Fatal("Failed starting dev_appserver: ", err)
	}
	defer inst.Close()

	// Write a config and load it.
	origCfg := &config.Config{
		BasicAuthUsers: []config.BasicAuthInfo{{user1, pass1}, {user2, pass2}},
		GoogleUsers:    []string{email1, email2},
		SongBucket:     "test-songs",
		CoverBucket:    "test-covers",
	}
	b, err := json.Marshal(origCfg)
	if err != nil {
		t.Fatal("Failed marshaling config: ", err)
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if err := ioutil.WriteFile(p, b, 0644); err != nil {
		t.Fatal("Failed writing config: ", err)
	}
	if err := config.LoadConfig(p); err != nil {
		t.Fatal("Failed loading config: ", err)
	}

	// Set up some HTTP handlers.
	var lastMethod, lastPath string // method and path from last accepted request
	handleReq := func(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
		if !reflect.DeepEqual(cfg, origCfg) {
			t.Fatalf("Got config %+v; want %+v", cfg, origCfg)
		}
		lastMethod = r.Method
		lastPath = r.URL.Path
	}
	addHandler("/", http.MethodGet, redirectUnauth, handleReq)
	addHandler("/get", http.MethodGet, rejectUnauth, handleReq)
	addHandler("/post", http.MethodPost, rejectUnauth, handleReq)

	for _, tc := range []struct {
		method, path string
		email        string // google auth
		user, pass   string // basic auth
		code         int    // expected HTTP status code
	}{
		{"GET", "/", email1, "", "", 200},
		{"GET", "/", email2, "", "", 200},
		{"GET", "/", "", user1, pass1, 200},
		{"GET", "/", "", user2, pass2, 200},
		{"GET", "/", email1, badUser, badPass, 302}, // bad basic user; don't check google
		{"GET", "/", badEmail, "", "", 302},         // bad google user
		{"GET", "/", "", badUser, badPass, 302},     // bad basic user
		{"GET", "/", "", user1, pass2, 302},         // bad basic password
		{"GET", "/", "", user2, "", 302},            // no basic password
		{"GET", "/", "", "", "", 302},               // no auth
		{"POST", "/", "", "", "", 302},              // no auth, wrong method
		{"POST", "/", email1, "", "", 405},          // valid auth, wrong method

		{"GET", "/get", email1, "", "", 200},
		{"GET", "/get", badEmail, "", "", 401},
		{"GET", "/get", "", "", "", 401},      // no auth
		{"POST", "/get", "", "", "", 401},     // no auth, wrong method
		{"POST", "/get", email1, "", "", 405}, // valid auth, wrong method

		{"POST", "/post", email1, "", "", 200},
		{"POST", "/post", badEmail, "", "", 401},
		{"POST", "/post", "", "", "", 401},    // no auth
		{"GET", "/post", "", "", "", 401},     // no auth, wrong method
		{"GET", "/post", email1, "", "", 405}, // valid auth, wrong method
	} {
		desc := tc.method + " " + tc.path
		req, err := inst.NewRequest(tc.method, tc.path, nil)
		if err != nil {
			t.Fatalf("Creating %v request failed: %v", desc, err)
		}

		// Add credentials.
		if tc.email != "" {
			aetest.Login(&user.User{Email: tc.email}, req)
			desc += " email=" + tc.email
		} else {
			aetest.Logout(req)
		}
		if tc.user != "" {
			req.SetBasicAuth(tc.user, tc.pass)
			desc += fmt.Sprintf(" basic=%s/%s", tc.user, tc.pass)
		}

		lastMethod, lastPath = "", ""
		rec := httptest.NewRecorder()
		http.DefaultServeMux.ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Errorf("%v returned %v; want %v", desc, rec.Code, tc.code)
			continue
		}
		if rec.Code == 200 {
			if lastMethod != tc.method || lastPath != tc.path {
				t.Errorf("%v resulted in %v %v", desc, lastMethod, lastPath)
			}
		} else {
			if lastMethod != "" || lastPath != "" {
				t.Errorf("%v resulted in %v %v; should've been rejected",
					desc, lastMethod, lastPath)
			}
		}
	}
}
