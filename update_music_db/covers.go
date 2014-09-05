package main

import (
	"io/ioutil"
	"path/filepath"
	"strings"
)

type coverFinder struct {
	artistAlbumMap map[string]string
	albumMap       map[string]string
	artistMap      map[string]string
}

func newCoverFinder(coverDir string) (*coverFinder, error) {
	entries, err := ioutil.ReadDir(coverDir)
	if err != nil {
		return nil, err
	}

	cf := &coverFinder{
		artistAlbumMap: make(map[string]string),
		albumMap:       make(map[string]string),
		artistMap:      make(map[string]string),
	}
	for _, fi := range entries {
		fn := fi.Name()

		ext := filepath.Ext(fn)
		if ext != ".jpg" && ext != ".png" && ext != ".gif" {
			continue
		}

		base := fn[0 : len(fn)-len(ext)]
		parts := strings.Split(base, "-")
		if len(parts) < 2 {
			continue
		}

		// Artist or album names may contain hyphens, so we'll just consider everything
		// following a hyphen to be a potential album and everything preceding a hyphen
		// to be a potential artist.
		cf.artistAlbumMap[base] = fn
		for i := 1; i < len(parts); i++ {
			cf.albumMap[strings.Join(parts[i:], "-")] = fn
			cf.artistMap[strings.Join(parts[0:i], "-")] = fn
		}
	}
	return cf, nil
}

func (cf *coverFinder) findPath(artist, album string) string {
	escape := func(s string) string {
		s = strings.Replace(s, "/", "%", -1)
		return s
	}
	artist = escape(artist)
	album = escape(album)

	if fn, ok := cf.artistAlbumMap[artist+"-"+album]; ok {
		return fn
	}
	if fn, ok := cf.albumMap[album]; ok {
		return fn
	}
	return cf.artistMap[album]
}
