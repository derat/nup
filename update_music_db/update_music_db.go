package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"

	"erat.org/nup"
)

const (
	updateBatchSize = 100
)

func main() {
	//dryRun := flag.Bool("dry-run", false, "Only print what would be done")
	fullScan := flag.Bool("full-scan", false, "Hash all files, even if we think they haven't changed")
	importDb := flag.String("import-db", "", "If non-empty, path to legacy SQLite database to read info from")
	musicDir := flag.String("music-dir", filepath.Join(os.Getenv("HOME"), "music"), "Directory where music is stored")
	//preserveFilenames := flag.Bool("preserve-filenames", false, "Treat filenames as not having changed; update hashes instead")
	subdir := flag.String("subdir", "", "Operate only on a relative subdirectory of --music-dir")
	flag.Parse()

	if len(*importDb) > 0 {
		if len(*musicDir) > 0 {
			log.Fatal("--import-db and --music-dir are incompatible with each other")
		}
	}

	if *fullScan && len(*subdir) > 0 {
		log.Fatal("--full-scan and --subdir are incompatible with each other")
	}

	startTime := time.Now()

	u, err := newDatabaseUpdater()
	if err != nil {
		log.Fatal(err)
	}

	lastUpdateTime, err := u.GetLastUpdateTime()
	if err != nil {
		log.Fatalf("Unable to get last update time: ", err)
	}

	updateChan := make(chan *nup.SongData, updateBatchSize)
	numUpdates := scanForUpdatedFiles(*musicDir, lastUpdateTime, updateChan)
	batchedUpdates := make([]*nup.SongData, 0, updateBatchSize)
	for i := 0; i < numUpdates; i++ {
		song := <-updateChan
		if song.Error != nil {
			log.Fatalf("Got error while reading %v: %v\n", song.Filename, song.Error)
		}

		batchedUpdates = append(batchedUpdates, song)
		if len(batchedUpdates) == updateBatchSize {
			if err := u.UpdateSongs(batchedUpdates); err != nil {
				log.Fatal("Failed updating songs: ", err)
			}
			batchedUpdates = batchedUpdates[:0]
		}
	}

	if len(batchedUpdates) > 0 {
		if err := u.UpdateSongs(batchedUpdates); err != nil {
			log.Fatal("Failed updating songs: ", err)
		}
	}
	if len(*subdir) == 0 {
		if err := u.SetLastUpdateTime(startTime); err != nil {
			log.Fatal("Failed setting last-update time: ", err)
		}
	}
}
