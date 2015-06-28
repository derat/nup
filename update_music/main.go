package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"log"
	"math"
	"os"
	"time"

	"erat.org/cloud"
	"erat.org/nup"
)

type SongOrErr struct {
	*nup.Song
	Err error
}

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

func main() {
	configFile := flag.String("config", "", "Path to config file")
	deleteSongId := flag.Int64("delete-song-id", 0, "Delete song with given ID")
	dryRun := flag.Bool("dry-run", false, "Only print what would be updated")
	forceGlob := flag.String("force-glob", "", "Glob pattern relative to music dir for files to scan and update even if they haven't changed")
	importDb := flag.String("import-db", "", "If non-empty, path to legacy SQLite database to read info from")
	importJsonFile := flag.String("import-json-file", "", "If non-empty, path to JSON file containing a stream of Song objects to import")
	importMinId := flag.Int64("import-min-id", 0, "Starting ID for --import-db (for resuming after failure)")
	importUserData := flag.Bool("import-user-data", true, "When importing from DB or JSON, replace user data (ratings, tags, plays, etc.)")
	limit := flag.Int("limit", 0, "If positive, limits the number of songs to update (for testing)")
	requireCovers := flag.Bool("require-covers", false, "Die if cover images aren't found for any songs")
	songPathsFile := flag.String("song-paths-file", "", "Path to a file containing one relative path per line for songs to force updating")
	flag.Parse()

	var cfg Config
	if err := cloud.ReadJson(*configFile, &cfg); err != nil {
		log.Fatal("Unable to read config file: ", err)
	}

	if *deleteSongId > 0 {
		log.Printf("Deleting song %v", *deleteSongId)
		deleteSong(cfg, *deleteSongId)
		return
	}

	log.Printf("Loading covers from %v", cfg.CoverDir)
	cf, err := newCoverFinder(cfg.CoverDir)
	if err != nil {
		log.Fatal(err)
	}

	numSongs := 0
	readChan := make(chan SongOrErr)
	startTime := time.Now()
	replaceUserData := false
	didFullScan := false

	if len(*importDb) > 0 {
		log.Printf("Reading songs from %v", *importDb)
		if numSongs, err = getSongsFromLegacyDb(*importDb, *importMinId, readChan); err != nil {
			log.Fatal(err)
		}
		replaceUserData = *importUserData
	} else if len(*importJsonFile) > 0 {
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
				log.Fatalf("Unable to get last update time: ", err)
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
			s.CoverFilename = cf.findPath(s.Artist, s.Album)
			if *requireCovers && len(s.CoverFilename) == 0 {
				log.Fatalf("Failed to find cover for %v (%v-%v)", s.Filename, s.Artist, s.Album)
			}

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
