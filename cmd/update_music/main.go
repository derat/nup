// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/derat/nup/mp3gain"
	"github.com/derat/nup/server/types"
)

type config struct {
	types.ClientConfig

	// CoverDir is the base directory containing cover art.
	CoverDir string `json:"coverDir"`
	// MusicDir is the base directory containing song files.
	MusicDir string `json:"musicDir"`
	// LastUpdateInfoFile is the path to a JSON file storing info about the last update.
	// The file will be created if it does not already exist.
	LastUpdateInfoFile string `json:"lastUpdateInfoFile"`
	// ComputeGain indicates whether the mp3gain program should be used to compute per-song
	// and per-album gain information so that volume can be normalized during playback.
	ComputeGain bool `json:"computeGain"`
	// ArtistRewrites is a map from original ID3 tag artist names to replacement names that should
	// be used for updates. This can be used to fix incorrectly-tagged files without needing to
	// reupload them.
	ArtistRewrites map[string]string `json:"artistRewrites"`
}

func main() {
	configFile := flag.String("config", "", "Path to config file")
	deleteSongID := flag.Int64("delete-song-id", 0, "Delete song with given ID")
	dryRun := flag.Bool("dry-run", false, "Only print what would be updated")
	dumpedGainsFile := flag.String("dumped-gains-file", "",
		"Path to a dump_music file from which gains will be read (instead of being computed)")
	forceGlob := flag.String("force-glob", "",
		"Glob pattern relative to music dir for files to scan and update even if they haven't changed")
	importJSONFile := flag.String("import-json-file", "",
		"If non-empty, path to JSON file containing a stream of Song objects to import")
	importUserData := flag.Bool("import-user-data", true,
		"When importing from JSON, replace user data (ratings, tags, plays, etc.)")
	limit := flag.Int("limit", 0, "If positive, limits the number of songs to update (for testing)")
	requireCovers := flag.Bool("require-covers", false,
		"Die if cover images aren't found for any songs that have album IDs")
	songPathsFile := flag.String("song-paths-file", "",
		"Path to a file containing one relative path per line for songs to force updating")
	testGainInfo := flag.String("test-gain-info", "",
		"Hardcoded gain info as \"track:album:amp\" (for testing)")
	flag.Parse()

	var cfg config
	if err := types.LoadClientConfig(*configFile, &cfg); err != nil {
		log.Fatal("Unable to read config file: ", err)
	}

	if *deleteSongID > 0 {
		if *dryRun {
			log.Fatal("-dry-run is incompatible with -delete-song-id")
		}
		log.Printf("Deleting song %v", *deleteSongID)
		deleteSong(&cfg, *deleteSongID)
		return
	}

	var err error
	var numSongs int
	var scannedDirs []string
	var replaceUserData, didFullScan bool
	readChan := make(chan types.SongOrErr)
	startTime := time.Now()

	if len(*testGainInfo) > 0 {
		var info mp3gain.Info
		if _, err := fmt.Sscanf(*testGainInfo, "%f:%f:%f",
			&info.TrackGain, &info.AlbumGain, &info.PeakAmp); err != nil {
			log.Fatalf("Bad -test-gain-info (want \"track:album:amp\"): %v", err)
		}
		mp3gain.SetInfoForTest(&info)
	}

	if len(*importJSONFile) > 0 {
		log.Printf("Reading songs from %v", *importJSONFile)
		if numSongs, err = readSongsFromJSONFile(*importJSONFile, readChan); err != nil {
			log.Fatal("Failed reading songs: ", err)
		}
		replaceUserData = *importUserData
	} else {
		if len(cfg.MusicDir) == 0 {
			log.Fatal("musicDir not set in config")
		}

		// Not all these options will necessarily be used (e.g. readSongList doesn't need forceGlob
		// or logProgress), but it doesn't hurt to pass them.
		opts := scanOptions{
			computeGain:    cfg.ComputeGain,
			forceGlob:      *forceGlob,
			logProgress:    true,
			artistRewrites: cfg.ArtistRewrites,
		}

		if len(*dumpedGainsFile) > 0 {
			opts.dumpedGains, err = readDumpedGains(*dumpedGainsFile)
			if err != nil {
				log.Fatal("Failed reading dumped gains: ", err)
			}
		}

		if len(*songPathsFile) > 0 {
			numSongs, err = readSongList(*songPathsFile, cfg.MusicDir, readChan, &opts)
			if err != nil {
				log.Fatal("Failed reading song list: ", err)
			}
		} else {
			if len(cfg.LastUpdateInfoFile) == 0 {
				log.Fatal("lastUpdateInfoFile not set in config")
			}
			info, err := readLastUpdateInfo(cfg.LastUpdateInfoFile)
			if err != nil {
				log.Fatal("Unable to get last update info: ", err)
			}
			log.Printf("Scanning for songs in %v updated since %v", cfg.MusicDir, info.Time.Local())
			numSongs, scannedDirs, err = scanForUpdatedSongs(cfg.MusicDir, info.Time, info.Dirs, readChan, &opts)
			if err != nil {
				log.Fatal("Scanning failed: ", err)
			}
			didFullScan = true
		}
	}

	if *limit > 0 && numSongs > *limit {
		numSongs = *limit
	}

	log.Printf("Sending %v song(s)", numSongs)

	// Look up covers and feed songs to the updater.
	updateChan := make(chan types.Song)
	go func() {
		for i := 0; i < numSongs; i++ {
			s := <-readChan
			if s.Err != nil {
				log.Fatalf("Got error for %v: %v\n", s.Filename, s.Err)
			}
			s.CoverFilename = getCoverFilename(cfg.CoverDir, s.Song)
			if *requireCovers && len(s.CoverFilename) == 0 &&
				(len(s.AlbumID) > 0 || len(s.CoverID) > 0 || len(s.RecordingID) > 0) {
				log.Fatalf("Failed to find cover for %v (album=%v, cover=%v, recording=%v)",
					s.Filename, s.AlbumID, s.CoverID, s.RecordingID)
			}
			s.RecordingID = ""

			log.Print("Sending ", s.Filename)
			updateChan <- *s.Song
		}
		close(updateChan)
	}()

	if *dryRun {
		enc := json.NewEncoder(os.Stdout)
		for s := range updateChan {
			if err := enc.Encode(s); err != nil {
				log.Fatal("Failed encoding song: ", err)
			}
		}
	} else {
		if err := updateSongs(&cfg, updateChan, replaceUserData); err != nil {
			log.Fatal("Failed updating songs: ", err)
		}
		if didFullScan {
			if err := writeLastUpdateInfo(cfg.LastUpdateInfoFile, lastUpdateInfo{
				Time: startTime,
				Dirs: scannedDirs,
			}); err != nil {
				log.Fatal("Failed saving update info: ", err)
			}
		}
	}
}

