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
}

func main() {
	configFile := flag.String("config", "", "Path to config file")
	//dryRun := flag.Bool("dry-run", false, "Only print what would be done")
	importDb := flag.String("import-db", "", "If non-empty, path to legacy SQLite database to read info from")
	limit := flag.Int("limit", 0, "If positive, limits the number of songs to update (for testing)")
	musicDir := flag.String("music-dir", "", "Directory where music is stored")
	server := flag.String("server", "", "URL of the server to update")
	flag.Parse()

	var cfg config
	if err := cloud.ReadJson(*configFile, &cfg); err != nil {
		log.Fatal("Unable to read config file: ", err)
	}

	if len(*server) == 0 {
		log.Fatal("--server must be specified")
	}
	u, err := newUpdater(*server, cfg.ClientId, cfg.ClientSecret, cfg.TokenCache)
	if err != nil {
		log.Fatal(err)
	}

	numSongs := 0
	updateChan := make(chan SongAndError, updateBatchSize)
	startTime := time.Now()
	replaceUserData := false

	if len(*importDb) > 0 {
		if len(*musicDir) > 0 {
			log.Fatal("--import-db and --music-dir are incompatible with each other")
		}
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
		log.Printf("Scanning for songs in %v updated since %v", *musicDir, lastUpdateTime.Local())
		if numSongs, err = scanForUpdatedSongs(*musicDir, lastUpdateTime, updateChan); err != nil {
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

		batchedSongs = append(batchedSongs, *songAndError.Song)
		if len(batchedSongs) == updateBatchSize || i == numSongs-1 {
			if err := u.UpdateSongs(&batchedSongs, replaceUserData); err != nil {
				log.Fatal("Failed updating songs: ", err)
			}
			batchedSongs = batchedSongs[:0]
		}
	}

	if len(*musicDir) > 0 {
		if err := u.SetLastUpdateTime(startTime); err != nil {
			log.Fatal("Failed setting last-update time: ", err)
		}
	}
}
