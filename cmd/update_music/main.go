// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/derat/nup/server/types"
)

type config struct {
	types.ClientConfig

	// CoverDir is the base directory containing cover art.
	CoverDir string `json:"coverDir"`
	// MusicDir is the base directory containing song files.
	MusicDir string `json:"musicDir"`
	// LastUpdateTimeFile is the path to a JSON file storing the time an update was last performed.
	// The file will be created if it does not already exist.
	LastUpdateTimeFile string `json:"lastUpdateTimeFile"`
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
	forceGlob := flag.String("force-glob", "", "Glob pattern relative to music dir for files to scan and update even if they haven't changed")
	importJSONFile := flag.String("import-json-file", "", "If non-empty, path to JSON file containing a stream of Song objects to import")
	importUserData := flag.Bool("import-user-data", true, "When importing from JSON, replace user data (ratings, tags, plays, etc.)")
	limit := flag.Int("limit", 0, "If positive, limits the number of songs to update (for testing)")
	requireCovers := flag.Bool("require-covers", false, "Die if cover images aren't found for any songs that have album IDs")
	songPathsFile := flag.String("song-paths-file", "", "Path to a file containing one relative path per line for songs to force updating")
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
	numSongs := 0
	readChan := make(chan types.SongOrErr)
	startTime := time.Now()
	replaceUserData := false
	didFullScan := false

	if len(*importJSONFile) > 0 {
		log.Printf("Reading songs from %v", *importJSONFile)
		if numSongs, err = readSongsFromJSONFile(*importJSONFile, readChan); err != nil {
			log.Fatal(err)
		}
		replaceUserData = *importUserData
	} else {
		if len(cfg.MusicDir) == 0 {
			log.Fatal("MusicDir not set in config")
		}

		if len(*songPathsFile) > 0 {
			numSongs, err = readSongList(*songPathsFile, cfg.MusicDir, readChan, cfg.ComputeGain, cfg.ArtistRewrites)
			if err != nil {
				log.Fatal("Failed reading song list: ", err)
			}
		} else {
			lastUpdateTime, err := getLastUpdateTime(cfg.LastUpdateTimeFile)
			if err != nil {
				log.Fatal("Unable to get last update time: ", err)
			}
			log.Printf("Scanning for songs in %v updated since %v", cfg.MusicDir, lastUpdateTime.Local())
			numSongs, err = scanForUpdatedSongs(cfg.MusicDir, lastUpdateTime, readChan, &scanOptions{
				computeGain:    cfg.ComputeGain,
				forceGlob:      *forceGlob,
				logProgress:    true,
				artistRewrites: cfg.ArtistRewrites,
			})

			if err != nil {
				log.Fatal("Scanning failed: ", err)
			}
			didFullScan = true
		}
	}

	if *limit > numSongs {
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
		e := json.NewEncoder(os.Stdout)
		for s := range updateChan {
			if err := e.Encode(s); err != nil {
				log.Fatal("Failed encoding song: ", err)
			}
		}
	} else {
		if err = updateSongs(&cfg, updateChan, replaceUserData); err != nil {
			log.Fatal("Failed updating songs: ", err)
		}
		if didFullScan {
			if err = setLastUpdateTime(cfg.LastUpdateTimeFile, startTime); err != nil {
				log.Fatal("Failed setting last-update time: ", err)
			}
		}
	}
}

// getLastUpdateTime JSON-unmarshals a time.Time value from p.
func getLastUpdateTime(p string) (t time.Time, err error) {
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return t, nil
		}
		return t, err
	}
	defer f.Close()

	if err = json.NewDecoder(f).Decode(&t); err != nil {
		return t, err
	}
	return t, nil
}

// setLastUpdateTime JSON-marshals t to p.
func setLastUpdateTime(p string, t time.Time) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(t)
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
