// Copyright 2023 Daniel Erat.
// All rights reserved.

package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

const (
	// https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting
	maxQPS         = 1
	rateBucketSize = 1
	userAgent      = "nup/0 ( https://github.com/derat/nup )"
)

// api fetches information using the MusicBrainz API.
// See https://musicbrainz.org/doc/MusicBrainz_API.
type api struct {
	limiter *rate.Limiter // rate-limits network requests
	srvURL  string        // base URL of web server, e.g. "https://musicbrainz.org"
}

func newAPI(srvURL string) *api {
	return &api{
		limiter: rate.NewLimiter(maxQPS, rateBucketSize),
		srvURL:  srvURL,
	}
}

// send sends a GET request to the API using the supplied path (e.g. "/ws/2/...?fmt=json")
// and unmarshals the JSON response into dst.
func (api *api) send(ctx context.Context, path string, dst interface{}) error {
	if err := api.limiter.Wait(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.srvURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned %v: %v", resp.StatusCode, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// getRelease fetches the release with the supplied MBID.
func (api *api) getRelease(ctx context.Context, mbid string) (*release, error) {
	var rel release
	err := api.send(ctx, "/ws/2/release/"+mbid+"?inc=artist-credits+recordings+release-groups&fmt=json", &rel)
	return &rel, err
}

// getRecording fetches the recording with the supplied MBID.
// This should only be used for standalone recordings that aren't included in releases.
func (api *api) getRecording(ctx context.Context, mbid string) (*recording, error) {
	var rec recording
	err := api.send(ctx, "/ws/2/recording/"+mbid+"?inc=artist-credits&fmt=json", &rec)
	return &rec, err
}

type release struct {
	Title        string         `json:"title"`
	Artists      []artistCredit `json:"artist-credit"`
	ID           string         `json:"id"`
	Media        []medium       `json:"media"`
	ReleaseGroup releaseGroup   `json:"release-group"`
	Date         date           `json:"date"`
}

func (rel *release) findTrack(recID string) (*track, *medium) {
	for _, m := range rel.Media {
		for _, t := range m.Tracks {
			if t.Recording.ID == recID {
				return &t, &m
			}
		}
	}
	return nil, nil
}

type releaseGroup struct {
	Title            string         `json:"title"`
	Artists          []artistCredit `json:"artist-credit"`
	ID               string         `json:"id"`
	FirstReleaseDate date           `json:"first-release-date"`
}

type artistCredit struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
}

func joinArtistCredits(acs []artistCredit) string {
	var s string
	for _, ac := range acs {
		s += ac.Name + ac.JoinPhrase
	}
	return s
}

type medium struct {
	Title    string  `json:"title"`
	Position int     `json:"position"`
	Tracks   []track `json:"tracks"`
}

type track struct {
	Title     string         `json:"title"`
	Artists   []artistCredit `json:"artist-credit"`
	Position  int            `json:"position"`
	ID        string         `json:"id"`
	Recording recording      `json:"recording"`
}

type recording struct {
	Title            string         `json:"title"`
	Artists          []artistCredit `json:"artist-credit"`
	ID               string         `json:"id"`
	FirstReleaseDate date           `json:"first-release-date"`
}

// date unmarshals a date provided as a JSON string like "2020-10-23".
type date time.Time

func (d *date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		*d = date(time.Time{})
		return nil
	}
	for _, format := range []string{"2006-01-02", "2006-01", "2006"} {
		if t, err := time.Parse(format, s); err == nil {
			*d = date(t)
			return nil
		}
	}
	return fmt.Errorf("malformed date %q", s)
}

func (d date) String() string { return time.Time(d).String() }
