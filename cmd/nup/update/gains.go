// Copyright 2022 Daniel Erat.
// All rights reserved.

package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/derat/nup/cmd/nup/mp3gain"
	"github.com/derat/nup/server/db"
	"github.com/derat/taglib-go/taglib"
)

// gainsCache computes and stores gain adjustments for MP3 files.
//
// Gain adjustments need to be computed across entire albums, so adjustments are cached
// so they won't need to be computed multiple times.
type gainsCache struct {
	infos map[string]mp3gain.Info // keyed by absolute path
	mu    sync.Mutex              // guards infos
}

// newGainsCache returns a new gainsCache.
//
// If dumpPath is non-empty, db.Song objects are JSON-unmarshaled from it to initialize the cache
// with previously-computed gain adjustments. musicDir should contain the base music directory
// (since db.Song only contains relative paths).
func newGainsCache(dumpPath, musicDir string) (*gainsCache, error) {
	gc := &gainsCache{infos: make(map[string]mp3gain.Info)}

	if dumpPath != "" {
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
			gc.infos[filepath.Join(musicDir, s.Filename)] = mp3gain.Info{
				TrackGain: s.TrackGain,
				AlbumGain: s.AlbumGain,
				PeakAmp:   s.PeakAmp,
			}
		}
	}

	return gc, nil
}

// get returns gain adjustments for the file at songPath, computing them if needed.
//
// songAlbum and songAlbumID correspond to songPath and are used to process additional
// songs from the same album in the directory.
func (gc *gainsCache) get(songPath, songAlbum, songAlbumID string) (mp3gain.Info, error) {
	// TODO: I think that mp3gain isn't multithreaded, so this should probably support
	// running multiple processes in parallel for different albums. Unfortunately, doing
	// this seems likely to make the code super-complicated.
	gc.mu.Lock()
	defer gc.mu.Unlock()

	// If we already computed info for this song, we're done.
	if info, ok := gc.infos[songPath]; ok {
		return info, nil
	}

	albumPaths := []string{songPath}

	// If the requested song was part of an album, we also need to process all of the other
	// songs in the album in order to compute gain adjustments relative to the entire album.
	if (songAlbumID != "" || songAlbum != "") && songAlbum != nonAlbumTracksValue {
		glob := filepath.Join(filepath.Dir(songPath), "*.[mM][pP]3") // case-insensitive
		dirPaths, err := filepath.Glob(glob)
		if err != nil {
			return mp3gain.Info{}, err
		}
		for _, p := range dirPaths {
			if p == songPath {
				continue
			}

			f, err := os.Open(p)
			if err != nil {
				return mp3gain.Info{}, err
			}
			defer f.Close()

			fi, err := f.Stat()
			if err != nil {
				return mp3gain.Info{}, err
			}
			if !fi.Mode().IsRegular() {
				continue
			}

			// TODO: Consider caching tags since we're also reading them in readSong.
			// In practice, computing gains is so incredibly slow (at least on my computer)
			// that reading tags twice probably doesn't matter in the big scheme of things.
			var album, albumID string
			if tag, err := taglib.Decode(f, fi.Size()); err == nil {
				album = tag.Album()
				albumID = tag.CustomFrames()[albumIDTag]
			} else {
				_, _, _, album, _ = readID3v1Footer(f, fi)
			}

			if album == songAlbum && albumID == songAlbumID {
				albumPaths = append(albumPaths, p)
			}
		}
	}

	if len(albumPaths) == 1 {
		log.Printf("Computing gain adjustments for %v", songPath)
	} else {
		log.Printf("Computing gain adjustments for %d songs in %v",
			len(albumPaths), filepath.Dir(songPath))
	}
	infos, err := mp3gain.ComputeAlbum(albumPaths)
	if err != nil {
		return mp3gain.Info{}, err
	}
	for p, gi := range infos {
		gc.infos[p] = gi
	}
	if info, ok := gc.infos[songPath]; ok {
		return info, nil
	}
	return mp3gain.Info{}, errors.New("missing gain info")
}
