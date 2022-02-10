// Copyright 2022 Daniel Erat.
// All rights reserved.

package db

import (
	"reflect"
	"testing"
	"time"
)

func TestSong_Update(t *testing.T) {
	t1 := time.Unix(1, 0)
	t2 := time.Unix(2, 0)
	t3 := time.Unix(3, 0)
	t4 := time.Unix(4, 0)

	src := Song{
		SHA1:           "deadbeef",
		Filename:       "foo/bar.mp3",
		CoverFilename:  "cover.jpg",
		Artist:         "The Artist",
		Title:          "The Title",
		Album:          "The Album",
		AlbumArtist:    "AlbumArtist",
		AlbumID:        "album-id",
		Track:          13,
		Disc:           2,
		Length:         154.3,
		TrackGain:      -5.6,
		AlbumGain:      -7.2,
		PeakAmp:        1.1,
		Rating:         0.5,
		FirstStartTime: t1,
		LastStartTime:  t2,
		NumPlays:       2,
		Tags:           []string{"rock", "guitar", "rock"},
	}

	dst := Song{
		Rating:         0.25,
		FirstStartTime: t3,
		LastStartTime:  t4,
		NumPlays:       4,
		Tags:           []string{"instrumental", "electronic", "instrumental"},
	}

	want := src

	// Set some automatically-generated fields.
	want.ArtistLower = "the artist"
	want.TitleLower = "the title"
	want.AlbumLower = "the album"
	want.Keywords = []string{"album", "albumartist", "artist", "the", "title"}

	// User data should also be preserved.
	want.Rating = dst.Rating
	want.FirstStartTime = dst.FirstStartTime
	want.LastStartTime = dst.LastStartTime
	want.NumPlays = dst.NumPlays
	want.Tags = []string{"electronic", "instrumental"} // sort and dedupe

	if err := dst.Update(&src, false /* copyUserData */); err != nil {
		t.Fatal("Update failed: ", err)
	}
	if !reflect.DeepEqual(dst, want) {
		t.Fatalf("Update didn't give desired results:\nwant: %+v\n got: %+v", want, dst)
	}

	dst = Song{}

	want.Rating = src.Rating
	want.RatingAtLeast50 = true
	want.RatingAtLeast25 = true
	want.RatingAtLeast0 = true
	want.FirstStartTime = src.FirstStartTime
	want.LastStartTime = src.LastStartTime
	want.NumPlays = src.NumPlays
	want.Tags = []string{"guitar", "rock"} // sort and dedupe

	if err := dst.Update(&src, true /* copyUserData */); err != nil {
		t.Fatal("Update failed: ", err)
	}
	if !reflect.DeepEqual(want, dst) {
		t.Fatalf("Update didn't give desired results:\nwant: %+v\n got: %+v", want, dst)
	}
}

func TestSong_Clean(t *testing.T) {
	p1 := NewPlay(time.Unix(1, 0), "a")
	p2 := NewPlay(time.Unix(1, 0), "b")
	p3 := NewPlay(time.Unix(2, 0), "a")

	s := Song{
		Keywords: []string{"a", "d", "b", "a", "d", "a"},
		Tags:     []string{"m", "p", "n", "n", "o", "m"},
		Plays:    []Play{p3, p1, p2, p3, p2},
	}
	s.Clean()

	want := Song{
		Keywords: []string{"a", "b", "d"},
		Tags:     []string{"m", "n", "o", "p"},
		Plays:    []Play{p1, p2, p3},
	}
	if !reflect.DeepEqual(want, s) {
		t.Fatalf("Clean didn't give desired results:\nwant: %+v\n got: %+v", want, s)
	}
}

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
