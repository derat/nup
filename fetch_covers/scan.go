package main

import (
	"fmt"
	"log"
	"path/filepath"

	"erat.org/nup"
	"erat.org/nup/lib"
)

const (
	logInterval = 100
)

func scanSongs(cfg *config, ch chan nup.SongOrErr, numSongs int) ([]*albumInfo, error) {
	var ocf *lib.CoverFinder
	if len(cfg.OldCoverDir) > 0 {
		log.Printf("Loading old covers from %v", cfg.OldCoverDir)
		ocf = lib.NewCoverFinder()
		if err := ocf.AddDir(cfg.OldCoverDir); err != nil {
			return nil, err
		}
	}

	log.Printf("Loading new covers from %v", cfg.NewCoverDir)
	ncf := lib.NewCoverFinder()
	if err := ncf.AddDir(cfg.NewCoverDir); err != nil {
		return nil, err
	}

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

		info, ok := albums[s.AlbumId]
		if !ok {
			info = &albumInfo{
				AlbumId:     s.AlbumId,
				AlbumName:   s.Album,
				ArtistCount: make(map[string]int),
			}

			if ocf != nil {
				if fn := ocf.FindFilename(s.Artist, s.Album); len(fn) > 0 {
					path := filepath.Join(cfg.OldCoverDir, fn)
					size, err := getImageSize(path)
					if err != nil {
						return nil, fmt.Errorf("Unable to get dimensions of %v: %v", path, err)
					}
					info.OldPath = path
					info.OldSize = size
				}
			}
			if fn := ncf.FindFilename(s.Artist, s.Album); len(fn) > 0 {
				path := filepath.Join(cfg.NewCoverDir, fn)
				size, err := getImageSize(path)
				if err != nil {
					return nil, fmt.Errorf("Unable to get dimensions of %v: %v", path, err)
				}
				info.NewPath = path
				info.NewSize = size
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
		ret = append(ret, info)
	}
	return ret, nil
}
