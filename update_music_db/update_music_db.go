package main

import (
	"flag"
	"log"
	"math"
	"time"

	"erat.org/cloud"
	"erat.org/nup"
)

const (
	updateBatchSize = 100
)

type SongAndError struct {
	Song  *nup.Song
	Error error
}

type config struct {
	ClientId     string
	ClientSecret string
	TokenCache   string

	ServerUrl     string
	MusicDir      string
	CoverDir      string
	RequireCovers bool
}

func main() {
	configFile := flag.String("config", "", "Path to config file")
	dryRun := flag.Bool("dry-run", false, "Only print what would be updated")
	importDb := flag.String("import-db", "", "If non-empty, path to legacy SQLite database to read info from")
	limit := flag.Int("limit", 0, "If positive, limits the number of songs to update (for testing)")
	flag.Parse()

	var cfg config
	if err := cloud.ReadJson(*configFile, &cfg); err != nil {
		log.Fatal("Unable to read config file: ", err)
	}
	u, err := newUpdater(cfg)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Loading covers from %v", cfg.CoverDir)
	cf, err := newCoverFinder(cfg.CoverDir)
	if err != nil {
		log.Fatal(err)
	}

	numSongs := 0
	updateChan := make(chan SongAndError, updateBatchSize)
	startTime := time.Now()
	replaceUserData := false

	if len(*importDb) > 0 {
		log.Printf("Reading songs from %v", *importDb)
		if numSongs, err = getSongsFromLegacyDb(*importDb, updateChan); err != nil {
			log.Fatal(err)
		}
		replaceUserData = true
	} else {
		lastUpdateTime, err := u.GetLastUpdateTime()
		if err != nil {
			log.Fatalf("Unable to get last update time: ", err)
		}
		log.Printf("Scanning for songs in %v updated since %v", cfg.MusicDir, lastUpdateTime.Local())
		if numSongs, err = scanForUpdatedSongs(cfg, lastUpdateTime, updateChan); err != nil {
			log.Fatal(err)
		}
	}

	if *limit > 0 {
		numSongs = int(math.Min(float64(numSongs), float64(*limit)))
	}

	log.Printf("Sending %v song(s)\n", numSongs)
	batchedSongs := make([]nup.Song, 0, updateBatchSize)
	for i := 0; i < numSongs; i++ {
		songAndError := <-updateChan
		if songAndError.Error != nil {
			log.Fatalf("Got error for %v: %v\n", songAndError.Song.Filename, songAndError.Error)
		}

		s := *songAndError.Song
		s.CoverFilename = cf.findPath(s.Artist, s.Album)
		if cfg.RequireCovers && len(s.CoverFilename) == 0 {
			log.Fatalf("Failed to find cover for %v (%v-%v)", s.Filename, s.Artist, s.Album)
		}

		if *dryRun {
			log.Print(s)
			continue
		}

		batchedSongs = append(batchedSongs, s)
		if len(batchedSongs) == updateBatchSize || i == numSongs-1 {
			if err := u.UpdateSongs(&batchedSongs, replaceUserData); err != nil {
				log.Fatal("Failed updating songs: ", err)
			}
			batchedSongs = batchedSongs[:0]
		}
	}

	if !*dryRun && len(*importDb) == 0 {
		if err := u.SetLastUpdateTime(startTime); err != nil {
			log.Fatal("Failed setting last-update time: ", err)
		}
	}
}
