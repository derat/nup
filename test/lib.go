package test

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"erat.org/nup"
)

var Song0s nup.Song = nup.Song{
	Sha1:     "fefac74a1d5928316d7131747107c8a61b71ffe4",
	Filename: "0s.mp3",
	Artist:   "First Artist",
	Title:    "Zero Seconds",
	Album:    "First Album",
	Track:    1,
	Disc:     0,
	Length:   0.026,
}
var Song0sUpdated nup.Song = nup.Song{
	Sha1:     "fefac74a1d5928316d7131747107c8a61b71ffe4",
	Filename: "0s-updated.mp3",
	Artist:   "First Artist",
	Title:    "Zero Seconds (Remix)",
	Album:    "First Album",
	Track:    1,
	Disc:     0,
	Length:   0.026,
}
var Song1s nup.Song = nup.Song{
	Sha1:     "c6e3230b4ed5e1f25d92dd6b80bfc98736bbee62",
	Filename: "1s.mp3",
	Artist:   "Second Artist",
	Title:    "One Second",
	Album:    "First Album",
	Track:    2,
	Disc:     0,
	Length:   1.071,
}
var Song5s nup.Song = nup.Song{
	Sha1:     "63afdde2b390804562d54788865fff1bfd11cf94",
	Filename: "5s.mp3",
	Artist:   "Third Artist",
	Title:    "Five Seconds",
	Album:    "Another Album",
	Track:    1,
	Disc:     2,
	Length:   5.041,
}

func CreateTempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return dir
}

func CopySongToTempDir(t *testing.T, dir, fn string) {
	_, td, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to get runtime caller info")
	}

	sp := filepath.Join(td, "../data/music", fn)
	s, err := os.Open(sp)
	if err != nil {
		t.Fatalf("failed to open %v: %v", sp, err)
	}
	defer s.Close()

	dp := filepath.Join(dir, fn)
	d, err := os.Create(dp)
	if err != nil {
		t.Fatalf("failed to create %v: %v", dp, err)
	}
	defer d.Close()

	if _, err = io.Copy(d, s); err != nil {
		t.Fatalf("failed to copy %v to %v: %v", sp, dp, err)
	}

	now := time.Now()
	if err = os.Chtimes(dp, now, now); err != nil {
		t.Fatalf("failed to set %v's modification time to %v", dp, now)
	}
}

func DeleteFromTempDir(t *testing.T, dir, fn string) {
	p := filepath.Join(dir, fn)
	if err := os.Remove(p); err != nil {
		t.Fatalf("failed to remove %v: %v", p, err)
	}
}

func CompareSongs(t *testing.T, expected, actual []nup.Song) {
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
