// Copyright 2021 Daniel Erat.
// All rights reserved.

package auth

import (
	"context"
	"net/http"

	"google.golang.org/appengine/v2/user"
)

const (
	// TestUsername and TestPassword are accepted for basic HTTP authentication by development servers.
	TestUsername = "testuser"
	TestPassword = "testpass"
)

// BasicAuthInfo contains information used for validating HTTP basic authentication.
type BasicAuthInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HasAllowedGoogleAuth checks whether ctx contains credentials for a Google user in accounts.
func HasAllowedGoogleAuth(ctx context.Context, accounts []string) (email string, allowed bool) {
	u := user.Current(ctx)
	if u == nil {
		return "", false
	}

	for _, e := range accounts {
		if u.Email == e {
			return u.Email, true
		}
	}
	return u.Email, false
}

// HasAllowedBasicAuth checks whether r is authorized via HTTP basic
// authentication with a username and password in infos. If basic auth was used in r,
// the username return value is set regardless of the user is actually allowed or not.
func HasAllowedBasicAuth(r *http.Request, infos []BasicAuthInfo) (username string, allowed bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", false
	}
	for _, u := range infos {
		if username == u.Username && password == u.Password {
			return username, true
		}
	}
	return username, false
}

// auth.HasWebDriverCookie returns true if r contains a special cookie set by browser
// tests that use WebDriver.
func HasWebDriverCookie(r *http.Request) bool {
	if _, err := r.Cookie("webdriver"); err != nil {
		return false
	}
	return true
}
