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

	"erat.org/nup"
	"erat.org/nup/lib"
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

	// Sizes of existing cover images, keyed by cover basename. If the same file
	// exists in both the old and new dir, the latter takes precedence.
	sizes := make(map[string]imageSize)

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

		needCover := false
		var size imageSize
		fn := cf.FindFilename(s.Artist, s.Album)
		if len(fn) == 0 {
			needCover = true
		} else {
			var ok bool
			if size, ok = sizes[fn]; !ok {
				path := ""
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
				sizes[fn] = size
			}
			// TODO: Should only do this if the image came from the old, rather
			// than new, directory.
			if size.Width < cfg.MinDimension || size.Height < cfg.MinDimension {
				needCover = true
			}
		}

		if needCover {
			if _, ok := albums[s.AlbumId]; !ok {
				albums[s.AlbumId] = &albumInfo{
					AlbumId:     s.AlbumId,
					AlbumName:   s.Album,
					ArtistCount: make(map[string]int),
					OldFilename: fn,
					OldSize:     size,
				}
			}
			oldName := albums[s.AlbumId].AlbumName
			if oldName != s.Album {
				return nil, fmt.Errorf("Album name mismatch for %v: had %q but saw %q for %v", s.AlbumId, oldName, s.Album, s.Filename)
			}
			albums[s.AlbumId].ArtistCount[s.Artist]++
		}
	}
	if numSongs%logInterval != 0 {
		log.Printf("Scanned %v songs", numSongs)
	}

	ret := make([]*albumInfo, 0, len(albums))
	for _, info := range albums {
		ret = append(ret, info)
	}
	return ret, nil
}
