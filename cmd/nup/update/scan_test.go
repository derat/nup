// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

// getSongsFromChannel reads and returns num songs from ch.
// If an error was sent to the channel, it is returned.
func getSongsFromChannel(ch chan songOrErr, num int) ([]db.Song, error) {
	songs := make([]db.Song, 0)
	for i := 0; i < num; i++ {
		s := <-ch
		if s.err != nil {
			return nil, fmt.Errorf("got error at position %v instead of song: %v", i, s.err)
		}
		songs = append(songs, *s.song)
	}
	return songs, nil
}

func scanAndCompareSongs(t *testing.T, desc, dir string, lastUpdateTime time.Time,
	lastUpdateDirs []string, opts *scanOptions, expected []db.Song) (dirs []string) {
	if opts == nil {
		opts = &scanOptions{}
	}
	ch := make(chan songOrErr)
	num, dirs, err := scanForUpdatedSongs(dir, lastUpdateTime, lastUpdateDirs, ch, opts)
	if err != nil {
		t.Errorf("%v: %v", desc, err)
		return dirs
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		t.Errorf("%v: %v", desc, err)
		return dirs
	}
	for i := range expected {
		found := false
		for j := range actual {
			if expected[i].Filename == actual[j].Filename {
				found = true
				if expected[i].RecordingID != actual[j].RecordingID {
					t.Errorf("%v: song %v didn't have expected recording id: expected %q, actual %q",
						desc, i, expected[i].RecordingID, actual[j].RecordingID)
					return dirs
				}
				expected[i].Rating = 0
				if !opts.computeGain {
					expected[i].TrackGain = 0
					expected[i].AlbumGain = 0
					expected[i].PeakAmp = 0
				}
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
	return dirs
}

func TestScanAndCompareSongs(t *testing.T) {
	dir := t.TempDir()
	test.Must(t, test.CopySongs(dir, test.Song0s.Filename, test.Song1s.Filename))
	startTime := time.Now()
	scanAndCompareSongs(t, "initial", dir, time.Time{}, nil, nil, []db.Song{test.Song0s, test.Song1s})
	scanAndCompareSongs(t, "unchanged", dir, startTime, nil, nil, []db.Song{})

	test.Must(t, test.CopySongs(dir, test.Song5s.Filename))
	addTime := time.Now()
	scanAndCompareSongs(t, "add", dir, startTime, nil, nil, []db.Song{test.Song5s})

	if err := os.Remove(filepath.Join(dir, test.Song0s.Filename)); err != nil {
		t.Fatal("Failed removing song: ", err)
	}
	test.Must(t, test.CopySongs(dir, test.Song0sUpdated.Filename))
	updateTime := time.Now()
	scanAndCompareSongs(t, "update", dir, addTime, nil, nil, []db.Song{test.Song0sUpdated})

	subdir := filepath.Join(dir, "foo")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal("Failed making subdir: ", err)
	}
	renamedPath := filepath.Join(subdir, test.Song1s.Filename)
	if err := os.Rename(filepath.Join(dir, test.Song1s.Filename), renamedPath); err != nil {
		t.Fatal("Failed renaming song: ", err)
	}
	now := time.Now()
	if err := os.Chtimes(renamedPath, now, now); err != nil {
		t.Fatal("Failed setting times: ", err)
	}
	renamedSong1s := test.Song1s
	renamedSong1s.Filename = filepath.Join(filepath.Base(subdir), test.Song1s.Filename)
	scanAndCompareSongs(t, "rename", dir, updateTime, nil, nil, []db.Song{renamedSong1s})

	scanAndCompareSongs(t, "force exact", dir, updateTime, nil, &scanOptions{forceGlob: test.Song0sUpdated.Filename},
		[]db.Song{test.Song0sUpdated})
	scanAndCompareSongs(t, "force wildcard", dir, updateTime, nil, &scanOptions{forceGlob: "foo/*"},
		[]db.Song{renamedSong1s})

	updateTime = time.Now()
	test.Must(t, test.CopySongs(dir, test.ID3V1Song.Filename))
	scanAndCompareSongs(t, "id3v1", dir, updateTime, nil, nil, []db.Song{test.ID3V1Song})
}

func TestScanAndCompareSongs_Rewrite(t *testing.T) {
	dir := t.TempDir()
	const newArtist = "Rewritten Artist"
	newSong1s := test.Song1s
	newSong1s.Artist = newArtist

	test.Must(t, test.CopySongs(dir, test.Song1s.Filename, test.Song5s.Filename))
	scanAndCompareSongs(t, "initial", dir, time.Time{}, nil,
		&scanOptions{artistRewrites: map[string]string{test.Song1s.Artist: newSong1s.Artist}},
		[]db.Song{newSong1s, test.Song5s})
}

func TestScanAndCompareSongs_NewFiles(t *testing.T) {
	dir := t.TempDir()
	const (
		oldArtist = "old_artist"
		oldAlbum  = "old_album"
		newAlbum  = "new_album"
		newArtist = "new_artist"
	)

	// Start out with an artist/album directory containing a single song.
	musicDir := filepath.Join(dir, "music")
	test.Must(t, test.CopySongs(filepath.Join(musicDir, oldArtist, oldAlbum), test.Song0s.Filename))

	// Copy some more songs into the temp dir to give them old timestamps,
	// but don't move them under the music dir yet.
	test.Must(t, test.CopySongs(dir, test.Song1s.Filename))
	test.Must(t, test.CopySongs(filepath.Join(dir, newAlbum), test.Song5s.Filename))
	test.Must(t, test.CopySongs(filepath.Join(dir, newArtist, newAlbum), test.ID3V1Song.Filename))

	// Updates the supplied song's filename to be under dir.
	gs := func(s db.Song, dir string) db.Song {
		s.Filename = filepath.Join(dir, s.Filename)
		return s
	}

	startTime := time.Now()
	origDirs := scanAndCompareSongs(t, "initial", musicDir, time.Time{}, nil, nil,
		[]db.Song{gs(test.Song0s, filepath.Join(oldArtist, oldAlbum))})
	if want := []string{filepath.Join(oldArtist, oldAlbum)}; !reflect.DeepEqual(origDirs, want) {
		t.Errorf("scanAndCompareSongs(...) = %v; want %v", origDirs, want)
	}

	// Move the new files into various locations under the music dir.
	mv := func(src, dst string) {
		if err := os.Rename(src, dst); err != nil {
			t.Fatal("Failed renaming file: ", err)
		}
	}

	// This sleep statement is really gross, but it seems like without it, the rename is often
	// (always?) fast enough that Song1s's ctime doesn't actually change during the rename,
	// resulting in it not getting picked up by the second scan. I initially set this to 1 ms, but
	// it was still flaky, so here we are...
	time.Sleep(10 * time.Millisecond)

	mv(filepath.Join(dir, test.Song1s.Filename),
		filepath.Join(musicDir, oldArtist, oldAlbum, test.Song1s.Filename))
	mv(filepath.Join(dir, newAlbum),
		filepath.Join(musicDir, oldArtist, newAlbum))
	mv(filepath.Join(dir, newArtist), filepath.Join(musicDir, newArtist))

	// All three of the new songs should be seen.
	updateTime := time.Now()
	newDirs := scanAndCompareSongs(t, "updated", musicDir, startTime, origDirs, nil, []db.Song{
		gs(test.Song1s, filepath.Join(oldArtist, oldAlbum)),
		gs(test.Song5s, filepath.Join(oldArtist, newAlbum)),
		gs(test.ID3V1Song, filepath.Join(newArtist, newAlbum)),
	})
	allDirs := []string{
		filepath.Join(newArtist, newAlbum),
		filepath.Join(oldArtist, newAlbum),
		filepath.Join(oldArtist, oldAlbum),
	}
	if !reflect.DeepEqual(newDirs, allDirs) {
		t.Errorf("scanAndCompareSongs(...) = %v; want %v", newDirs, allDirs)
	}

	// Do one more scan and check that no songs are returned.
	newDirs = scanAndCompareSongs(t, "updated", musicDir, updateTime, newDirs, nil, []db.Song{})
	if !reflect.DeepEqual(newDirs, allDirs) {
		t.Errorf("scanAndCompareSongs(...) = %v; want %v", newDirs, allDirs)
	}
}

// TODO: Test errors, skipping bogus files, etc.