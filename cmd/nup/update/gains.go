// Copyright 2022 Daniel Erat.
// All rights reserved.

package update

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/mp3gain"
	"github.com/derat/nup/server/db"
)

// gainsCache computes and stores gain adjustments for MP3 files.
//
// Gain adjustments need to be computed across entire albums, so adjustments are cached
// so they won't need to be computed multiple times.
type gainsCache struct {
	dumped map[string]mp3gain.Info // read from dumped db.Songs
	cache  *client.TaskCache       // computes and stores gain adjustments
}

// newGainsCache returns a new gainsCache.
//
// If dumpPath is non-empty, db.Song objects are JSON-unmarshaled from it to initialize the cache
// with previously-computed gain adjustments. In this case, musicDir should also contain the base
// music directory (needed since db.Song only contains relative paths).
func newGainsCache(dumpPath, musicDir string) (*gainsCache, error) {
	// mp3gain doesn't seem to take advantage of multiple cores, so run multiple copies in parallel:
	//  https://hydrogenaud.io/index.php?topic=72197.0
	//  https://sound.stackexchange.com/questions/33069/multi-core-batch-volume-gain
	gc := gainsCache{cache: client.NewTaskCache(runtime.NumCPU())}

	if dumpPath != "" {
		gc.dumped = make(map[string]mp3gain.Info)

		f, err := os.Open(dumpPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		d := json.NewDecoder(f)
		for {
			var s db.Song
			if err := d.Decode(&s); err == io.EOF {
				break
			} else if err != nil {
				return nil, err
			}
			if s.TrackGain == 0 && s.AlbumGain == 0 && s.PeakAmp == 0 {
				return nil, fmt.Errorf("missing gain info for %q", s.Filename)
			}
			gc.dumped[filepath.Join(musicDir, s.Filename)] = mp3gain.Info{
				TrackGain: s.TrackGain,
				AlbumGain: s.AlbumGain,
				PeakAmp:   s.PeakAmp,
			}
		}
	}

	return &gc, nil
}

// get returns gain adjustments for the file at p, computing them if needed.
//
// album and albumID correspond to p and are used to process additional
// songs from the same album in the directory.
func (gc *gainsCache) get(p, album, albumID string) (mp3gain.Info, error) {
	// If we already loaded this file's adjustments from a dump, use them.
	if info, ok := gc.dumped[p]; ok {
		return info, nil
	}

	// If the requested song was part of an album, we also need to process all of the other
	// songs in the album in order to compute gain adjustments relative to the entire album.
	// The task key here is arbitrary but needs to be the same for all files in the album.
	dir := filepath.Dir(p)
	hasAlbum := (albumID != "" || album != "") && album != nonAlbumTracksValue
	var key string
	if hasAlbum {
		key = fmt.Sprintf("%q %q %q", dir, album, albumID)
	} else {
		key = fmt.Sprintf("%q", p)
	}

	// Request the adjustments from the TaskCache. The supplied task will only be run
	// if the adjustments aren't already available and there isn't already another task
	// with the same key.
	info, err := gc.cache.Get(p, key, func() (map[string]interface{}, error) {
		var paths []string
		if hasAlbum {
			dirPaths, err := filepath.Glob(filepath.Join(dir, "*.[mM][pP]3")) // case-insensitive
			if err != nil {
				return nil, err
			}
			for _, p := range dirPaths {
				fi, err := os.Stat(p)
				if err != nil {
					return nil, err
				} else if !fi.Mode().IsRegular() {
					continue
				}
				// TODO: Consider caching tags somewhere since we're also reading them in the
				// original readSong call. In practice, computing gains is so incredibly slow (at
				// least on my computer) that reading tags twice probably doesn't matter in the big
				// scheme of things.
				// I'm ignoring errors here since it's weird if we fail to add a new song because
				// some other song in the same directory is broken.
				s, err := readSong(p, "", fi, true /* onlyTags */, nil, nil)
				if err == nil && s.Album == album && s.AlbumID == albumID {
					paths = append(paths, p)
				}
			}
		} else {
			paths = []string{p}
		}
		if len(paths) == 1 {
			log.Printf("Computing gain adjustments for %v", paths[0])
		} else {
			log.Printf("Computing gain adjustments for %d songs in %v", len(paths), dir)
		}

		infos, err := mp3gain.ComputeAlbum(paths)
		if err != nil {
			return nil, err
		}
		res := make(map[string]interface{}, len(infos))
		for p, info := range infos {
			res[p] = info
		}
		return res, nil
	})

	if err != nil {
		return mp3gain.Info{}, err
	}
	return info.(mp3gain.Info), nil
}
