// Copyright 2022 Daniel Erat.
// All rights reserved.

package query

import (
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/derat/nup/server/db"
)

func TestSpreadSongs(t *testing.T) {
	const (
		numArtists        = 10
		numSongsPerArtist = 10
		numAlbums         = 5
	)
	songs := make([]*db.Song, numArtists*numSongsPerArtist)
	for i := range songs {
		artist := "ar" + strconv.Itoa((i/numSongsPerArtist)+1)
		album := "al" + strconv.Itoa((i%numAlbums)+1)
		songs[i] = &db.Song{ArtistLower: artist, AlbumLower: album}
	}

	// Do a regular Fisher-Yates shuffle and then spread out the songs.
	rand.Seed(0xbeefface)
	for i := 0; i < len(songs)-1; i++ {
		j := i + rand.Intn(len(songs)-i)
		songs[i], songs[j] = songs[j], songs[i]
	}
	spreadSongs(songs)

	// Check that the same artist doesn't appear back-to-back and that we don't play the same album
	// twice in a row for a given artist.
	lastArtistAlbum := make(map[string]string, numArtists)
	for i, s := range songs {
		if i < len(songs)-1 {
			next := songs[i+1]
			if s.ArtistLower == next.ArtistLower {
				t.Errorf("Artist %q appears at %d and %d", s.ArtistLower, i, i+1)
			}
		}
		if s.AlbumLower == lastArtistAlbum[s.ArtistLower] {
			t.Errorf("Album %q repeated for artist %q at %d", s.AlbumLower, s.ArtistLower, i)
		}
		lastArtistAlbum[s.ArtistLower] = s.AlbumLower
	}
}

func TestSpreadSongs_AlbumArtist(t *testing.T) {
	mk := func(artist, album, albumArtist string) *db.Song {
		return &db.Song{
			Artist:      artist,
			Album:       album,
			ArtistLower: strings.ToLower(artist),
			AlbumLower:  strings.ToLower(album),
			AlbumArtist: albumArtist,
		}
	}
	songs := []*db.Song{
		mk("Foo", "Album 1", ""),
		mk("Foo feat. Bar", "Album 2", "Foo"),
		mk("Foo", "Album 2", "Foo"),
		mk("Foo feat. Baz", "Album 2", "Foo"),
		mk("Foo feat. Someone", "Album 2", "Foo"),
		mk("Foo", "Album 1", ""),
	}
	// Throw in enough songs by "Bar" that the songs by "Foo" and friends
	// probably won't be adjacent if we properly group them together.
	for i := 0; i < 20; i++ {
		songs = append(songs, mk("Bar", "Album 3", ""))
	}
	spreadSongs(songs)

	var last string
	for i, s := range songs {
		name := strings.Fields(s.Artist)[0]
		if i > 0 && name == "Foo" && last == "Foo" {
			t.Errorf("Song %d has artist %q; last was %q", i, s.Artist, songs[i-1].Artist)
		}
		last = name
	}
}
