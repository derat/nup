package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"code.google.com/p/goauth2/oauth"
	"erat.org/cloud"
	"erat.org/nup"
)

const (
	oauthScope = "https://www.googleapis.com/auth/userinfo.email"

	// Server path to update songs and query params.
	updatePath         = "update_songs"
	updateSongsKey     = "songs"
	updateReplaceKey   = "replace"
	updateReplaceValue = "1"
)

type updater struct {
	serverUrl url.URL
	transport *oauth.Transport
}

func newUpdater(cfg config) (*updater, error) {
	serverUrl, err := url.Parse(cfg.ServerUrl)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(serverUrl.Path, "/") {
		serverUrl.Path += "/"
	}
	transport, err := cloud.CreateTransport(cfg.ClientId, cfg.ClientSecret, oauthScope, cfg.TokenCache)
	if err != nil {
		return nil, err
	}
	return &updater{serverUrl: *serverUrl, transport: transport}, nil
}

func (u *updater) GetLastUpdateTime() (time.Time, error) {
	return time.Now(), nil
}

func (u *updater) SetLastUpdateTime(t time.Time) error {
	return nil
}

func (u *updater) UpdateSongs(songs []nup.Song, replace bool) error {
	b, err := json.Marshal(songs)
	if err != nil {
		return err
	}

	if err = cloud.MaybeRefreshToken(u.transport); err != nil {
		return err
	}

	destUrl := u.serverUrl
	destUrl.Path += updatePath
	form := make(url.Values)
	form.Add(updateSongsKey, string(b))
	if replace {
		form.Add(updateReplaceKey, updateReplaceValue)
	}
	resp, err := u.transport.Client().PostForm(destUrl.String(), form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Got non-OK status from %v: %v", updatePath, resp.Status)
	}
	return nil
}
