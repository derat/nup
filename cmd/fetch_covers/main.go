package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/derat/nup/types"
	"github.com/derat/taglib-go/taglib"
)

const (
	logInterval = 100
	albumIdTag  = "MusicBrainz Album Id"
)

func getCoverFilename(albumId string) string {
	return albumId + ".jpg"
}

func readSong(path string) (albumId string, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	tag, err := taglib.Decode(f, fi.Size())
	if err != nil {
		return "", err
	}
	return tag.CustomFrames()[albumIdTag], nil
}

func readDumpFile(jsonPath string, coverDir string, maxSongs int) (albumIds []string, err error) {
	missingAlbumIds := make(map[string]bool)

	f, err := os.Open(jsonPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	numSongs := 0
	for true {
		if maxSongs >= 0 && numSongs >= maxSongs {
			break
		}

		s := types.Song{}
		if err = d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		numSongs++

		if numSongs%logInterval == 0 {
			log.Printf("Scanned %v songs", numSongs)
		}

		// Can't do anything if the song doesn't have an album ID.
		if len(s.AlbumId) == 0 {
			continue
		}

		// Check if we already have the cover.
		if _, err := os.Stat(filepath.Join(coverDir, getCoverFilename(s.AlbumId))); err == nil {
			continue
		}

		missingAlbumIds[s.AlbumId] = true
	}
	if numSongs%logInterval != 0 {
		log.Printf("Scanned %v songs", numSongs)
	}

	ret := make([]string, len(missingAlbumIds))
	i := 0
	for id := range missingAlbumIds {
		ret[i] = id
		i++
	}
	return ret, nil
}

// downloadCover downloads cover art for albumId into dir.
// If the cover was not found, path is empty and err is nil.
func downloadCover(albumId, dir string) (path string, err error) {
	url := fmt.Sprintf("https://coverartarchive.org/release/%s/front-500", albumId)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("Fetching %v failed: %v", url, err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		if resp.StatusCode == 404 {
			return "", nil
		}
		return "", fmt.Errorf("Got %v when fetching %v", resp.StatusCode, url)
	}
	defer resp.Body.Close()

	path = filepath.Join(dir, getCoverFilename(albumId))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err = io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("Failed to read from %v: %v", url, err)
	}
	return path, nil
}

func downloadCovers(albumIds []string, dir string, maxRequests int) {
	numReq := 0
	canStartReq := func() bool { return numReq < maxRequests }
	cond := sync.NewCond(&sync.Mutex{})

	wg := sync.WaitGroup{}
	wg.Add(len(albumIds))

	go func() {
		for _, id := range albumIds {
			cond.L.Lock()
			for !canStartReq() {
				cond.Wait()
			}
			numReq++
			cond.L.Unlock()

			go func(id string) {
				path, err := downloadCover(id, dir)

				cond.L.Lock()
				numReq--
				cond.Signal()
				cond.L.Unlock()

				if err != nil {
					log.Printf("Failed to get %v: %v", id, err)
				} else if len(path) == 0 {
					log.Printf("Didn't find %v", id)
				} else {
					log.Printf("Wrote %v", path)
				}
				wg.Done()
			}(id)
		}
	}()

	wg.Wait()
}

func main() {
	dumpFile := flag.String("dump-file", "", "Path to file containing dumped JSON songs")
	coverDir := flag.String("cover-dir", "", "Path to directory where cover images should be written")
	maxSongs := flag.Int("max-songs", -1, "Maximum number of songs to inspect")
	maxRequests := flag.Int("max-requests", 2, "Maximum number of parallel HTTP requests")
	flag.Parse()

	if len(*coverDir) == 0 {
		log.Fatal("-cover-dir must be set")
	}

	albumIds := make([]string, 0)
	if len(*dumpFile) > 0 {
		if len(flag.Args()) > 0 {
			log.Fatal("Cannot both set -dump-file and list music files")
		}
		log.Printf("Reading songs from %v", *dumpFile)
		var err error
		if albumIds, err = readDumpFile(*dumpFile, *coverDir, *maxSongs); err != nil {
			log.Fatal(err)
		}
	} else if len(flag.Args()) > 0 {
		ids := make(map[string]bool)
		for _, p := range flag.Args() {
			if id, err := readSong(p); err != nil {
				log.Fatalf("Failed to read %v: %v", p, err)
			} else if len(id) > 0 {
				log.Printf("%v has album ID %v", p, id)
				ids[id] = true
			}
		}
		for id, _ := range ids {
			albumIds = append(albumIds, id)
		}
	} else {
		log.Fatal("Either set -dump-file or list music files as arguments")
	}

	log.Printf("Downloading cover(s) for %v album(s)", len(albumIds))
	downloadCovers(albumIds, *coverDir, *maxRequests)
}