package main

import (
	"encoding/json"
	"testing"

	"erat.org/nup"
)

func checkSongs(t *testing.T, expected []nup.Song, ch chan SongAndError, num int) {
	actual := make([]SongAndError, 0)
	for i := 0; i < num; i++ {
		actual = append(actual, <-ch)
	}

	for i := 0; i < len(expected); i++ {
		es := expected[i]
		if i >= len(actual) {
			t.Errorf("missing song at position %v; expected %q", i, es.Filename)
		} else if actual[i].Error != nil {
			t.Errorf("got error at position %v instead of %q: %v", i, es.Filename, actual[i].Error)
		} else {
			e, err := json.Marshal(expected[i])
			if err != nil {
				t.Fatalf("unable to marshal to JSON: %v", err)
			}
			a, err := json.Marshal(actual[i].Song)
			if err != nil {
				t.Fatalf("unable to marshall to JSON: %v", err)
			}
			if string(a) != string(e) {
				t.Errorf("song %v didn't match expected values:\nexpected: %v\n  actual: %v", i, string(e), string(a))
			}
		}
	}

	for i := len(expected); i < len(actual); i++ {
		if actual[i].Error != nil {
			t.Errorf("got extra error at position %v: %v", i, actual[i].Error)
		} else {
			t.Errorf("got unexpected song %q at position %v", actual[i].Song.Filename, i)
		}
	}
}
