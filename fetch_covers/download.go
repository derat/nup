package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"erat.org/nup/lib"
)

const (
	artistNameThreshold = 0.5
	variousArtists      = "Various"
)

// chooseFilename returns the "best" filename for an album.
func chooseFilename(info *albumInfo) string {
	// If there was already an existing file, use its basename so that songs
	// don't need to be updated.
	if len(info.OldPath) > 0 {
		return filepath.Base(info.OldPath)
	}

	// Find the most-commonly-occurring artist name.
	bestArtist := ""
	bestCount := 0
	totalCount := 0
	for name, count := range info.ArtistCount {
		if count > bestCount {
			bestArtist = name
			bestCount = count
		}
		totalCount += count
	}
	if float32(bestCount)/float32(totalCount) < artistNameThreshold {
		bestArtist = variousArtists
	}
	return lib.EscapeCoverString(fmt.Sprintf("%s-%s.jpg", bestArtist, info.AlbumName))
}

// downloadCover downloads cover art for info into dir.
// It returns the image file's path if successful, an empty path if the cover
// was not found, or an error for a probably-transient error.
func downloadCover(info *albumInfo, dir string) error {
	url := fmt.Sprintf("https://coverartarchive.org/release/%s/front-500", info.AlbumId)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("Fetching %v failed: %v", url, err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		if resp.StatusCode == 404 {
			return nil
		}
		return fmt.Errorf("Got %v when fetching %v", resp.StatusCode, url)
	}
	defer resp.Body.Close()

	path := filepath.Join(dir, chooseFilename(info))
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	if _, err = io.Copy(f, resp.Body); err != nil {
		f.Close()
		return fmt.Errorf("Failed to read from %v: %v", url, err)
	}
	f.Close()

	info.NewSize, err = getImageSize(path)
	if err != nil {
		return err
	}
	info.NewPath = path
	return nil
}

func getAlbumsToDownload(albums []*albumInfo, minDimension int) []*albumInfo {
	albumsToDownload := make([]*albumInfo, 0)
	for _, info := range albums {
		// If we've already downloaded it, downloading it again probably won't help.
		if len(info.NewPath) != 0 {
			continue
		}

		if len(info.OldPath) == 0 || info.OldSize.Width < minDimension || info.OldSize.Height < minDimension {
			albumsToDownload = append(albumsToDownload, info)
		}
	}
	return albumsToDownload
}

type downloadResult struct {
	AlbumInfo *albumInfo
	Err       error
}

func downloadCovers(cfg *config, albums []*albumInfo, retryFailures bool) {
	albumsToDownload := getAlbumsToDownload(albums, cfg.MinDimension)

	numReq := 0
	canStartReq := func() bool { return numReq < cfg.MaxRequests }
	cond := sync.NewCond(&sync.Mutex{})
	ch := make(chan downloadResult)
	go func() {
		for i := range albumsToDownload {
			cond.L.Lock()
			for !canStartReq() {
				cond.Wait()
			}
			numReq++
			cond.L.Unlock()

			go func(info *albumInfo) {
				ch <- downloadResult{info, downloadCover(info, cfg.NewCoverDir)}
				cond.L.Lock()
				numReq--
				cond.Signal()
				cond.L.Unlock()
			}(albumsToDownload[i])
		}
	}()

	retryableAlbums := make([]*albumInfo, 0)
	for i := 0; i < len(albumsToDownload); i++ {
		res := <-ch
		if res.Err != nil {
			log.Printf("Failed to get %v (%v): %v", res.AlbumInfo.AlbumId, res.AlbumInfo.AlbumName, res.Err)
			retryableAlbums = append(retryableAlbums, res.AlbumInfo)
		} else if len(res.AlbumInfo.NewPath) == 0 {
			log.Printf("Didn't find %v (%v)", res.AlbumInfo.AlbumId, res.AlbumInfo.AlbumName)
			// Cache the negative result somewhere? Maybe not, since it seems
			// like the archive sometimes returns transient 404s for covers it
			// actualy has...
		} else {
			log.Printf("Wrote %v to %v", res.AlbumInfo.AlbumId, res.AlbumInfo.NewPath)
		}
	}

	if len(retryableAlbums) > 0 && retryFailures {
		log.Printf("Retrying %v album(s)", len(retryableAlbums))
		downloadCovers(cfg, retryableAlbums, false)
	}
}
