// Copyright 2023 Daniel Erat.
// All rights reserved.

package scan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/test"
	"github.com/google/go-cmp/cmp"
	"golang.org/x/time/rate"
)

type testEnv struct {
	t   *testing.T
	mux *http.ServeMux
	srv *httptest.Server

	cfg *client.Config
	api *api

	recordings map[string]recording
	releases   map[string]release
}

func newTestEnv(t *testing.T) *testEnv {
	dir := t.TempDir()
	env := testEnv{
		t:   t,
		mux: http.NewServeMux(),
		cfg: &client.Config{
			MusicDir:    filepath.Join(dir, "music"),
			MetadataDir: filepath.Join(dir, "metadata"),
		},
		recordings: make(map[string]recording),
		releases:   make(map[string]release),
	}
	env.mux.HandleFunc(recPathPrefix, env.handleRecording)
	env.mux.HandleFunc(relPathPrefix, env.handleRelease)
	env.srv = httptest.NewServer(env.mux)
	env.api = newAPI(env.srv.URL)
	env.api.limiter.SetLimit(rate.Inf)
	return &env
}

func (env *testEnv) close() {
	env.srv.Close()
}

func (env *testEnv) handleRecording(w http.ResponseWriter, req *http.Request) {
	if rec, ok := env.recordings[req.URL.Path[len(recPathPrefix):]]; !ok {
		http.NotFound(w, req)
	} else {
		env.writeJSON(w, rec)
	}
}

func (env *testEnv) handleRelease(w http.ResponseWriter, req *http.Request) {
	if rel, ok := env.releases[req.URL.Path[len(relPathPrefix):]]; !ok {
		http.NotFound(w, req)
	} else {
		env.writeJSON(w, rel)
	}
}

func (env *testEnv) writeJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(obj); err != nil {
		env.t.Error("Failed writing response:", err)
	}
}

const (
	recPathPrefix = "/ws/2/recording/"
	relPathPrefix = "/ws/2/release/"
)

func TestProcessSong_Release(t *testing.T) {
	env := newTestEnv(t)
	defer env.close()

	song := test.Song0s
	test.Must(t, test.CopySongs(env.cfg.MusicDir, song.Filename))

	want := song
	want.Title = "New Title"
	want.Artist = "Artist A feat. B & C"
	want.Album = "New Album"
	want.AlbumArtist = "Artist A"
	want.DiscSubtitle = "The Third Disc"
	want.AlbumID = "5109de80-7946-41ea-9060-05d10da87219"
	want.RecordingID = "4b834e6e-a694-4cbe-a715-f5b0e36c57e2"
	want.OrigAlbumID = song.AlbumID
	want.OrigRecordingID = song.RecordingID
	want.Track = 4
	want.Disc = 3
	want.Date = time.Date(2020, 6, 7, 0, 0, 0, 0, time.UTC)

	want.SHA1 = ""
	want.Length = 0
	want.TrackGain = 0
	want.AlbumGain = 0
	want.PeakAmp = 0

	env.releases[song.AlbumID] = release{
		Title:   want.Album,
		Artists: []artistCredit{{Name: "Artist A"}},
		ID:      want.AlbumID,
		Media: []medium{
			{Position: 1},
			{Position: 2},
			{
				Title:    want.DiscSubtitle,
				Position: 3,
				Tracks: []track{
					{Position: 1},
					{Position: 2},
					{Position: 3},
					{
						Title: want.Title,
						Artists: []artistCredit{
							{Name: "Artist A", JoinPhrase: " feat. "},
							{Name: "B", JoinPhrase: " & "},
							{Name: "C"},
						},
						Recording: recording{ID: want.RecordingID},
						Position:  4,
					},
				},
			},
		},
		ReleaseGroup: releaseGroup{FirstReleaseDate: date(want.Date)},
	}
	env.recordings[song.RecordingID] = recording{ID: want.RecordingID}

	ctx := context.Background()
	p := filepath.Join(env.cfg.MusicDir, song.Filename)
	test.Must(t, processSong(ctx, env.cfg, env.api, p, nil, nil))
	if got, err := files.ReadSong(env.cfg, p, nil /* fi */, true /* onlyTags */, nil /* gc */); err != nil {
		t.Error("ReadSong failed:", err)
	} else if diff := cmp.Diff(want, *got); diff != "" {
		t.Error("ReadSong returned unexpected results:\n" + diff)
	}
}

// TODO: Add a test for a standalone recording (non-album track).
// I think that this will be hard to do unless I add a song file
// with a recording ID but no album ID.
