// Copyright 2020 Daniel Erat.
// All rights reserved.

package covers

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
	"github.com/derat/taglib-go/taglib"
	"github.com/google/subcommands"
)

const (
	logInterval = 100
	albumIDTag  = "MusicBrainz Album Id"
)

type Command struct {
	Cfg *client.Config

	coverDir    string // directory to write covers to
	maxSongs    int    // songs to inspect
	maxRequests int    // parallel HTTP requests
	size        int    // image size to download (250, 500, 1200)
}

func (*Command) Name() string     { return "covers" }
func (*Command) Synopsis() string { return "download album art" }
func (*Command) Usage() string {
	return `covers [flags]:
	Download album art from coverartarchive.org for dumped songs from
	stdin. Image files are written to the directory specified via
	-cover-dir.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.coverDir, "cover-dir", "", "Directory to write covers to")
	f.IntVar(&cmd.maxSongs, "max-songs", -1, "Maximum number of songs to inspect")
	f.IntVar(&cmd.maxRequests, "max-requests", 2, "Maximum number of parallel HTTP requests")
	f.IntVar(&cmd.size, "size", 1200, "Image size to download (250, 500, or 1200)")
}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	if cmd.coverDir == "" {
		fmt.Fprintln(os.Stderr, "-cover-dir must be supplied")
		return subcommands.ExitUsageError
	}

	albumIDs := make([]string, 0)
	if len(args) > 0 {
		ids := make(map[string]bool)
		for _, p := range args {
			if id, err := readSong(p.(string)); err != nil {
				fmt.Fprintf(os.Stderr, "Failed reading %v: %v", p, err)
				return subcommands.ExitFailure
			} else if len(id) > 0 {
				log.Printf("%v has album ID %v", p, id)
				ids[id] = true
			}
		}
		for id, _ := range ids {
			albumIDs = append(albumIDs, id)
		}
	} else {
		log.Print("Reading songs from stdin")
		var err error
		if albumIDs, err = readDumpedSongs(os.Stdin, cmd.coverDir, cmd.maxSongs); err != nil {
			fmt.Fprintln(os.Stderr, "Failed reading dumped songs:", err)
			return subcommands.ExitFailure
		}
	}

	log.Printf("Downloading cover(s) for %v album(s)", len(albumIDs))
	downloadCovers(albumIDs, cmd.coverDir, cmd.size, cmd.maxRequests)
	return subcommands.ExitSuccess
}

func getCoverFilename(albumID string) string {
	return albumID + ".jpg"
}

func readSong(path string) (albumID string, err error) {
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
	return tag.CustomFrames()[albumIDTag], nil
}

func readDumpedSongs(r io.Reader, coverDir string, maxSongs int) (albumIDs []string, err error) {
	missingAlbumIDs := make(map[string]struct{})
	d := json.NewDecoder(r)
	numSongs := 0
	for {
		if maxSongs >= 0 && numSongs >= maxSongs {
			break
		}

		s := db.Song{}
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
		if len(s.AlbumID) == 0 {
			continue
		}

		// Check if we already have the cover.
		if _, err := os.Stat(filepath.Join(coverDir, getCoverFilename(s.AlbumID))); err == nil {
			continue
		}

		missingAlbumIDs[s.AlbumID] = struct{}{}
	}
	if numSongs%logInterval != 0 {
		log.Printf("Scanned %v songs", numSongs)
	}

	ret := make([]string, len(missingAlbumIDs))
	i := 0
	for id := range missingAlbumIDs {
		ret[i] = id
		i++
	}
	return ret, nil
}

// downloadCover downloads cover art for albumID into dir.
// If the cover was not found, path is empty and err is nil.
func downloadCover(albumID, dir string, size int) (path string, err error) {
	url := fmt.Sprintf("https://coverartarchive.org/release/%s/front-%d", albumID, size)
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

	path = filepath.Join(dir, getCoverFilename(albumID))
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

func downloadCovers(albumIDs []string, dir string, size, maxRequests int) {
	cache := client.NewTaskCache(maxRequests)
	wg := sync.WaitGroup{}
	wg.Add(len(albumIDs))

	for _, id := range albumIDs {
		go func(id string) {
			if path, err := cache.Get(id, id, func() (map[string]interface{}, error) {
				if p, err := downloadCover(id, dir, size); err != nil {
					return nil, err
				} else {
					return map[string]interface{}{id: p}, nil
				}
			}); err != nil {
				log.Printf("Failed to get %v: %v", id, err)
			} else if len(path.(string)) == 0 {
				log.Printf("Didn't find %v", id)
			} else {
				log.Printf("Wrote %v", path.(string))
			}
			wg.Done()
		}(id)
	}
	wg.Wait()
}
