// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

// getSongsFromChannel reads and returns num songs from ch.
// If an error was sent to the channel, it is returned.
func getSongsFromChannel(ch chan songOrErr, num int) ([]db.Song, error) {
	var songs []db.Song
	for i := 0; i < num; i++ {
		s := <-ch
		if s.err != nil {
			return nil, fmt.Errorf("got error at position %v instead of song: %v", i, s.err)
		}
		songs = append(songs, *s.song)
	}
	return songs, nil
}

type scanTestOptions struct {
	metadataDir     string            // client.Config.MetadataDir
	artistRewrites  map[string]string // client.Config.ArtistRewrites
	albumIDRewrites map[string]string // client.Config.AlbumIDRewrites
	lastUpdateDirs  []string          // scanForUpdatedSongs lastUpdateDirs param
	forceGlob       string            // scanOptions.forceGlob
}

func scanAndCompareSongs(t *testing.T, desc, dir string, lastUpdateTime time.Time,
	testOpts *scanTestOptions, expected []db.Song) (dirs []string) {
	cfg := &client.Config{MusicDir: dir}
	opts := &scanOptions{}
	var lastUpdateDirs []string
	if testOpts != nil {
		cfg.MetadataDir = testOpts.metadataDir
		cfg.ArtistRewrites = testOpts.artistRewrites
		cfg.AlbumIDRewrites = testOpts.albumIDRewrites
		opts.forceGlob = testOpts.forceGlob
		lastUpdateDirs = testOpts.lastUpdateDirs
	}
	ch := make(chan songOrErr)
	num, dirs, err := scanForUpdatedSongs(cfg, lastUpdateTime, lastUpdateDirs, ch, opts)
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
				expected[i].TrackGain = 0
				expected[i].AlbumGain = 0
				expected[i].PeakAmp = 0
				break
			}
		}
		if !found {
			t.Errorf("%v: didn't get song %v", desc, i)
		}
	}
	if err := test.CompareSongs(expected, actual, test.IgnoreOrder); err != nil {
		t.Errorf("%v: %v", desc, err)
	}
	return dirs
}

func TestScanAndCompareSongs(t *testing.T) {
	dir := t.TempDir()
	test.Must(t, test.CopySongs(dir, test.Song0s.Filename, test.Song1s.Filename))
	startTime := time.Now()
	scanAndCompareSongs(t, "initial", dir, time.Time{}, nil, []db.Song{test.Song0s, test.Song1s})
	scanAndCompareSongs(t, "unchanged", dir, startTime, nil, []db.Song{})

	test.Must(t, test.CopySongs(dir, test.Song5s.Filename))
	addTime := time.Now()
	scanAndCompareSongs(t, "add", dir, startTime, nil, []db.Song{test.Song5s})

	if err := os.Remove(filepath.Join(dir, test.Song0s.Filename)); err != nil {
		t.Fatal("Failed removing song: ", err)
	}
	test.Must(t, test.CopySongs(dir, test.Song0sUpdated.Filename))
	updateTime := time.Now()
	scanAndCompareSongs(t, "update", dir, addTime, nil, []db.Song{test.Song0sUpdated})

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
	scanAndCompareSongs(t, "rename", dir, updateTime, nil, []db.Song{renamedSong1s})

	scanAndCompareSongs(t, "force exact", dir, updateTime,
		&scanTestOptions{forceGlob: test.Song0sUpdated.Filename},
		[]db.Song{test.Song0sUpdated})
	scanAndCompareSongs(t, "force wildcard", dir, updateTime,
		&scanTestOptions{forceGlob: "foo/*"},
		[]db.Song{renamedSong1s})

	updateTime = time.Now()
	test.Must(t, test.CopySongs(dir, test.ID3V1Song.Filename))
	scanAndCompareSongs(t, "id3v1", dir, updateTime, nil, []db.Song{test.ID3V1Song})
}

