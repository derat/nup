package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

func downloadCovers(albums []albumInfo, dir string) {
	ch := make(chan (downloadResult))
	for i := range albums {
		go func(info *albumInfo) {
			path, err := downloadCover(info, dir)
			ch <- downloadResult{info, path, err}
		}(&(albums[i]))
	}

	for i := 0; i < len(albums); i++ {
		res := <-ch
		if res.Err != nil {
			log.Printf("Failed to get %v (%v): %v", res.AlbumInfo.AlbumId, res.AlbumInfo.AlbumName, res.Err)
		} else if len(res.Path) == 0 {
			log.Printf("Didn't find %v (%v)", res.AlbumInfo.AlbumId, res.AlbumInfo.AlbumName)
		} else {
			log.Printf("Wrote %v to %v", res.AlbumInfo.AlbumId, res.Path)
		}
	}
}
