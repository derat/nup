package main

import (
	"encoding/json"
	"testing"

	"erat.org/nup"
)

func getSongsFromChannel(t *testing.T, ch chan SongAndError, num int) []nup.Song {
	songs := make([]nup.Song, 0)
	for i := 0; i < num; i++ {
		sae := <-ch
		if sae.Error != nil {
			t.Errorf("got error at position %v instead of song: %v", i, sae.Error)
		}
		songs = append(songs, *sae.Song)
	}
	return songs
}

func compareSongs(t *testing.T, expected, actual []nup.Song) {
	for i := 0; i < len(expected); i++ {
		if i >= len(actual) {
			t.Errorf("missing song at position %v; expected %q", i, expected[i].Filename)
		} else {
			e, err := json.Marshal(expected[i])
			if err != nil {
				t.Fatalf("unable to marshal to JSON: %v", err)
			}
			a, err := json.Marshal(actual[i])
			if err != nil {
				t.Fatalf("unable to marshal to JSON: %v", err)
			}
			if string(a) != string(e) {
				t.Errorf("song %v didn't match expected values:\nexpected: %v\n  actual: %v", i, string(e), string(a))
			}
		}
	}
	for i := len(expected); i < len(actual); i++ {
		t.Errorf("got unexpected song %q at position %v", actual[i].Filename, i)
	}
}
