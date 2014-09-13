package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"erat.org/nup"
	"erat.org/nup/test"
)

func scanAndCompareSongs(t *testing.T, dir string, lastUpdateTime time.Time, expected []nup.Song) {
	ch := make(chan SongAndError)
	num, err := scanForUpdatedSongs(dir, lastUpdateTime, ch, false)
	if err != nil {
		t.Errorf("scanning for songs failed")
	} else {
		test.CompareSongs(t, expected, getSongsFromChannel(t, ch, num))
	}
}

func TestScan(t *testing.T) {
	dir := test.CreateTempDir(t)
	defer os.RemoveAll(dir)

	test.CopySongToTempDir(t, dir, test.Song0s.Filename)
	test.CopySongToTempDir(t, dir, test.Song1s.Filename)
	startTime := time.Now()
	scanAndCompareSongs(t, dir, time.Time{}, []nup.Song{test.Song0s, test.Song1s})

	scanAndCompareSongs(t, dir, startTime, []nup.Song{})

	test.CopySongToTempDir(t, dir, test.Song5s.Filename)
	addTime := time.Now()
	scanAndCompareSongs(t, dir, startTime, []nup.Song{test.Song5s})

	test.DeleteFromTempDir(t, dir, test.Song0s.Filename)
	test.CopySongToTempDir(t, dir, test.Song0sUpdated.Filename)
	updateTime := time.Now()
	scanAndCompareSongs(t, dir, addTime, []nup.Song{test.Song0sUpdated})

	subdir := filepath.Join(dir, "foo")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatalf("unable to create subdirectory %v: %v", subdir, err)
	}
	renamedPath := filepath.Join(subdir, test.Song1s.Filename)
	if err := os.Rename(filepath.Join(dir, test.Song1s.Filename), renamedPath); err != nil {
		t.Fatalf("unable to move file to %v: %v", renamedPath, err)
	}
	now := time.Now()
	if err := os.Chtimes(renamedPath, now, now); err != nil {
		t.Fatalf("failed to set %v's modification time to %v", renamedPath, now)
	}
	renamedSong1s := test.Song1s
	renamedSong1s.Filename = filepath.Join(filepath.Base(subdir), test.Song1s.Filename)
	scanAndCompareSongs(t, dir, updateTime, []nup.Song{renamedSong1s})
}

// TODO: Test errors, skipping bogus files, etc.
