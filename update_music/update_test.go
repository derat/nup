package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"erat.org/nup"
	"erat.org/nup/test"
)

func TestUpdate(t *testing.T) {
	receivedSongs := make([]nup.Song, 0)
	replace := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replace = r.FormValue("replaceUserData")

		defer r.Body.Close()
		d := json.NewDecoder(r.Body)
		for true {
			s := nup.Song{}
			if err := d.Decode(&s); err == io.EOF {
				break
			} else if err != nil {
				t.Errorf("failed to decode song: %v", err)
			}
			receivedSongs = append(receivedSongs, s)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	cfg := Config{ClientConfig: nup.ClientConfig{ServerUrl: server.URL}}
	ch := make(chan nup.Song)

	s0 := nup.Song{
		Sha1:     "1977c91fea860245695dcceea0805c14cede7559",
		Filename: "arovane/atol_scrap/thaem_nue.mp3",
		Artist:   "Arovane",
		Title:    "Thaem Nue",
		Album:    "Atol Scrap",
		Track:    3,
		Disc:     1,
		Length:   449,
		Rating:   0.75,
		Plays:    []nup.Play{{time.Unix(1276057170, 0), "127.0.0.1"}, {time.Unix(1297316913, 0), "1.2.3.4"}},
		Tags:     []string{"electronic", "instrumental"},
	}
	s1 := nup.Song{
		Sha1:     "b70984a4ac5084999b70478cdf163218b90cefdb",
		Filename: "gary_hoey/animal_instinct/motown_fever.mp3",
		Artist:   "Gary Hoey",
		Title:    "Motown Fever",
		Album:    "Animal Instinct",
		Track:    7,
		Disc:     1,
		Length:   182,
		Rating:   0.5,
		Plays:    []nup.Play{{time.Unix(1394773930, 0), "8.8.8.8"}},
		Tags:     []string{"instrumental", "rock"},
	}
	go func() {
		ch <- s0
		ch <- s1
	}()
	if err := updateSongs(cfg, ch, 2, true); err != nil {
		t.Fatalf("failed to send songs: %v", err)
	}
	if err := test.CompareSongs([]nup.Song{s0, s1}, receivedSongs, true); err != nil {
		t.Error(err)
	}
	if replace != "1" {
		t.Errorf("replaceUserData param was %q instead of 1", replace)
	}

	receivedSongs = receivedSongs[:0]
	sentSongs := make([]nup.Song, 250, 250)
	go func() {
		for i := 0; i < len(sentSongs); i++ {
			sentSongs[i].Sha1 = strconv.Itoa(i)
			ch <- sentSongs[i]
		}
	}()
	if err := updateSongs(cfg, ch, len(sentSongs), false); err != nil {
		t.Fatalf("failed to send songs: %v", err)
	}
	if err := test.CompareSongs(sentSongs, receivedSongs, true); err != nil {
		t.Error(err)
	}
	if len(replace) > 0 {
		t.Errorf("replaceUserData param was %q instead of empty", replace)
	}
}
