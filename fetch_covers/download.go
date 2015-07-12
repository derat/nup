package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// chooseArtistName returns the "best" artist name for an album, given a map
// from track artist names to counts.
func chooseArtistName(artistCount map[string]int) string {
	bestName := ""
	bestCount := 0
	totalCount := 0
	for name, count := range artistCount {
		if count > bestCount {
			bestName = name
			bestCount = count
		}
		totalCount += count
	}
	if float32(bestCount)/float32(totalCount) >= artistNameThreshold {
		return bestName
	}
	return variousArtists
}

// downloadCover downloads cover art for info into dir.
// It returns the image file's path if successful, an empty path if the cover
// was not found, or an error for a probably-transient error.
func downloadCover(info *albumInfo, dir string) (path string, err error) {
	url := fmt.Sprintf("https://coverartarchive.org/release/%s/front-500", info.AlbumId)
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

	artist := chooseArtistName(info.ArtistCount)
	path = filepath.Join(dir, fmt.Sprintf("%s-%s.jpg", artist, info.AlbumName))
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

type downloadResult struct {
	AlbumInfo *albumInfo
	Path      string
	Err       error
}

func downloadCovers(albums []*albumInfo, dir string, maxReq int, retryFailures bool) {
	numReq := 0
	canStartReq := func() bool { return numReq < maxReq }
	cond := sync.NewCond(&sync.Mutex{})
	ch := make(chan (downloadResult))
	go func() {
		for i := range albums {
			cond.L.Lock()
			for !canStartReq() {
				cond.Wait()
			}
			numReq++
			cond.L.Unlock()

			go func(info *albumInfo) {
				path, err := downloadCover(info, dir)
				ch <- downloadResult{info, path, err}
				cond.L.Lock()
				numReq--
				cond.Signal()
				cond.L.Unlock()
			}(albums[i])
		}
	}()

	retryableAlbums := make([]*albumInfo, 0)
	for i := 0; i < len(albums); i++ {
		res := <-ch
		if res.Err != nil {
			log.Printf("Failed to get %v (%v): %v", res.AlbumInfo.AlbumId, res.AlbumInfo.AlbumName, res.Err)
			retryableAlbums = append(retryableAlbums, res.AlbumInfo)
		} else if len(res.Path) == 0 {
			log.Printf("Didn't find %v (%v)", res.AlbumInfo.AlbumId, res.AlbumInfo.AlbumName)
			// TODO: Cache the negative result somewhere.
		} else {
			log.Printf("Wrote %v to %v", res.AlbumInfo.AlbumId, res.Path)
		}
	}

	if len(retryableAlbums) > 0 && retryFailures {
		log.Printf("Retrying %v album(s)", len(retryableAlbums))
		downloadCovers(retryableAlbums, dir, 1, false)
	}
}
