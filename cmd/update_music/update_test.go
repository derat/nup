// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/derat/nup/internal/pkg/test"
	"github.com/derat/nup/internal/pkg/types"
)

func TestUpdate(t *testing.T) {
	recv := make([]types.Song, 0)
	replace := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replace = r.FormValue("replaceUserData")

		defer r.Body.Close()
		d := json.NewDecoder(r.Body)
		for true {
			s := types.Song{}
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

	cfg := config{ClientConfig: types.ClientConfig{ServerURL: server.URL}}
	ch := make(chan types.Song)

	s0 := types.Song{
		SHA1:     "1977c91fea860245695dcceea0805c14cede7559",
		Filename: "arovane/atol_scrap/thaem_nue.mp3",
		Artist:   "Arovane",
		Title:    "Thaem Nue",
		Album:    "Atol Scrap",
		Track:    3,
		Disc:     1,
		Length:   449,
		Rating:   0.75,
		Plays:    []types.Play{{time.Unix(1276057170, 0), "127.0.0.1"}, {time.Unix(1297316913, 0), "1.2.3.4"}},
		Tags:     []string{"electronic", "instrumental"},
	}
	s1 := types.Song{
		SHA1:     "b70984a4ac5084999b70478cdf163218b90cefdb",
		Filename: "gary_hoey/animal_instinct/motown_fever.mp3",
		Artist:   "Gary Hoey",
		Title:    "Motown Fever",
		Album:    "Animal Instinct",
		Track:    7,
		Disc:     1,
		Length:   182,
		Rating:   0.5,
		Plays:    []types.Play{{time.Unix(1394773930, 0), "8.8.8.8"}},
		Tags:     []string{"instrumental", "rock"},
	}
	go func() {
		ch <- s0
		ch <- s1
		close(ch)
	}()
	if err := updateSongs(cfg, ch, true); err != nil {
		t.Fatalf("Failed to send songs: %v", err)
	}
	if err := test.CompareSongs([]types.Song{s0, s1}, recv, test.CompareOrder); err != nil {
		t.Error(err)
	}
	if replace != "1" {
		t.Errorf("replaceUserData param was %q instead of 1", replace)
	}

	recv = recv[:0]
	sent := make([]types.Song, 250, 250)
	ch = make(chan types.Song)
	go func() {
		for i := range sent {
			sent[i].SHA1 = strconv.Itoa(i)
			ch <- sent[i]
		}
		close(ch)
	}()
	if err := updateSongs(cfg, ch, false); err != nil {
		t.Fatalf("Failed to send songs: %v", err)
	}
	if err := test.CompareSongs(sent, recv, test.CompareOrder); err != nil {
		t.Error(err)
	}
	if len(replace) > 0 {
		t.Errorf("replaceUserData param was %q instead of empty", replace)
	}
}
