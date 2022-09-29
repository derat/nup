// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/mpeg"
	"github.com/derat/nup/server/db"
	"github.com/derat/taglib-go/taglib"
)

const (
	albumIDTag          = "MusicBrainz Album Id"   // usually used as cover ID
	coverIDTag          = "nup Cover Id"           // can be set for non-MusicBrainz tracks
	recordingIDOwner    = "http://musicbrainz.org" // UFID for Song.RecordingID
	nonAlbumTracksValue = "[non-album tracks]"     // MusicBrainz/Picard album name

	// Maximum number of songs to read at once. This needs to not be too high to avoid
	// running out of FDs when readSong calls block on computing gain adjustments (my system
	// has a default soft limit of 1024 per "ulimit -Sn"), but it should also be high enough
	// that we're processing songs from different albums simultaneously so we can run
	// multiple copies of mp3gain in parallel on multicore systems.
	maxScanWorkers = 64

	logProgressInterval = 100
)

// readSong creates a Song for the file at the supplied path.
// If onlyTags is true, only fields derived from the file's MP3 tags will be filled.
func readSong(path, relPath string, fi os.FileInfo, onlyTags bool,
	opts *scanOptions, gains *gainsCache) (*db.Song, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := db.Song{Filename: relPath}

	var headerLen, footerLen int64

	if tag, err := mpeg.ReadID3v1Footer(f, fi); err != nil {
		return nil, err
	} else if tag != nil {
		footerLen = mpeg.ID3v1Length
		s.Artist = tag.Artist
		s.Title = tag.Title
		s.Album = tag.Album
		if year, err := strconv.Atoi(tag.Year); err == nil {
			s.Date = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		}
	}

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

		if date, err := getSongDate(func(id string) (string, error) {
			return mpeg.GetID3v2TextFrame(tag, id)
		}); err != nil {
			return nil, err
		} else if !date.IsZero() {
			s.Date = date
		}

		// ID3 v2.4 defines TPE2 (Band/orchestra/accompaniment) as
		// "additional information about the performers in the recording".
		// Only save the album artist if it's different from the track artist.
		if aa, err := mpeg.GetID3v2TextFrame(tag, "TPE2"); err != nil {
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

	if onlyTags {
		return &s, nil
	}

	if repl, ok := opts.artistRewrites[s.Artist]; ok {
		s.Artist = repl
	}

	s.SHA1, err = mpeg.ComputeAudioSHA1(f, fi, headerLen, footerLen)
	if err != nil {
		return nil, err
	}
	dur, _, _, err := mpeg.ComputeAudioDuration(f, fi, headerLen, footerLen)
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

// getSongDate tries to extract a song's release or recording date.
// getFrame should call mpeg.GetID3v2TextFrame; it's injected for tests.
func getSongDate(getFrame func(id string) (string, error)) (time.Time, error) {
	// There are a bunch of different date-related ID3 frames:
	// https://github.com/derat/nup/issues/42
	//
	// ID3 v2.4:
	//   TDOR (Original release time): timestamp describing when the original recording of the audio was released
	//   TDRC (Recording time): timestamp describing when the audio was recorded
	//   TDRL (Release time): timestamp describing when the audio was first released
	//
	// ID3 v2.3:
	//   TYER (Year): numeric string with a year of the recording (always 4 characters)
	//   TDAT (Date): numeric string in the DDMM format containing the date for the recording

	// Look for v2.4 frames in order of descending preference.
	for _, id := range []string{"TDOR", "TDRC", "TDRL"} {
		val, err := getFrame(id)
		if err != nil {
			return time.Time{}, err
		} else if len(val) < 4 {
			continue
		}

		// Section 4 of https://id3.org/id3v2.4.0-structure:
		//   The timestamp fields are based on a subset of ISO 8601. When being as precise as
		//   possible the format of a time string is yyyy-MM-ddTHH:mm:ss (year, "-", month, "-",
		//   day, "T", hour (out of 24), ":", minutes, ":", seconds), but the precision may be
		//   reduced by removing as many time indicators as wanted. Hence valid timestamps are yyyy,
		//   yyyy-MM, yyyy-MM-dd, yyyy-MM-ddTHH, yyyy-MM-ddTHH:mm and yyyy-MM-ddTHH:mm:ss. All time
		//   stamps are UTC. For durations, use the slash character as described in 8601, and for
		//   multiple non-contiguous dates, use multiple strings, if allowed by the frame
		//   definition.
		for _, layout := range []string{
			"2006-01-02T15:04:05",
			"2006-01-02T15:04",
			"2006-01-02T15",
			"2006-01-02",
			"2006-01",
			"2006",
		} {
			if t, err := time.Parse(layout, val); err == nil {
				return t, nil
			}
		}
	}

	// Fall back to v2.3.
	if y, err := getFrame("TYER"); err != nil {
		return time.Time{}, err
	} else if len(y) == 4 {
		if md, err := getFrame("TDAT"); err != nil {
			return time.Time{}, err
		} else if len(md) == 4 {
			if t, err := time.Parse("20060102", y+md); err == nil {
				return t, nil
			}
		}
		if t, err := time.Parse("2006", y); err == nil {
			return t, nil
		}
	}

	return time.Time{}, nil
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
				s, err := readSong(full, rel, fi, false, opts, gains)
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
			s, err := readSong(path, relPath, fi, false, opts, gains)
			<-workers
			if err != nil && s == nil {
				s = &db.Song{Filename: relPath} // return the filename for error reporting
			}
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
