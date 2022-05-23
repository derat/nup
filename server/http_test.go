// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/derat/nup/server/config"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/aetest"
	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/user"
)

// This test also exercises a lot of code from the config package, but the aetest package is slow
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
	inst, err := aetest.NewInstance(&aetest.Options{SuppressDevAppServerLog: true})
	if err != nil {
		t.Fatal("Failed starting dev_appserver: ", err)
	}
	defer inst.Close()

	// Save a config to Datastore.
	origCfg := &config.Config{
		BasicAuthUsers: []config.BasicAuthInfo{
			{Username: user1, Password: pass1},
			{Username: user2, Password: pass2},
		},
		GoogleUsers: []string{email1, email2},
		SongBucket:  "test-songs",
		CoverBucket: "test-covers",
	}
	b, err := json.Marshal(origCfg)
	if err != nil {
		t.Fatal("Failed marshaling config: ", err)
	}

	// The aetest package makes no sense. It looks like I need to call NewRequest
	// just to get a context.
	req, err := inst.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal("Failed creating request: ", err)
	}
	ctx := appengine.NewContext(req)
	scfg := config.SavedConfig{JSON: string(b)}
	key := datastore.NewKey(ctx, config.DatastoreKind, config.DatastoreKeyName, 0, nil)
	if _, err := datastore.Put(ctx, key, &scfg); err != nil {
		t.Fatal("Failed saving config: ", err)
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
	addHandler("/cron", http.MethodGet, rejectUnauthCron, handleReq)
	addHandler("/allow", http.MethodGet, allowUnauth, handleReq)

	for _, tc := range []struct {
		method, path string
		email        string // google auth
		user, pass   string // basic auth
		cron         bool   // set X-Appengine-Cron header
		code         int    // expected HTTP status code
	}{
		{"GET", "/", email1, "", "", false, 200},
		{"GET", "/", email2, "", "", false, 200},
		{"GET", "/", "", user1, pass1, false, 200},
		{"GET", "/", "", user2, pass2, false, 200},
		{"GET", "/", email1, badUser, badPass, false, 302}, // bad basic user; don't check google
		{"GET", "/", badEmail, "", "", false, 302},         // bad google user
		{"GET", "/", "", badUser, badPass, false, 302},     // bad basic user
		{"GET", "/", "", user1, pass2, false, 302},         // bad basic password
		{"GET", "/", "", user2, "", false, 302},            // no basic password
		{"GET", "/", "", "", "", false, 302},               // no auth
		{"POST", "/", "", "", "", false, 302},              // no auth, wrong method
		{"POST", "/", email1, "", "", false, 405},          // valid auth, wrong method

		{"GET", "/get", email1, "", "", false, 200},
		{"GET", "/get", badEmail, "", "", false, 401},
		{"GET", "/get", "", "", "", false, 401},      // no auth
		{"POST", "/get", "", "", "", false, 401},     // no auth, wrong method
		{"POST", "/get", email1, "", "", false, 405}, // valid auth, wrong method

		{"POST", "/post", email1, "", "", false, 200},
		{"POST", "/post", badEmail, "", "", false, 401},
		{"POST", "/post", "", "", "", false, 401},    // no auth
		{"GET", "/post", "", "", "", false, 401},     // no auth, wrong method
		{"GET", "/post", email1, "", "", false, 405}, // valid auth, wrong method

		{"GET", "/cron", email1, "", "", false, 200},
		{"GET", "/cron", "", user1, pass1, false, 200},
		{"GET", "/cron", "", "", "", true, 200},
		{"GET", "/cron", "", "", "", false, 401},       // no auth
		{"GET", "/cron", badEmail, "", "", false, 401}, // bad google user
		{"POST", "/cron", "", "", "", true, 405},       // wrong method

		{"GET", "/allow", "", "", "", false, 200},       // no auth
		{"GET", "/allow", email1, "", "", false, 200},   // valid user
		{"GET", "/allow", "", user1, pass1, false, 200}, // valid auth
		{"POST", "/allow", "", "", "", false, 405},      // wrong method
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
		if tc.cron {
			req.Header.Set("X-Appengine-Cron", "true")
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
			if rec.Code == 302 {
				// These checks depend on the format of App Engine URLs:
				//  /_ah/login?continue=http%3A//localhost%3A34773/
				//  /_ah/logout?continue=http%3A//localhost%3A34773/_ah/login%3Fcontinue%3Dhttp%253A//localhost%253A34773/
				if loc := rec.Result().Header.Get("Location"); !strings.Contains(loc, "/login") {
					t.Errorf("%v redirected to non-login URL %v", desc, loc)
				} else if tc.email != "" && !strings.Contains(loc, "/logout") {
					t.Errorf("%v redirected to non-logout URL %v", desc, loc)
				}
			}
		}
	}
}
