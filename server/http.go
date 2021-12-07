// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/derat/nup/server/config"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/log"
	"google.golang.org/appengine/v2/user"
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

// handlerAuth is passed to addHandler to describe authorization requirements.
type handlerAuth int

const (
	rejectUnauth   handlerAuth = iota // 401 if unauthorized
	redirectUnauth                    // 302 to login page if unauthorized
)

// handlerFunc handles HTTP requests to a single endpoint.
type handlerFunc func(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request)

// addHandler registers fn to handle HTTP requests to the specified path.
// Requests are verified to meet authorization requirements and use
// the specified HTTP method before they are passed to fn.
func addHandler(path, method string, auth handlerAuth, fn handlerFunc) {
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		ctx := appengine.NewContext(r)
		cfg, err := config.GetConfig(ctx)
		if err != nil {
			log.Criticalf(ctx, "Failed getting config: %v", err)
			http.Error(w, "Failed getting config", http.StatusInternalServerError)
			return
		}

		if ok, username := cfg.Auth(r); !ok {
			switch auth {
			case rejectUnauth:
				log.Debugf(ctx, "Unauthorized request for %v from %v (user %q)",
					r.URL.String(), r.RemoteAddr, username)
				http.Error(w, "Request requires authorization", http.StatusUnauthorized)
			case redirectUnauth:
				if u, err := getLoginURL(ctx); err != nil {
					log.Errorf(ctx, "Failed generating login URL: %v", err)
					http.Error(w, "Failed redirecting to login", http.StatusInternalServerError)
				} else {
					log.Debugf(ctx, "Unauthorized request for %v from %v (user %q); redirecting to %v",
						r.URL.String(), r.RemoteAddr, username, u)
					http.Redirect(w, r, u, http.StatusFound)
				}
			}
			return
		}

		if r.Method != method {
			log.Debugf(ctx, "Invalid %v request for %v (expected %v)", r.Method, r.URL.String(), method)
			w.Header().Set("Allow", method)
			http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
			return
		}

		fn(ctx, cfg, w, r)
	})
}

// getLoginURL returns a login URL for the app.
func getLoginURL(ctx context.Context) (string, error) {
	u, err := user.LoginURL(ctx, "/")
	if err != nil {
		return "", err
	}
	// If the user is already logged in, send them to a URL that logs
	// them out first to avoid a redirect loop.
	if user.Current(ctx) != nil {
		return user.LogoutURL(ctx, u)
	}
	return u, err
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
	// App Engine "helpfully" rewrites Cache-Control to "no-cache, must-revalidate" in
	// response to requests from admin users: https://github.com/derat/nup/issues/1
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Expires", time.Now().UTC().Add(24*time.Hour).Format(time.RFC1123))
}
