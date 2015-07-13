package main

import (
	"flag"
	"fmt"
	"log"

	"erat.org/nup"
	"erat.org/nup/lib"
)

type config struct {
	OldCoverDir  string
	NewCoverDir  string
	MinDimension int
	MaxRequests  int
}

type imageSize struct {
	Width, Height int
}

type albumInfo struct {
	AlbumId          string
	AlbumName        string
	ArtistCount      map[string]int
	OldPath, NewPath string
	OldSize, NewSize imageSize
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

	cfg := &config{
		OldCoverDir:  *oldCoverDir,
		NewCoverDir:  *newCoverDir,
		MinDimension: *minDimension,
		MaxRequests:  *maxRequests,
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
	albums, err := scanSongs(cfg, ch, numSongs)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Got %v album(s)", len(albums))

	downloadCovers(cfg, albums, true)
	fmt.Println("The following old files can be moved away:")
	fmt.Println("------------------------------------------")
	for _, info := range albums {
		if len(info.OldPath) > 0 && len(info.NewPath) > 0 {
			fmt.Printf("%q\n", info.OldPath)
		}
	}
}
