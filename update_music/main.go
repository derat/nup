package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"erat.org/nup"
)

type Config struct {
	nup.ClientConfig
	CoverDir           string
	MusicDir           string
	LastUpdateTimeFile string
}

func getLastUpdateTime(path string) (t time.Time, err error) {
	f, err := os.Open(path)
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

func setLastUpdateTime(path string, t time.Time) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(t)
}

func getCoverFilename(dir string, song *nup.Song) string {
	if len(song.AlbumId) != 0 {
		fn := song.AlbumId + ".jpg"
		if _, err := os.Stat(filepath.Join(dir, fn)); err == nil {
			return fn
		}
	}
	if len(song.RecordingId) != 0 {
		fn := song.RecordingId + ".jpg"
		if _, err := os.Stat(filepath.Join(dir, fn)); err == nil {
			return fn
		}
	}
	return ""
}

func main() {
	configFile := flag.String("config", "", "Path to config file")
	deleteSongId := flag.Int64("delete-song-id", 0, "Delete song with given ID")
	dryRun := flag.Bool("dry-run", false, "Only print what would be updated")
	forceGlob := flag.String("force-glob", "", "Glob pattern relative to music dir for files to scan and update even if they haven't changed")
	importJsonFile := flag.String("import-json-file", "", "If non-empty, path to JSON file containing a stream of Song objects to import")
	importUserData := flag.Bool("import-user-data", true, "When importing from JSON, replace user data (ratings, tags, plays, etc.)")
	limit := flag.Int("limit", 0, "If positive, limits the number of songs to update (for testing)")
	requireCovers := flag.Bool("require-covers", false, "Die if cover images aren't found for any songs that have album IDs")
	songPathsFile := flag.String("song-paths-file", "", "Path to a file containing one relative path per line for songs to force updating")
	flag.Parse()

	var cfg Config
	if err := nup.ReadJSON(*configFile, &cfg); err != nil {
		log.Fatal("Unable to read config file: ", err)
	}

	if *deleteSongId > 0 {
		log.Printf("Deleting song %v", *deleteSongId)
		deleteSong(cfg, *deleteSongId)
		return
	}

	var err error
	numSongs := 0
	readChan := make(chan nup.SongOrErr)
	startTime := time.Now()
	replaceUserData := false
	didFullScan := false

	if len(*importJsonFile) > 0 {
		log.Printf("Reading songs from %v", *importJsonFile)
		if numSongs, err = getSongsFromJsonFile(*importJsonFile, readChan); err != nil {
			log.Fatal(err)
		}
		replaceUserData = *importUserData
	} else {
		if len(cfg.MusicDir) == 0 {
			log.Fatal("MusicDir not set in config")
		}

		if len(*songPathsFile) > 0 {
			f, err := os.Open(*songPathsFile)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				go func(relPath string) { getSongByPath(cfg.MusicDir, relPath, readChan) }(scanner.Text())
				numSongs++
			}
		} else {
			lastUpdateTime, err := getLastUpdateTime(cfg.LastUpdateTimeFile)
			if err != nil {
				log.Fatal("Unable to get last update time: ", err)
			}
			log.Printf("Scanning for songs in %v updated since %v", cfg.MusicDir, lastUpdateTime.Local())
			if numSongs, err = scanForUpdatedSongs(cfg.MusicDir, *forceGlob, lastUpdateTime, readChan, true); err != nil {
				log.Fatal(err)
			}
			didFullScan = true
		}
	}

	if *limit > 0 {
		numSongs = int(math.Min(float64(numSongs), float64(*limit)))
	}

	log.Printf("Sending %v song(s)\n", numSongs)

	// Look up covers and feed songs to the updater.
	updateChan := make(chan nup.Song)
	go func() {
		for i := 0; i < numSongs; i++ {
			s := <-readChan
			if s.Err != nil {
				log.Fatalf("Got error for %v: %v\n", s.Filename, s.Err)
			}
			s.CoverFilename = getCoverFilename(cfg.CoverDir, s.Song)
			if *requireCovers && len(s.CoverFilename) == 0 && (len(s.AlbumId) > 0 || len(s.RecordingId) > 0) {
				log.Fatalf("Failed to find cover for %v (album=%v, recording=%v)", s.Filename, s.AlbumId, s.RecordingId)
			}
			s.RecordingId = ""

			log.Print("Sending ", s.Filename)
			updateChan <- *s.Song
		}
	}()

	if *dryRun {
		e := json.NewEncoder(os.Stdout)
		for i := 0; i < numSongs; i++ {
			e.Encode(<-updateChan)
		}
	} else {
		if err = updateSongs(cfg, updateChan, numSongs, replaceUserData); err != nil {
			log.Fatal(err)
		}
		if didFullScan {
			if err = setLastUpdateTime(cfg.LastUpdateTimeFile, startTime); err != nil {
				log.Fatal("Failed setting last-update time: ", err)
			}
		}
	}
}
