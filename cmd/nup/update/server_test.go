// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

func TestImportSongs(t *testing.T) {
	var numReqs int
	recv := make([]db.Song, 0)
	replace := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replace = r.FormValue("replaceUserData")
		defer r.Body.Close()

		// Make the first request fail to test the retry logic.
		if numReqs++; numReqs == 1 {
			http.Error(w, "Intentional failure", http.StatusInternalServerError)
			return
		}

		d := json.NewDecoder(r.Body)
		for {
			s := db.Song{}
			if err := d.Decode(&s); err == io.EOF {
				break
			} else if err != nil {
				t.Errorf("failed to decode song: %v", err)
			}
			recv = append(recv, s)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	cfg := &client.Config{ServerURL: server.URL}
	ch := make(chan db.Song)

	s0 := db.Song{
		SHA1:     "1977c91fea860245695dcceea0805c14cede7559",
		Filename: "arovane/atol_scrap/thaem_nue.mp3",
		Artist:   "Arovane",
		Title:    "Thaem Nue",
		Album:    "Atol Scrap",
		Track:    3,
		Disc:     1,
		Length:   449,
		Rating:   4,
		Plays: []db.Play{
			db.NewPlay(test.Date(2010, 6, 9, 4, 19, 30), "127.0.0.1"),
			db.NewPlay(test.Date(2011, 2, 10, 5, 48, 33), "1.2.3.4"),
		},
		Tags: []string{"electronic", "instrumental"},
	}
	s1 := db.Song{
		SHA1:     "b70984a4ac5084999b70478cdf163218b90cefdb",
		Filename: "gary_hoey/animal_instinct/motown_fever.mp3",
		Artist:   "Gary Hoey",
		Title:    "Motown Fever",
		Album:    "Animal Instinct",
		Track:    7,
		Disc:     1,
		Length:   182,
		Rating:   3,
		Plays:    []db.Play{db.NewPlay(test.Date(2014, 3, 14, 5, 12, 10), "8.8.8.8")},
		Tags:     []string{"instrumental", "rock"},
	}
	go func() {
		ch <- s0
		ch <- s1
		close(ch)
	}()
	if err := importSongs(cfg, ch, importReplaceUserData|importNoRetryDelay); err != nil {
		t.Fatalf("Failed to send songs: %v", err)
	}
	if err := test.CompareSongs([]db.Song{s0, s1}, recv, test.CompareOrder); err != nil {
		t.Error("Bad songs after initial import: ", err)
	}
	if replace != "1" {
		t.Errorf("replaceUserData param was %q instead of 1", replace)
	}

	recv = recv[:0]
	sent := make([]db.Song, 250, 250)
	ch = make(chan db.Song)
	go func() {
		for i := range sent {
			sent[i].SHA1 = strconv.Itoa(i)
			ch <- sent[i]
		}
		close(ch)
	}()
	if err := importSongs(cfg, ch, importNoRetryDelay); err != nil {
		t.Fatalf("Failed to send songs: %v", err)
	}
	if err := test.CompareSongs(sent, recv, test.CompareOrder); err != nil {
		t.Error("Bad songs after second import: ", err)
	}
	if len(replace) > 0 {
		t.Errorf("replaceUserData param was %q instead of empty", replace)
	}
}
