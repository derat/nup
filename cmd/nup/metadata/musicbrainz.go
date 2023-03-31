// Copyright 2023 Daniel Erat.
// All rights reserved.

package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/server/db"
	"golang.org/x/time/rate"
)

const (
	// https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting
	maxQPS         = 1
	rateBucketSize = 1
	userAgent      = "nup/0 ( https://github.com/derat/nup )"

	maxTries   = 3
	retryDelay = 5 * time.Second
)

// api fetches information using the MusicBrainz API.
// See https://musicbrainz.org/doc/MusicBrainz_API.
type api struct {
	srvURL  string        // base URL of web server, e.g. "https://musicbrainz.org"
	limiter *rate.Limiter // rate-limits network requests

	lastRelMBID string // last ID passed to getRelease (differs from lastRel.ID if merged)
	lastRel     *release
	lastRelErr  error
}

func newAPI(srvURL string) *api {
	return &api{
		srvURL:  srvURL,
		limiter: rate.NewLimiter(maxQPS, rateBucketSize),
	}
}

// httpError is returned by send for non-200 status codes.
type httpError struct {
	code   int
	status string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("server returned %v (%q)", e.code, e.status)
}

// fatal returns true if the request that caused the error should not be retried.
func (e *httpError) fatal() bool {
	switch e.code {
	case http.StatusBadRequest: // returned if we e.g. send an empty MBID
		return true
	case http.StatusNotFound: // returned if the entity doesn't exist
		return true
	default:
		return false
	}
}

// send sends a GET request to the API using the supplied path (e.g. "/ws/2/...?fmt=json")
// and unmarshals the JSON response into dst.
func (api *api) send(ctx context.Context, path string, dst interface{}) error {
	try := func() (io.ReadCloser, error) {
		if err := api.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.srvURL+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			err = &httpError{resp.StatusCode, resp.Status}
		}
		return resp.Body, err
	}

	var tries int
	for {
		body, err := try()
		tries++

		if err == nil {
			defer body.Close()
			return json.NewDecoder(body).Decode(dst)
		}

		if body != nil {
			body.Close()
		}
		if tries >= maxTries {
			return err
		} else if he, ok := err.(*httpError); ok && he.fatal() {
			return err
		}
		time.Sleep(retryDelay)
	}
}

// getRelease fetches the release with the supplied MBID.
func (api *api) getRelease(ctx context.Context, mbid string) (*release, error) {
	if mbid == api.lastRelMBID {
		return api.lastRel, api.lastRelErr
	}
	api.lastRelMBID = mbid
	api.lastRel = &release{}
	api.lastRelErr = api.send(ctx, "/ws/2/release/"+mbid+"?inc=artist-credits+recordings+release-groups&fmt=json", api.lastRel)
	return api.lastRel, api.lastRelErr
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

func (rel *release) getTrackByIndex(idx int) *track {
	for _, m := range rel.Media {
		if idx < len(m.Tracks) {
			return &m.Tracks[idx]
		}
		idx -= len(m.Tracks)
	}
	return nil
}

func (rel *release) numTracks() int {
	var n int
	for _, m := range rel.Media {
		n += len(m.Tracks)
	}
	return n
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
	Length    int64          `json:"length"` // milliseconds
	Recording recording      `json:"recording"`
}

type recording struct {
	Title            string         `json:"title"`
	Artists          []artistCredit `json:"artist-credit"`
	ID               string         `json:"id"`
	Length           int64          `json:"length"` // milliseconds
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

func (d date) MarshalJSON() ([]byte, error) {
	t := time.Time(d)
	if t.IsZero() {
		return json.Marshal("")
	} else {
		return json.Marshal(t.Format("2006-01-02"))
	}
}

func (d date) String() string { return time.Time(d).String() }

// updateSongFromRelease updates fields in song using data from rel.
// false is returned if the recording isn't included in the release.
func updateSongFromRelease(song *db.Song, rel *release) bool {
	tr, med := rel.findTrack(song.RecordingID)
	if tr == nil {
		return false
	}

	song.Artist = joinArtistCredits(tr.Artists)
	song.Title = tr.Title
	song.Album = rel.Title
	song.DiscSubtitle = med.Title
	song.AlbumID = rel.ID
	song.Track = tr.Position
	song.Disc = med.Position
	song.Date = time.Time(rel.ReleaseGroup.FirstReleaseDate)

	// Only set the album artist if it differs from the song artist or if it was previously set.
	// Otherwise we're creating needless churn, since the update command won't send it to the server
	// if it's the same as the song artist.
	if aa := joinArtistCredits(rel.Artists); aa != song.Artist || song.AlbumArtist != "" {
		song.AlbumArtist = aa
	}

	return true
}

// updateSongFromRecording updates fields in song using data from rec.
// This should only be used for standalone recordings.
func updateSongFromRecording(song *db.Song, rec *recording) {
	song.Artist = joinArtistCredits(rec.Artists)
	song.Title = rec.Title
	song.Album = files.NonAlbumTracksValue
	song.AlbumID = ""
	song.Date = time.Time(rec.FirstReleaseDate) // always zero?
}
