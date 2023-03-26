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
	"syscall"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/server/db"
)

const (
	// Maximum number of songs to read at once. This needs to not be too high to avoid
	// running out of FDs when readSong calls block on computing gain adjustments (my system
	// has a default soft limit of 1024 per "ulimit -Sn"), but it should also be high enough
	// that we're processing songs from different albums simultaneously so we can run
	// multiple copies of mp3gain in parallel on multicore systems.
	maxScanWorkers = 64

	logProgressInterval = 100
)

// readSongList reads a list of relative (to cfg.MusicDir) paths from listPath
// and asynchronously sends the resulting Song structs to ch.
// The number of songs that will be sent to the channel is returned.
func readSongList(cfg *client.Config, listPath string, ch chan songOrErr,
	opts *scanOptions) (numSongs int, err error) {
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

	gains, err := files.NewGainsCache(cfg, opts.dumpedGainsPath)
	if err != nil {
		return 0, err
	}

	// Now read the files asynchronously (but one at a time).
	// TODO: Consider reading multiple songs simultaneously as in scanForUpdatedSongs
	// so that gain calculation is parallelized.
	go func() {
		for _, rel := range paths {
			full := filepath.Join(cfg.MusicDir, rel)
			s, err := files.ReadSong(cfg, full, nil, false, gains)
			ch <- songOrErr{s, err}
		}
	}()

	return len(paths), nil
}

// scanOptions contains options for scanForUpdatedSongs and readSongList.
// Some of the options aren't used by readSongList.
type scanOptions struct {
	forceGlob       string // glob matching files to update even if unchanged
	logProgress     bool   // periodically log progress while scanning
	dumpedGainsPath string // file with JSON-marshaled db.Song objects
}

// scanForUpdatedSongs looks for songs under cfg.MusicDir updated more recently than lastUpdateTime or
// in directories not listed in lastUpdateDirs and asynchronously sends the resulting Song structs
// to ch. The number of songs that will be sent to the channel and seen directories (relative to
// musicDir) are returned.
func scanForUpdatedSongs(cfg *client.Config, lastUpdateTime time.Time, lastUpdateDirs []string,
	ch chan songOrErr, opts *scanOptions) (numUpdates int, seenDirs []string, err error) {
	var numSongs int // total number of songs under cfg.MusicDir

	oldDirs := make(map[string]struct{}, len(lastUpdateDirs))
	for _, d := range lastUpdateDirs {
		oldDirs[d] = struct{}{}
	}
	newDirs := make(map[string]struct{})

	gains, err := files.NewGainsCache(cfg, opts.dumpedGainsPath)
	if err != nil {
		return 0, nil, err
	}

	workers := make(chan struct{}, maxScanWorkers)
	if err := filepath.Walk(cfg.MusicDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() || !files.IsMusicPath(path) {
			return nil
		}
		relPath, err := filepath.Rel(cfg.MusicDir, path)
		if err != nil {
			return fmt.Errorf("%q isn't subpath of %q: %v", path, cfg.MusicDir, err)
		}

		numSongs++
		if opts.logProgress && numSongs%logProgressInterval == 0 {
			log.Printf("Scanned %v files", numSongs)
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
			oldFile := fi.ModTime().Before(lastUpdateTime) && getCtime(fi).Before(lastUpdateTime)
			_, oldDir := oldDirs[relDir]

			// Handle old configs that don't include previously-seen directories.
			if len(oldDirs) == 0 {
				oldDir = true
			}

			// Also check if an updated metadata override file exists.
			// TODO: If override files are also used to add synthetic songs for
			// https://github.com/derat/nup/issues/32, then scanForUpdatedSongs will need to
			// scan all of cfg.MetadataDir while also avoiding duplicate updates in the case
			// where both the song file and the corresponding override file have been updated.
			var newMetadata bool
			if mp, err := files.MetadataOverridePath(cfg, relPath); err == nil {
				if mfi, err := os.Stat(mp); err == nil {
					// TODO: This check is somewhat incorrect since it doesn't include the oldDirs
					// trickiness used above for song files. More worryingly, a song won't be
					// rescanned if its override file is deleted. I guess override files should be
					// set to "{}" instead of being deleted. The only other alternative seems to be
					// listing all known override files within cfg.LastUpdateInfoFile.
					newMetadata = !(mfi.ModTime().Before(lastUpdateTime) && getCtime(mfi).Before(lastUpdateTime))
				}
			}

			if oldFile && oldDir && !newMetadata {
				return nil
			}
		}

		go func() {
			// Avoid having too many parallel readSong calls, as we can run out of FDs.
			workers <- struct{}{}
			s, err := files.ReadSong(cfg, path, fi, false, gains)
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
		log.Printf("Found %v update(s) among %v files", numUpdates, numSongs)
	}
	for d := range newDirs {
		seenDirs = append(seenDirs, d)
	}
	sort.Strings(seenDirs)
	return numUpdates, seenDirs, nil
}

// getCtime returns fi's ctime (i.e. when its metadata was last changed).
func getCtime(fi os.FileInfo) time.Time {
	stat := fi.Sys().(*syscall.Stat_t)
	return time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
}
