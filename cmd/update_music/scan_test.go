// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/derat/nup/internal/pkg/test"
	"github.com/derat/nup/server/types"
)

func scanAndCompareSongs(t *testing.T, desc, dir string, lastUpdateTime time.Time,
	opts *scanOptions, expected []types.Song) {
	if opts == nil {
		opts = &scanOptions{}
	}
	ch := make(chan types.SongOrErr)
	num, err := scanForUpdatedSongs(dir, lastUpdateTime, ch, opts)
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

func TestScanAndCompareSongs(t *testing.T) {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	test.CopySongsToTempDir(dir, test.Song0s.Filename, test.Song1s.Filename)
	startTime := time.Now()
	scanAndCompareSongs(t, "initial", dir, time.Time{}, nil, []types.Song{test.Song0s, test.Song1s})
	scanAndCompareSongs(t, "unchanged", dir, startTime, nil, []types.Song{})

	test.CopySongsToTempDir(dir, test.Song5s.Filename)
	addTime := time.Now()
	scanAndCompareSongs(t, "add", dir, startTime, nil, []types.Song{test.Song5s})

	if err = os.Remove(filepath.Join(dir, test.Song0s.Filename)); err != nil {
		panic(err)
	}
	test.CopySongsToTempDir(dir, test.Song0sUpdated.Filename)
	updateTime := time.Now()
	scanAndCompareSongs(t, "update", dir, addTime, nil, []types.Song{test.Song0sUpdated})

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
	scanAndCompareSongs(t, "rename", dir, updateTime, nil, []types.Song{renamedSong1s})

	scanAndCompareSongs(t, "force exact", dir, updateTime, &scanOptions{forceGlob: test.Song0sUpdated.Filename},
		[]types.Song{test.Song0sUpdated})
	scanAndCompareSongs(t, "force wildcard", dir, updateTime, &scanOptions{forceGlob: "foo/*"},
		[]types.Song{renamedSong1s})

	updateTime = time.Now()
	test.CopySongsToTempDir(dir, test.ID3V1Song.Filename)
	scanAndCompareSongs(t, "id3v1", dir, updateTime, nil, []types.Song{test.ID3V1Song})
}

func TestScanAndCompareSongs_Rewrite(t *testing.T) {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	const newArtist = "Rewritten Artist"
	newSong1s := test.Song1s
	newSong1s.Artist = newArtist

	test.CopySongsToTempDir(dir, test.Song1s.Filename, test.Song5s.Filename)
	scanAndCompareSongs(t, "initial", dir, time.Time{},
		&scanOptions{artistRewrites: map[string]string{test.Song1s.Artist: newSong1s.Artist}},
		[]types.Song{newSong1s, test.Song5s})
}

// TODO: Test errors, skipping bogus files, etc.
