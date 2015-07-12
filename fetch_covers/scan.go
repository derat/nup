package main

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"erat.org/nup"
	"erat.org/nup/lib"
)

const (
	logInterval = 100
)

func getImageSize(path string) (imageSize, error) {
	f, err := os.Open(path)
	if err != nil {
		return imageSize{}, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return imageSize{}, err
	}

	return imageSize{img.Bounds().Max.X - img.Bounds().Min.X, img.Bounds().Max.Y - img.Bounds().Min.Y}, nil
}

func scanSongsForNeededCovers(cfg *config, cf *lib.CoverFinder, ch chan nup.SongOrErr, numSongs int) ([]*albumInfo, error) {
	albums := make(map[string]*albumInfo)

	for i := 0; i < numSongs; i++ {
		s := <-ch
		if s.Err != nil {
			return nil, fmt.Errorf("Got error for %v: %v", s.Filename, s.Err)
		}

		if i > 0 && i%logInterval == 0 {
			log.Printf("Scanned %v songs", i)
		}

		// Can't do anything if the song doesn't have an album ID.
		if len(s.AlbumId) == 0 {
			continue
		}

		ok := false
		var info *albumInfo
		if info, ok = albums[s.AlbumId]; !ok {
			path := ""
			size := imageSize{}

			if fn := cf.FindFilename(s.Artist, s.Album); len(fn) > 0 {
				if _, err := os.Stat(filepath.Join(cfg.NewCoverDir, fn)); err == nil {
					path = filepath.Join(cfg.NewCoverDir, fn)
				} else {
					path = filepath.Join(cfg.OldCoverDir, fn)
				}

				var err error
				size, err = getImageSize(path)
				if err != nil {
					return nil, fmt.Errorf("Unable to get dimensions of %v: %v", path, err)
				}
			}

			info = &albumInfo{
				AlbumId:     s.AlbumId,
				AlbumName:   s.Album,
				ArtistCount: make(map[string]int),
				OldPath:     path,
				OldSize:     size,
			}
			albums[s.AlbumId] = info
		}

		if info.AlbumName != s.Album {
			return nil, fmt.Errorf("Album name mismatch for %v: had %q but saw %q for %v", s.AlbumId, info.AlbumName, s.Album, s.Filename)
		}
		info.ArtistCount[s.Artist]++
	}
	if numSongs%logInterval != 0 {
		log.Printf("Scanned %v songs", numSongs)
	}

	ret := make([]*albumInfo, 0, len(albums))
	for _, info := range albums {
		alreadyDownloaded := strings.HasPrefix(info.OldPath, cfg.NewCoverDir)
		if len(info.OldPath) == 0 || ((info.OldSize.Width < cfg.MinDimension || info.OldSize.Height < cfg.MinDimension) && !alreadyDownloaded) {
			ret = append(ret, info)
		}
	}
	return ret, nil
}
