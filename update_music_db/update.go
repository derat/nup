package main

import (
	"bytes"
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
	jsonType        = "application/json"
	oauthScope      = "https://www.googleapis.com/auth/userinfo.email"
	updateSongsPath = "update_songs"
)

type updater struct {
	serverUrl string
	transport *oauth.Transport
}

func newUpdater(server, clientId, clientSecret, tokenCache string) (*updater, error) {
	serverUrl, err := url.Parse(server)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(serverUrl.Path, "/") {
		serverUrl.Path += "/"
	}
	transport, err := cloud.CreateTransport(clientId, clientSecret, oauthScope, tokenCache)
	if err != nil {
		return nil, err
	}
	return &updater{serverUrl: serverUrl.String(), transport: transport}, nil
}

func (u *updater) GetLastUpdateTime() (time.Time, error) {
	return time.Now(), nil
}

func (u *updater) SetLastUpdateTime(t time.Time) error {
	return nil
}

func (u *updater) UpdateSongs(songs *[]nup.SongData) error {
	b, err := json.Marshal(*songs)
	if err != nil {
		return err
	}

	if err = cloud.MaybeRefreshToken(u.transport); err != nil {
		return err
	}
	resp, err := u.transport.Client().Post(u.serverUrl+updateSongsPath, jsonType, bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Got non-OK status from %v: %v", updateSongsPath, resp.Status)
	}
	return nil
}

func (u *updater) UpdateExtra(extra *[]nup.ExtraSongData) error {
	return nil
}
