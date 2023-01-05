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
		normEmail = "normal@example.org"
		badEmail  = "bad@example.org"

		normUser = "norm"
		normPass = "normPass"

		adminUser = "admin"
		adminPass = "adminPass"

		guestUser = "guest"
		guestPass = "guestPass"

		badUser = "bad"
		badPass = "badPass"
	)

	// Start dev_appserver.py.
	inst, err := aetest.NewInstance(&aetest.Options{SuppressDevAppServerLog: true})
	if err != nil {
		t.Fatal("Failed starting dev_appserver: ", err)
	}
	defer inst.Close()

	// Save a config to Datastore.
	origCfg := &config.Config{
		Users: []config.User{
			{Username: normUser, Password: normPass},
			{Username: adminUser, Password: adminPass, Admin: true},
			{Username: guestUser, Password: guestPass, Guest: true},
			{Email: normEmail},
		},
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

	norm := config.NormalUser
	admin := config.AdminUser
	guest := config.GuestUser
	cron := config.CronUser

	addHandler("/", http.MethodGet, norm|admin|guest, redirectUnauth, handleReq)
	addHandler("/get", http.MethodGet, norm|admin|guest, rejectUnauth, handleReq)
	addHandler("/post", http.MethodPost, norm|admin, rejectUnauth, handleReq)
	addHandler("/admin", http.MethodPost, admin, rejectUnauth, handleReq)
	addHandler("/cron", http.MethodGet, norm|admin|cron, rejectUnauth, handleReq)
	addHandler("/allow", http.MethodGet, norm|admin, allowUnauth, handleReq)

	for _, tc := range []struct {
		method, path string
		email        string // google auth
		user, pass   string // basic auth
		cron         bool   // set X-Appengine-Cron header
		code         int    // expected HTTP status code
	}{
		{"GET", "/", normEmail, "", "", false, 200},
		{"GET", "/", "", normUser, normPass, false, 200},
		{"GET", "/", "", adminUser, adminPass, false, 200},
		{"GET", "/", "", guestUser, guestPass, false, 200},
		{"GET", "/", normEmail, badUser, badPass, false, 302}, // bad basic user; don't check google
		{"GET", "/", badEmail, "", "", false, 302},            // bad google user
		{"GET", "/", "", badUser, badPass, false, 302},        // bad basic user
		{"GET", "/", "", normUser, badPass, false, 302},       // bad basic password
		{"GET", "/", "", normUser, "", false, 302},            // no basic password
		{"GET", "/", "", "", "", false, 302},                  // no auth
		{"POST", "/", "", "", "", false, 302},                 // no auth, wrong method
		{"POST", "/", normEmail, "", "", false, 405},          // valid auth, wrong method

		{"GET", "/get", normEmail, "", "", false, 200},
		{"GET", "/get", "", adminUser, adminPass, false, 200},
		{"GET", "/get", "", guestUser, guestPass, false, 200},
		{"GET", "/get", badEmail, "", "", false, 401},
		{"GET", "/get", "", "", "", false, 401},         // no auth
		{"POST", "/get", "", "", "", false, 401},        // no auth, wrong method
		{"POST", "/get", normEmail, "", "", false, 405}, // valid auth, wrong method

		{"POST", "/post", normEmail, "", "", false, 200},
		{"POST", "/post", "", adminUser, adminPass, false, 200},
		{"POST", "/post", badEmail, "", "", false, 401},
		{"POST", "/post", "", "", "", false, 401},               // no auth
		{"GET", "/post", "", "", "", false, 401},                // no auth, wrong method
		{"POST", "/post", "", guestUser, guestPass, false, 403}, // guest not allowed
		{"GET", "/post", normEmail, "", "", false, 405},         // valid auth, wrong method

		{"POST", "/admin", "", adminUser, adminPass, false, 200},
		{"POST", "/admin", normEmail, "", "", false, 403},      // not admin
		{"POST", "/admin", "", normUser, normPass, false, 403}, // not admin
		{"POST", "/post", "", "", "", false, 401},              // no auth

		{"GET", "/cron", normEmail, "", "", false, 200},
		{"GET", "/cron", "", normUser, normPass, false, 200},
		{"GET", "/cron", "", adminUser, adminPass, false, 200},
		{"GET", "/cron", "", "", "", true, 200},
		{"GET", "/cron", "", "", "", false, 401},       // no auth
		{"GET", "/cron", badEmail, "", "", false, 401}, // bad google user
		{"POST", "/cron", "", "", "", true, 405},       // wrong method

		{"GET", "/allow", "", "", "", false, 200},               // no auth
		{"GET", "/allow", normEmail, "", "", false, 200},        // valid user
		{"GET", "/allow", "", normUser, normPass, false, 200},   // valid auth
		{"GET", "/allow", "", adminUser, adminPass, false, 200}, // valid auth
		{"GET", "/allow", "", guestUser, guestPass, false, 200}, // unlisted auth
		{"POST", "/allow", "", "", "", false, 405},              // wrong method
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
