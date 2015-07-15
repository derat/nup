package main

import (
	"io/ioutil"
	"path/filepath"
	"strings"

	"erat.org/nup"
)

type CoverFinder struct {
	// Artist-Album -> filename
	artistAlbumMap map[string]string
	// Album -> Artist
	albumMap map[string][]string
}

func NewCoverFinder() *CoverFinder {
	return &CoverFinder{
		artistAlbumMap: make(map[string]string),
		albumMap:       make(map[string][]string),
	}
}

func (cf *CoverFinder) AddDir(dir string) error {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, fi := range entries {
		ext := filepath.Ext(fi.Name())
		if ext != ".jpg" && ext != ".png" && ext != ".gif" {
			continue
		}

		base := fi.Name()[0 : len(fi.Name())-len(ext)]
		parts := strings.Split(base, "-")
		if len(parts) < 2 {
			continue
		}

		// Artist or album names may contain hyphens, so we'll just consider everything
		// following a hyphen to be a potential album.
		cf.artistAlbumMap[base] = fi.Name()
		for i := 1; i < len(parts); i++ {
			artist := strings.Join(parts[:i], "-")
			album := strings.Join(parts[i:], "-")
			if len(album) > 0 {
				cf.albumMap[album] = append(cf.albumMap[album], artist)
			}
		}
	}
	return nil
}

func (cf *CoverFinder) FindFilename(artist, album string) string {
	artist = nup.EscapeCoverString(artist)
	album = nup.EscapeCoverString(album)

	if fn, ok := cf.artistAlbumMap[artist+"-"+album]; ok {
		return fn
	}

	// Return the filename corresponding to an artist in cf.albumMap.
	artistFunc := func(ca string) string { return cf.artistAlbumMap[ca+"-"+album] }

	// If we just have a single matching album, run with it.
	coverArtists := cf.albumMap[album]
	if len(coverArtists) == 1 {
		return artistFunc(coverArtists[0])
	}

	// Match e.g. "[artist] feat. [someone else]".
	for _, coverArtist := range coverArtists {
		if strings.HasPrefix(artist, coverArtist) {
			return artistFunc(coverArtist)
		}
	}

	// Look for an album with a generic artist name.
	var variousNames = [...]string{"Various Artists", "Various"}
	for _, coverArtist := range coverArtists {
		for _, various := range variousNames {
			if coverArtist == various {
				return artistFunc(coverArtist)
			}
		}
	}

	return ""
}
