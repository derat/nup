// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/derat/nup/internal/pkg/types"
	"github.com/derat/nup/server/common"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/user"
)

// Maximum response size permitted by App Engine:
// https://cloud.google.com/appengine/docs/standard/go111/how-requests-are-handled
const maxResponseSize = 32 * 1024 * 1024

// writeJSONResponse serializes v to JSON and writes it to w.
func writeJSONResponse(w http.ResponseWriter, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
}

// writeTextResponse writes s to w as a text response.
func writeTextResponse(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	w.Write([]byte(s))
}

// hasAllowedGoogleAuth checks whether ctx contains credentials for a Google
// user registered in cfg.
func hasAllowedGoogleAuth(ctx context.Context, cfg *types.ServerConfig) (email string, allowed bool) {
	u := user.Current(ctx)
	if u == nil {
		return "", false
	}

	for _, e := range cfg.GoogleUsers {
		if u.Email == e {
			return u.Email, true
		}
	}
	return u.Email, false
}

// hasAllowedBasicAuth checks whether r is authorized via HTTP basic
// authentication with a user registered in cfg. If basic auth was used, the
// username return value is set regardless of the user is allowed or not.
func hasAllowedBasicAuth(r *http.Request, cfg *types.ServerConfig) (username string, allowed bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", false
	}
	for _, u := range cfg.BasicAuthUsers {
		if username == u.Username && password == u.Password {
			return username, true
		}
	}
	return username, false
}

// hasWebDriverCookie returns true if r contains a special cookie set by browser
// tests that use WebDriver.
func hasWebDriverCookie(r *http.Request) bool {
	if _, err := r.Cookie("webdriver"); err != nil {
		return false
	}
	return true
}

// checkRequest verifies that r is an authorized request using method.
// If the request is unauthorized and redirectToLogin is true, the client
// is redirected to the login screen.
func checkRequest(ctx context.Context, w http.ResponseWriter, r *http.Request,
	method string, redirectToLogin bool) bool {
	cfg := common.Config(ctx)
	username, allowed := hasAllowedGoogleAuth(ctx, cfg)
	if !allowed && len(username) == 0 {
		username, allowed = hasAllowedBasicAuth(r, cfg)
	}
	// Ugly hack since WebDriver doesn't support basic auth.
	if !allowed && appengine.IsDevAppServer() && hasWebDriverCookie(r) {
		allowed = true
	}
	if !allowed {
		if len(username) == 0 && redirectToLogin {
			loginURL, _ := user.LoginURL(ctx, "/")
			log.Debugf(ctx, "Unauthenticated request for %v from %v; redirecting to login", r.URL.String(), r.RemoteAddr)
			http.Redirect(w, r, loginURL, http.StatusFound)
		} else {
			log.Debugf(ctx, "Unauthorized request for %v from %q at %v", r.URL.String(), username, r.RemoteAddr)
			http.Error(w, "Request requires authorization", http.StatusUnauthorized)
		}
		return false
	}

	if r.Method != method {
		log.Debugf(ctx, "Invalid %v request for %v (expected %v)", r.Method, r.URL.String(), method)
		w.Header().Set("Allow", method)
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return false
	}

	return true
}

// parseIntParam parses and returns the named int64 form parameter from r.
// If the parameter is missing or unparseable, a bad request error is written
// to w, an error is logged, and the ok return value is false.
func parseIntParam(ctx context.Context, w http.ResponseWriter, r *http.Request,
	name string) (v int64, ok bool) {
	s := r.FormValue(name)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Errorf(ctx, "Unable to parse %v param %q", name, s)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return v, false
	}
	return v, true
}

// parseFloatParam parses and returns the float64 form parameter from r.
// If the parameter is missing or unparseable, a bad request error is written
// to w, an error is logged, and the ok return value is false.
func parseFloatParam(ctx context.Context, w http.ResponseWriter, r *http.Request,
	name string) (v float64, ok bool) {
	s := r.FormValue(name)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Errorf(ctx, "Unable to parse %v param %q", name, s)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return v, false
	}
	return v, true
}

// addLongCacheHeaders adds headers to w such that it will be cached for a long time.
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control.
func addLongCacheHeaders(w http.ResponseWriter) {
	// TODO: App Engine "helpfully" rewrites Cache-Control to "no-cache, must-revalidate" in
	// response to requests from admin users: https://github.com/derat/nup/issues/1
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Expires", time.Now().UTC().Add(24*time.Hour).Format(time.RFC1123))
}