func TestScanAndCompareSongs_Rewrite(t *testing.T) {
	dir := t.TempDir()

	newSong1s := test.Song1s
	newSong1s.Artist = "Rewritten Artist"

	// The album name, disc number, and disc subtitle should all be derived from
	// the original "Another Album (disc 3: The Third Disc)" album name.
	newSong5s := test.Song5s
	newSong5s.Album = "Another Album"
	newSong5s.AlbumID = "bf7ff94c-2a6a-4357-a30e-71da8c117ebc"
	newSong5s.CoverID = test.Song5s.AlbumID
	newSong5s.Disc = 3
	newSong5s.DiscSubtitle = "The Third Disc"

	// This also verifies that Song10s's TSST frame is used to fill DiscSubtitle.
	test.Must(t, test.CopySongs(dir, test.Song1s.Filename, test.Song5s.Filename, test.Song10s.Filename))
	opts := &scanTestOptions{
		artistRewrites:  map[string]string{test.Song1s.Artist: newSong1s.Artist},
		albumIDRewrites: map[string]string{test.Song5s.AlbumID: newSong5s.AlbumID},
	}
	scanAndCompareSongs(t, "initial", dir, time.Time{}, opts, []db.Song{newSong1s, newSong5s, test.Song10s})
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
	origDirs := scanAndCompareSongs(t, "initial", musicDir, time.Time{}, nil,
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
	waitCtime(t, dir, startTime)
	mv(filepath.Join(dir, test.Song1s.Filename),
		filepath.Join(musicDir, oldArtist, oldAlbum, test.Song1s.Filename))
	mv(filepath.Join(dir, newAlbum),
		filepath.Join(musicDir, oldArtist, newAlbum))
	mv(filepath.Join(dir, newArtist), filepath.Join(musicDir, newArtist))

	// All three of the new songs should be seen.
	updateTime := time.Now()
	newDirs := scanAndCompareSongs(t, "updated", musicDir, startTime,
		&scanTestOptions{lastUpdateDirs: origDirs},
		[]db.Song{
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
	newDirs = scanAndCompareSongs(t, "rescan", musicDir, updateTime,
		&scanTestOptions{lastUpdateDirs: newDirs}, []db.Song{})
	if !reflect.DeepEqual(newDirs, allDirs) {
		t.Errorf("scanAndCompareSongs(...) = %v; want %v", newDirs, allDirs)
	}
}

func TestScanAndCompareSongs_OverrideMetadata(t *testing.T) {
	td := t.TempDir()
	musicDir := filepath.Join(td, "music")
	test.Must(t, test.CopySongs(musicDir, test.Song1s.Filename))

	metadataDir := filepath.Join(td, "metadata")
	cfg := &client.Config{MusicDir: musicDir, MetadataDir: metadataDir}
	opts := &scanTestOptions{metadataDir: metadataDir}

	// Perform an initial scan to pick up the song.
	startTime := time.Now()
	scanAndCompareSongs(t, "initial", musicDir, time.Time{}, opts, []db.Song{test.Song1s})

	// Write a file to override the song's metadata.
	updated := test.Song1s
	updated.Artist = "New Artist"
	updated.Title = "New Title"
	waitCtime(t, metadataDir, startTime)
	test.Must(t, files.UpdateMetadataOverride(cfg, &updated))

	// The next scan should pick up the new metadata even though the song file wasn't updated.
	overrideTime := time.Now()
	scanAndCompareSongs(t, "wrote override", musicDir, startTime, opts, []db.Song{updated})

	// Clear the override file and scan again to go back to the original metadata.
	waitCtime(t, metadataDir, overrideTime)
	test.Must(t, files.UpdateMetadataOverride(cfg, &test.Song1s))
	clearTime := time.Now()
	scanAndCompareSongs(t, "cleared override", musicDir, overrideTime, opts, []db.Song{test.Song1s})

	// Nothing should happen after doing a scan without any changes.
	scanAndCompareSongs(t, "no change", musicDir, clearTime, opts, []db.Song{})
}

// TODO: Test errors, skipping bogus files, etc.

// waitCtime waits until the kernel is assigning ctimes after ref to new files in dir.
// This is super-cheesy, but ctimes (and mtimes?) appear to get rounded, so it's sometimes
// (often) the case that a newly-created file's ctime/mtime will precede the time returned
// by an earlier time.Now() call.
func waitCtime(t *testing.T, dir string, ref time.Time) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for {
		if func() time.Time {
			f, err := ioutil.TempFile(dir, "temp.")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(f.Name())
			defer f.Close()
			fi, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			return getCtime(fi)
		}().After(ref) {
			break
		}
		time.Sleep(time.Millisecond)
	}
}
