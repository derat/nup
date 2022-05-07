// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
	"github.com/derat/taglib-go/taglib"
)

const (
	albumIDTag          = "MusicBrainz Album Id" // usually used as cover ID
	coverIDTag          = "nup Cover Id"         // can be set for non-MusicBrainz tracks
	albumArtistTag      = "TPE2"                 // "Band/Orchestra/Accompaniment"
	recordingIDOwner    = "http://musicbrainz.org"
	nonAlbumTracksValue = "[non-album tracks]" // MusicBrainz/Picard album name

	maxScanWorkers      = 8 // maximum number of songs to read at once
	logProgressInterval = 100
)

// computeAudioSHA1 returns a SHA1 hash of the audio (i.e. non-metadata) portion of f.
func computeAudioSHA1(f *os.File, fi os.FileInfo, headerLen, footerLen int64) (string, error) {
	if _, err := f.Seek(headerLen, 0); err != nil {
		return "", err
	}
	hasher := sha1.New()
	if _, err := io.CopyN(hasher, f, fi.Size()-headerLen-footerLen); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// readSong creates a Song for the file at the supplied path.
func readSong(path, relPath string, fi os.FileInfo, opts *scanOptions, gains *gainsCache) (*db.Song, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := db.Song{Filename: relPath}
	var footerLen int64
	footerLen, s.Artist, s.Title, s.Album, err = readID3v1Footer(f, fi)
	if err != nil {
		return nil, err
	}

	var headerLen int64
	if tag, err := taglib.Decode(f, fi.Size()); err != nil {
		// Tolerate missing ID3v2 tags if we got an artist and title from ID3v1.
		if len(s.Artist) == 0 && len(s.Title) == 0 {
			return nil, err
		}
	} else {
		s.Artist = tag.Artist()
		s.Title = tag.Title()
		s.Album = tag.Album()
		s.AlbumID = tag.CustomFrames()[albumIDTag]
		s.CoverID = tag.CustomFrames()[coverIDTag]
		s.RecordingID = tag.UniqueFileIdentifiers()[recordingIDOwner]
		s.Track = int(tag.Track())
		s.Disc = int(tag.Disc())
		headerLen = int64(tag.TagSize())

		// Only save the album artist if it's different from the track artist.
		if aa, err := getID3v2TextFrame(tag, albumArtistTag); err != nil {
			return nil, err
		} else if aa != s.Artist {
			s.AlbumArtist = aa
		}

		// Some old files might be missing the TPOS "part of set" frame.
		// Assume that they're from a single-disc album in that case:
		// https://github.com/derat/nup/issues/37
		if s.Disc == 0 && s.Track > 0 && s.Album != nonAlbumTracksValue {
			s.Disc = 1
		}
	}

	if repl, ok := opts.artistRewrites[s.Artist]; ok {
		s.Artist = repl
	}

	s.SHA1, err = computeAudioSHA1(f, fi, headerLen, footerLen)
	if err != nil {
		return nil, err
	}
	dur, _, _, err := computeAudioDuration(f, fi, headerLen, footerLen)
	if err != nil {
		return nil, err
	}
	s.Length = dur.Seconds()

	if opts.computeGain {
		gain, err := gains.get(path, s.Album, s.AlbumID)
		if err != nil {
			return nil, err
		}
		s.TrackGain = gain.TrackGain
		s.AlbumGain = gain.AlbumGain
		s.PeakAmp = gain.PeakAmp
	}

	return &s, nil
}

// readSongList reads a list of relative (to musicDir) paths from listPath
// and asynchronously sends the resulting Song structs to ch.
// The number of songs that will be sent to the channel is returned.
func readSongList(listPath, musicDir string, ch chan songOrErr, opts *scanOptions) (numSongs int, err error) {
	f, err := os.Open(listPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Read the list synchronously first to get the number of songs.
	var paths []string // relative paths
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		paths = append(paths, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}

	gains, err := newGainsCache(opts.dumpedGainsPath, musicDir)
	if err != nil {
		return 0, err
	}

	// Now read the files asynchronously (but one at a time).
	go func() {
		for _, rel := range paths {
			full := filepath.Join(musicDir, rel)
			if fi, err := os.Stat(full); err != nil {
				ch <- songOrErr{nil, err}
			} else {
				s, err := readSong(full, rel, fi, opts, gains)
				ch <- songOrErr{s, err}
			}
		}
	}()

	return len(paths), nil
}

// scanOptions contains options for scanForUpdatedSongs and readSongList.
// Some of the options aren't used by readSongList.
type scanOptions struct {
	computeGain     bool              // use mp3gain to compute gain adjustments
	forceGlob       string            // glob matching files to update even if unchanged
	logProgress     bool              // periodically log progress while scanning
	artistRewrites  map[string]string // artist names from tags to rewrite
	dumpedGainsPath string            // file with JSON-marshaled db.Song objects
}

// scanForUpdatedSongs looks for songs under musicDir updated more recently than lastUpdateTime or
// in directories not listed in lastUpdateDirs and asynchronously sends the resulting Song structs
// to ch. The number of songs that will be sent to the channel and seen directories (relative to
// musicDir) are returned.
func scanForUpdatedSongs(musicDir string, lastUpdateTime time.Time, lastUpdateDirs []string,
	ch chan songOrErr, opts *scanOptions) (numUpdates int, seenDirs []string, err error) {
	var numMP3s int // total number of songs under musicDir

	oldDirs := make(map[string]struct{}, len(lastUpdateDirs))
	for _, d := range lastUpdateDirs {
		oldDirs[d] = struct{}{}
	}
	newDirs := make(map[string]struct{})

	gains, err := newGainsCache(opts.dumpedGainsPath, musicDir)
	if err != nil {
		return 0, nil, err
	}

	workers := make(chan struct{}, maxScanWorkers)
	if err := filepath.Walk(musicDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() || !client.IsMusicPath(path) {
			return nil
		}
		relPath, err := filepath.Rel(musicDir, path)
		if err != nil {
			return fmt.Errorf("%q isn't subpath of %q: %v", path, musicDir, err)
		}

		numMP3s++
		if opts.logProgress && numMP3s%logProgressInterval == 0 {
			log.Printf("Scanned %v files", numMP3s)
		}

		relDir := filepath.Dir(relPath)
		newDirs[relDir] = struct{}{}

		if opts.forceGlob != "" {
			if matched, err := filepath.Match(opts.forceGlob, relPath); err != nil {
				return fmt.Errorf("invalid glob %q: %v", opts.forceGlob, err)
			} else if !matched {
				return nil
			}
		} else {
			// Bail out if the file isn't new and we saw its directory in the last update.
			// We need to check for new directories to handle the situation described at
			// https://github.com/derat/nup/issues/22 where a directory containing files
			// with old timestamps is moved into the tree.
			stat := fi.Sys().(*syscall.Stat_t)
			ctime := time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
			oldFile := fi.ModTime().Before(lastUpdateTime) && ctime.Before(lastUpdateTime)
			_, oldDir := oldDirs[relDir]

			// Handle old configs that don't include previously-seen directories.
			if len(oldDirs) == 0 {
				oldDir = true
			}

			if oldFile && oldDir {
				return nil
			}
		}

		go func() {
			// Avoid having too many parallel readSong calls, as we can run out of FDs.
			workers <- struct{}{}
			s, err := readSong(path, relPath, fi, opts, gains)
			<-workers
			ch <- songOrErr{s, err}
		}()

		numUpdates++
		return nil
	}); err != nil {
		return 0, nil, err
	}

	if opts.logProgress {
		log.Printf("Found %v update(s) among %v files", numUpdates, numMP3s)
	}
	for d := range newDirs {
		seenDirs = append(seenDirs, d)
	}
	sort.Strings(seenDirs)
	return numUpdates, seenDirs, nil
}
