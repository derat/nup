// Copyright 2022 Daniel Erat.
// All rights reserved.

package query

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/derat/nup/server/db"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"foo", "foo"},
		{"Foo", "foo"},
		{"N*E*R*D", "n*e*r*d"},
		{"Björk", "bjork"},
		{"múm", "mum"},
		{"Björk feat. múm", "bjork feat. mum"},
		{"Queensrÿche", "queensryche"},
		{"mañana", "manana"},
		{"Antônio Carlos Jobim", "antonio carlos jobim"},
		{"Mattias Häggström Gerdt", "mattias haggstrom gerdt"},
		{"Tomáš Dvořák", "tomas dvorak"},
		{"µ-Ziq", "μ-ziq"}, // MICRO SIGN mapped to GREEK SMALL LETTER MU
		{"μ-Ziq", "μ-ziq"}, // GREEK SMALL LETTER MU unchanged
		{"2winz²", "2winz2"},
		{"®", "®"}, // surprised that this doesn't become "r"!
		{"™", "tm"},
		{"✝", "✝"},
		{"…", "..."},
		{"Сергей Васильевич Рахманинов", "сергеи васильевич рахманинов"},
		{"永田権太", "永田権太"},
	} {
		if got, err := Normalize(tc.in); err != nil {
			t.Errorf("Normalize(%q) failed: %v", tc.in, err)
		} else if got != tc.want {
			t.Errorf("Normalize(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

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
	// twice in a row for a given artist. This skips checking the album of the final song. *shrug*
	lastArtistAlbum := make(map[string]string, numArtists)
	for i, s := range songs[:len(songs)-1] {
		next := songs[i+1]
		if s.ArtistLower == next.ArtistLower {
			t.Errorf("Artist %q appears at %d and %d", s.ArtistLower, i, i+1)
		}
		if s.AlbumLower == lastArtistAlbum[s.ArtistLower] {
			t.Errorf("Album %q repeated for artist %q at %d", s.AlbumLower, s.ArtistLower, i)
		}
		lastArtistAlbum[s.ArtistLower] = s.AlbumLower
	}
}
