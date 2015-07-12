package main

import (
	"flag"
	"log"

	"erat.org/nup"
	"erat.org/nup/lib"
)

const (
	logInterval         = 100
	artistNameThreshold = 0.5
	variousArtists      = "Various Artists"
)

type config struct {
	OldCoverDir  string
	NewCoverDir  string
	MinDimension int
}

type imageSize struct {
	Width, Height int
}

type albumInfo struct {
	AlbumId     string
	AlbumName   string
	ArtistCount map[string]int
	OldFilename string
	OldSize     imageSize
}

func main() {
	dumpFile := flag.String("dump-file", "", "Path to file containing dumped JSON songs")
	oldCoverDir := flag.String("old-cover-dir", "", "Path to directory containing existing cover images")
	newCoverDir := flag.String("new-cover-dir", "", "Path to directory where cover images should be written")
	minDimension := flag.Int("min-dimension", 400, "Minimum image width or height")
	maxSongs := flag.Int("max-songs", -1, "Maximum number of songs to inspect")
	maxRequests := flag.Int("max-requests", 2, "Maximum number of parallel HTTP requests")
	flag.Parse()

	if len(*dumpFile) == 0 {
		log.Fatal("-dump-file must be set")
	}
	if len(*newCoverDir) == 0 {
		log.Fatal("-new-cover-dir must be set")
	}

	cfg := config{
		OldCoverDir:  *oldCoverDir,
		NewCoverDir:  *newCoverDir,
		MinDimension: *minDimension,
	}

	var err error
	cf := lib.NewCoverFinder()
	if len(*oldCoverDir) > 0 {
		log.Printf("Loading old covers from %v", *oldCoverDir)
		if err = cf.AddDir(*oldCoverDir); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("Loading new covers from %v", *newCoverDir)
	if err = cf.AddDir(*newCoverDir); err != nil {
		log.Fatal(err)
	}

	log.Printf("Reading songs from %v", *dumpFile)
	ch := make(chan nup.SongOrErr)
	numSongs, err := lib.GetSongsFromJsonFile(*dumpFile, ch)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Read %v song(s)", numSongs)

	if *maxSongs >= 0 && *maxSongs < numSongs {
		numSongs = *maxSongs
	}

	albums, err := scanSongsForNeededCovers(&cfg, cf, ch, numSongs)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Need to fetch %v cover(s)", len(albums))

	downloadCovers(albums, *newCoverDir, *maxRequests, true)
}
