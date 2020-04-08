package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/derat/nup/test"
	"github.com/derat/nup/types"
)

func scanAndCompareSongs(t *testing.T, desc, dir, forceGlob string, lastUpdateTime time.Time, expected []types.Song) {
	ch := make(chan types.SongOrErr)
	num, err := scanForUpdatedSongs(dir, forceGlob, lastUpdateTime, ch, false)
	if err != nil {
		t.Errorf("%v: %v", desc, err)
		return
	}
	actual, err := test.GetSongsFromChannel(ch, num)
	if err != nil {
		t.Errorf("%v: %v", desc, err)
		return
	}
	for i := range expected {
		found := false
		for j := range actual {
			if expected[i].Filename == actual[j].Filename {
				found = true
				if expected[i].RecordingID != actual[j].RecordingID {
					t.Errorf("%v: song %v didn't have expected recording id: expected %q, actual %q",
						desc, i, expected[i].RecordingID, actual[j].RecordingID)
					return
				}
				expected[i].Rating = 0
				break
			}
		}
		if !found {
			t.Errorf("%v: didn't get song %v", desc, i)
		}
	}
	if err = test.CompareSongs(expected, actual, test.IgnoreOrder); err != nil {
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
	scanAndCompareSongs(t, "initial", dir, "", time.Time{}, []types.Song{test.Song0s, test.Song1s})
	scanAndCompareSongs(t, "unchanged", dir, "", startTime, []types.Song{})

	test.CopySongsToTempDir(dir, test.Song5s.Filename)
	addTime := time.Now()
	scanAndCompareSongs(t, "add", dir, "", startTime, []types.Song{test.Song5s})

	if err = os.Remove(filepath.Join(dir, test.Song0s.Filename)); err != nil {
		panic(err)
	}
	test.CopySongsToTempDir(dir, test.Song0sUpdated.Filename)
	updateTime := time.Now()
	scanAndCompareSongs(t, "update", dir, "", addTime, []types.Song{test.Song0sUpdated})

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
	scanAndCompareSongs(t, "rename", dir, "", updateTime, []types.Song{renamedSong1s})

	scanAndCompareSongs(t, "force exact", dir, test.Song0sUpdated.Filename, updateTime, []types.Song{test.Song0sUpdated})
	scanAndCompareSongs(t, "force wildcard", dir, "foo/*", updateTime, []types.Song{renamedSong1s})

	updateTime = time.Now()
	test.CopySongsToTempDir(dir, test.Id3v1Song.Filename)
	scanAndCompareSongs(t, "id3v1", dir, "", updateTime, []types.Song{test.Id3v1Song})
}

// TODO: Test errors, skipping bogus files, etc.
