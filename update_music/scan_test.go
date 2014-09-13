package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"erat.org/nup"
)

func createTempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	return dir
}

func copyToTempDir(t *testing.T, dir, fn string) {
	sp := filepath.Join("testdata/music", fn)
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

func deleteFromTempDir(t *testing.T, dir, fn string) {
	p := filepath.Join(dir, fn)
	if err := os.Remove(p); err != nil {
		t.Fatalf("failed to remove %v: %v", p, err)
	}
}

func scanAndCheckSongs(t *testing.T, dir string, lastUpdateTime time.Time, expected []nup.Song) {
	ch := make(chan SongAndError)
	num, err := scanForUpdatedSongs(dir, lastUpdateTime, ch)
	if err != nil {
		t.Errorf("scanning for songs failed")
	} else {
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
				var m string
				cmp := func(field string, ev, av interface{}) {
					if ev != av {
						m += fmt.Sprintf("%v: expected %v, actual %v\n", field, ev, av)
					}
				}

				as := actual[i].Song
				cmp("Sha1", es.Sha1, as.Sha1)
				cmp("Filename", es.Filename, as.Filename)
				cmp("Artist", es.Artist, as.Artist)
				cmp("Title", es.Title, as.Title)
				cmp("Album", es.Album, as.Album)
				cmp("Track", es.Track, as.Track)
				cmp("Disc", es.Disc, as.Disc)
				cmp("Length", es.Length, as.Length)

				if len(m) > 0 {
					t.Errorf("song %v didn't match expected values:\n%v", i, m)
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
}

func TestScan(t *testing.T) {
	var song0s nup.Song = nup.Song{
		Sha1:     "fefac74a1d5928316d7131747107c8a61b71ffe4",
		Filename: "0s.mp3",
		Artist:   "First Artist",
		Title:    "Zero Seconds",
		Album:    "First Album",
		Track:    1,
		Disc:     0,
		Length:   0.026,
	}
	var song0sUpdated nup.Song = nup.Song{
		Sha1:     "fefac74a1d5928316d7131747107c8a61b71ffe4",
		Filename: "0s-updated.mp3",
		Artist:   "First Artist",
		Title:    "Zero Seconds (Remix)",
		Album:    "First Album",
		Track:    1,
		Disc:     0,
		Length:   0.026,
	}
	var song1s nup.Song = nup.Song{
		Sha1:     "c6e3230b4ed5e1f25d92dd6b80bfc98736bbee62",
		Filename: "1s.mp3",
		Artist:   "Second Artist",
		Title:    "One Second",
		Album:    "First Album",
		Track:    2,
		Disc:     0,
		Length:   1.071,
	}
	var song5s nup.Song = nup.Song{
		Sha1:     "63afdde2b390804562d54788865fff1bfd11cf94",
		Filename: "5s.mp3",
		Artist:   "Third Artist",
		Title:    "Five Seconds",
		Album:    "Another Album",
		Track:    1,
		Disc:     2,
		Length:   5.041,
	}

	dir := createTempDir(t)
	defer os.RemoveAll(dir)

	copyToTempDir(t, dir, song0s.Filename)
	copyToTempDir(t, dir, song1s.Filename)
	startTime := time.Now()
	scanAndCheckSongs(t, dir, time.Time{}, []nup.Song{song0s, song1s})

	scanAndCheckSongs(t, dir, startTime, []nup.Song{})

	copyToTempDir(t, dir, song5s.Filename)
	addTime := time.Now()
	scanAndCheckSongs(t, dir, startTime, []nup.Song{song5s})

	deleteFromTempDir(t, dir, song0s.Filename)
	copyToTempDir(t, dir, song0sUpdated.Filename)
	updateTime := time.Now()
	scanAndCheckSongs(t, dir, addTime, []nup.Song{song0sUpdated})

	subdir := filepath.Join(dir, "foo")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatalf("unable to create subdirectory %v: %v", subdir, err)
	}
	renamedPath := filepath.Join(subdir, song1s.Filename)
	if err := os.Rename(filepath.Join(dir, song1s.Filename), renamedPath); err != nil {
		t.Fatalf("unable to move file to %v: %v", renamedPath, err)
	}
	now := time.Now()
	if err := os.Chtimes(renamedPath, now, now); err != nil {
		t.Fatalf("failed to set %v's modification time to %v", renamedPath, now)
	}
	renamedSong1s := song1s
	renamedSong1s.Filename = filepath.Join(filepath.Base(subdir), song1s.Filename)
	scanAndCheckSongs(t, dir, updateTime, []nup.Song{renamedSong1s})
}

// TODO: Test errors, skipping bogus files, etc.