// lastUpdateInfo contains information about the last full update that was performed.
// It is used to identify new music files.
type lastUpdateInfo struct {
	// Time contains the time at which the last update was started.
	Time time.Time `json:"time"`
	// Dirs contains all song-containing directories that were seen (relative to config.MusicDir).
	Dirs []string `json:"dirs"`
}

// readLastUpdateInfo JSON-unmarshals a lastUpdateInfo struct from the file at p.
func readLastUpdateInfo(p string) (info lastUpdateInfo, err error) {
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return info, nil
		}
		return info, err
	}
	defer f.Close()

	err = json.NewDecoder(f).Decode(&info)
	return info, err
}

// writeLastUpdateInfo JSON-marshals info to a file at p.
func writeLastUpdateInfo(p string, info lastUpdateInfo) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(info); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// getCoverFilename returns the relative path under dir for song's cover image.
func getCoverFilename(dir string, song *types.Song) string {
	for _, s := range []string{song.CoverID, song.AlbumID, song.RecordingID} {
		if len(s) > 0 {
			fn := s + ".jpg"
			if _, err := os.Stat(filepath.Join(dir, fn)); err == nil {
				return fn
			}
		}
	}
	return ""
}

// readDumpedGains reads gains from a dump_music file at p.
// The returned map is keyed by song filename.
func readDumpedGains(p string) (map[string]mp3gain.Info, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gains := make(map[string]mp3gain.Info)
	d := json.NewDecoder(f)
	for {
		var s types.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if s.TrackGain == 0 && s.AlbumGain == 0 && s.PeakAmp == 0 {
			return nil, fmt.Errorf("missing gain info for %q", s.Filename)
		}
		gains[s.Filename] = mp3gain.Info{
			TrackGain: s.TrackGain,
			AlbumGain: s.AlbumGain,
			PeakAmp:   s.PeakAmp,
		}
	}
	return gains, nil
}
