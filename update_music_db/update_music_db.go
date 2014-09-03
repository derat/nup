package main

import (
	"flag"
	"log"
	"time"

	"erat.org/cloud"
	"erat.org/nup"
)

const (
	updateBatchSize = 100
)

type config struct {
	ClientId     string
	ClientSecret string
	TokenCache   string
}

func main() {
	configFile := flag.String("config", "", "Path to config file")
	//dryRun := flag.Bool("dry-run", false, "Only print what would be done")
	fullScan := flag.Bool("full-scan", false, "Hash all files, even if we think they haven't changed")
	importDb := flag.String("import-db", "", "If non-empty, path to legacy SQLite database to read info from")
	musicDir := flag.String("music-dir", "", "Directory where music is stored")
	//preserveFilenames := flag.Bool("preserve-filenames", false, "Treat filenames as not having changed; update hashes instead")
	server := flag.String("server", "", "URL of the server to update")
	subdir := flag.String("subdir", "", "Operate only on a relative subdirectory of --music-dir")
	flag.Parse()

	var cfg config
	if err := cloud.ReadJson(*configFile, &cfg); err != nil {
		log.Fatal("Unable to read config file: ", err)
	}

	if len(*server) == 0 {
		log.Fatal("--server must be specified")
	}

	if len(*importDb) > 0 {
		if len(*musicDir) > 0 {
			log.Fatal("--import-db and --music-dir are incompatible with each other")
		}

		u, err := newUpdater(*server, cfg.ClientId, cfg.ClientSecret, cfg.TokenCache)
		if err != nil {
			log.Fatal(err)
		}

		songs, err := importFromLegacyDb(*importDb)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Got %v songs from legacy database\n", len(*songs))

		songsBatch := make([]nup.Song, 0, updateBatchSize)
		for i := 0; i < len(*songs); i++ {
			songsBatch = append(songsBatch, *(*songs)[i])
			if len(songsBatch) == updateBatchSize {
				if err = u.UpdateSongs(&songsBatch, true); err != nil {
					log.Fatal(err)
				}
				songsBatch = songsBatch[:0]
			}
		}
		if len(songsBatch) > 0 {
			if err = u.UpdateSongs(&songsBatch, true); err != nil {
				log.Fatal(err)
			}
		}
		return
	}

	if *fullScan && len(*subdir) > 0 {
		log.Fatal("--full-scan and --subdir are incompatible with each other")
	}

	startTime := time.Now()

	u, err := newUpdater(*server, cfg.ClientId, cfg.ClientSecret, cfg.TokenCache)
	if err != nil {
		log.Fatal(err)
	}

	lastUpdateTime, err := u.GetLastUpdateTime()
	if err != nil {
		log.Fatalf("Unable to get last update time: ", err)
	}

	updateChan := make(chan SongAndError, updateBatchSize)
	numUpdates := scanForUpdatedFiles(*musicDir, lastUpdateTime, updateChan)
	batchedUpdates := make([]nup.Song, 0, updateBatchSize)
	for i := 0; i < numUpdates; i++ {
		songAndError := <-updateChan
		if songAndError.Error != nil {
			log.Fatalf("Got error while reading %v: %v\n", songAndError.Song.Filename, songAndError.Error)
		}

		batchedUpdates = append(batchedUpdates, *songAndError.Song)
		if len(batchedUpdates) == updateBatchSize {
			if err := u.UpdateSongs(&batchedUpdates, false); err != nil {
				log.Fatal("Failed updating songs: ", err)
			}
			batchedUpdates = batchedUpdates[:0]
		}
	}

	if len(batchedUpdates) > 0 {
		if err := u.UpdateSongs(&batchedUpdates, false); err != nil {
			log.Fatal("Failed updating songs: ", err)
		}
	}
	if len(*subdir) == 0 {
		if err := u.SetLastUpdateTime(startTime); err != nil {
			log.Fatal("Failed setting last-update time: ", err)
		}
	}
}
