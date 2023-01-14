// Copyright 2022 Daniel Erat.
// All rights reserved.

package query

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/derat/nup/server/db"
)

func TestIntersectSortedIDs(t *testing.T) {
	for _, tc := range []struct{ a, b, want []int64 }{
		{nil, nil, []int64{}},
		{[]int64{1, 2}, nil, []int64{}},
		{nil, []int64{1, 2}, []int64{}},
		{[]int64{1, 2}, []int64{1, 2}, []int64{1, 2}},
		{[]int64{0, 1, 2, 3}, []int64{1, 2}, []int64{1, 2}},
		{[]int64{0, 1, 2, 3, 4}, []int64{-1, 1, 3, 5}, []int64{1, 3}},
	} {
		if got := intersectSortedIDs(tc.a, tc.b); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("intersectSortedIDs(%v, %v) = %v; want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSubtractSortedIDs(t *testing.T) {
	for _, tc := range []struct{ a, b, want []int64 }{
		{nil, nil, []int64{}},
		{[]int64{1, 2}, nil, []int64{1, 2}},
		{nil, []int64{1, 2}, []int64{}},
		{[]int64{1, 2}, []int64{1, 2}, []int64{}},
		{[]int64{0, 1, 2, 3}, []int64{1, 2}, []int64{0, 3}},
		{[]int64{0, 1, 2, 3, 4}, []int64{-1, 1, 3, 5}, []int64{0, 2, 4}},
	} {
		if got := subtractSortedIDs(tc.a, tc.b); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("subtractSortedIDs(%v, %v) = %v; want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSortSongs(t *testing.T) {
	makeSong := func(artist, album string, setAlbumID bool,
		date string, disc, track int) *db.Song {
		var albumID string
		if setAlbumID {
			sum := sha1.Sum([]byte(artist + album))
			albumID = hex.EncodeToString(sum[:])
		}
		var tm time.Time
		if date != "" {
			var err error
			if tm, err = time.Parse("2006", date); err != nil {
				t.Fatalf("Failed parsing %q: %v", date, err)
			}
		}
		return &db.Song{
			AlbumID:     albumID,
			AlbumArtist: artist,
			AlbumLower:  album,
			Date:        tm,
			Disc:        disc,
			Track:       track,
		}
	}

	// Generate a bunch of songs in the desired order.
	want := []*db.Song{
		// Songs with album IDs should be sorted by artist name, then album release date,
		// then album name, and finally disc and track number.
		makeSong("Alphabets", "Our First Album", true, "2001", 1, 1),
		makeSong("Alphabets", "Our First Album", true, "2001", 1, 2),
		makeSong("Alphabets", "Number 2", true, "2002", 1, 1),
		makeSong("Alphabets", "Drei", true, "2005", 1, 1),
		makeSong("Alphabets", "Drei", true, "2005", 2, 1),
		makeSong("Alphabets", "Same Year?!", true, "2005", 1, 1),
		makeSong("Balcony", "Album", true, "1998", 1, 1),
		makeSong("Cakewalk", "Hello", true, "2008", 1, 1),
		// Songs without album IDs should appear at the end, sorted by album name.
		makeSong("Aardvark", "Animals", false, "", 1, 1),
		makeSong("Dracula", "Mansion", false, "", 1, 1),
		makeSong("Dracula", "Mansion", false, "", 1, 2),
		makeSong("Dracula", "Mansion", false, "", 2, 1),
		makeSong("Bakery", "Zebra", false, "", 1, 1),
	}

	// Shuffle the songs before sorting them.
	got := append([]*db.Song(nil), want...)
	shuffleSongsForTest(got)
	sortSongs(got)

	if !reflect.DeepEqual(got, want) {
		stringify := func(songs []*db.Song) string {
			strs := make([]string, len(songs))
			for i, s := range songs {
				var date string
				if !s.Date.IsZero() {
					date = s.Date.Format("2006")
				}
				strs[i] = fmt.Sprintf("  [%q %q %q %d %d]",
					s.AlbumArtist, s.AlbumLower, date, s.Disc, s.Track)
			}
			return strings.Join(strs, "\n")
		}
		t.Errorf("sortSongs produced:\n%s\nwant:\n%s", stringify(got), stringify(want))
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
	shuffleSongsForTest(songs)
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

func shuffleSongsForTest(songs []*db.Song) {
	rand.Seed(0xbeefface)
	for i := 0; i < len(songs)-1; i++ {
		j := i + rand.Intn(len(songs)-i)
		songs[i], songs[j] = songs[j], songs[i]
	}
}
