package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"erat.org/nup"
	"erat.org/nup/test"
)

func scanAndCompareSongs(t *testing.T, desc, dir string, lastUpdateTime time.Time, expected []nup.Song) {
	ch := make(chan SongAndError)
	num, err := scanForUpdatedSongs(dir, lastUpdateTime, ch, false)
	if err != nil {
		t.Errorf("%v: %v", desc, err)
		return
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		t.Errorf("%v: %v", desc, err)
		return
	}
	for i := range expected {
		expected[i].Rating = 0
	}
	if err = test.CompareSongs(expected, actual, true); err != nil {
		t.Errorf("%v: %v", desc, err)
	}
}

func TestScan(t *testing.T) {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	test.CopySongsToTempDir(dir, test.Song0s.Filename, test.Song1s.Filename)
	startTime := time.Now()
	scanAndCompareSongs(t, "initial", dir, time.Time{}, []nup.Song{test.Song0s, test.Song1s})
	scanAndCompareSongs(t, "unchanged", dir, startTime, []nup.Song{})

	test.CopySongsToTempDir(dir, test.Song5s.Filename)
	addTime := time.Now()
	scanAndCompareSongs(t, "add", dir, startTime, []nup.Song{test.Song5s})

	if err = os.Remove(filepath.Join(dir, test.Song0s.Filename)); err != nil {
		panic(err)
	}
	test.CopySongsToTempDir(dir, test.Song0sUpdated.Filename)
	updateTime := time.Now()
	scanAndCompareSongs(t, "update", dir, addTime, []nup.Song{test.Song0sUpdated})

	subdir := filepath.Join(dir, "foo")
	if err = os.Mkdir(subdir, 0700); err != nil {
		panic(err)
	}
	renamedPath := filepath.Join(subdir, test.Song1s.Filename)
	if err := os.Rename(filepath.Join(dir, test.Song1s.Filename), renamedPath); err != nil {
		panic(err)
	}
	now := time.Now()
	if err := os.Chtimes(renamedPath, now, now); err != nil {
		panic(err)
	}
	renamedSong1s := test.Song1s
	renamedSong1s.Filename = filepath.Join(filepath.Base(subdir), test.Song1s.Filename)
	scanAndCompareSongs(t, "rename", dir, updateTime, []nup.Song{renamedSong1s})
}

// TODO: Test errors, skipping bogus files, etc.
