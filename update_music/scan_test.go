package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"erat.org/nup"
	"erat.org/nup/test"
)

func scanAndCompareSongs(dir string, lastUpdateTime time.Time, expected []nup.Song) error {
	ch := make(chan SongAndError)
	num, err := scanForUpdatedSongs(dir, lastUpdateTime, ch, false)
	if err != nil {
		return fmt.Errorf("scanning for songs failed")
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		return err
	}
	return test.CompareSongs(expected, actual)
}

func TestScan(t *testing.T) {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	test.CopySongsToTempDir(dir, test.Song0s.Filename, test.Song1s.Filename)
	startTime := time.Now()
	if err := scanAndCompareSongs(dir, time.Time{}, []nup.Song{test.Song0s, test.Song1s}); err != nil {
		t.Error(err)
	}

	if err = scanAndCompareSongs(dir, startTime, []nup.Song{}); err != nil {
		t.Error(err)
	}

	test.CopySongsToTempDir(dir, test.Song5s.Filename)
	addTime := time.Now()
	if err := scanAndCompareSongs(dir, startTime, []nup.Song{test.Song5s}); err != nil {
		t.Error(err)
	}

	if err = os.Remove(filepath.Join(dir, test.Song0s.Filename)); err != nil {
		panic(err)
	}
	test.CopySongsToTempDir(dir, test.Song0sUpdated.Filename)
	updateTime := time.Now()
	if err = scanAndCompareSongs(dir, addTime, []nup.Song{test.Song0sUpdated}); err != nil {
		t.Error(err)
	}

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
	if err = scanAndCompareSongs(dir, updateTime, []nup.Song{renamedSong1s}); err != nil {
		t.Error(err)
	}
}

// TODO: Test errors, skipping bogus files, etc.
