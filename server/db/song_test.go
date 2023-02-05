// Copyright 2022 Daniel Erat.
// All rights reserved.

package db

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestSong_MarshalJSON(t *testing.T) {
	src := Song{Artist: "The Artist", Date: time.Date(2022, 4, 10, 13, 24, 45, 0, time.UTC)}
	var dst Song
	if b, err := json.Marshal(src); err != nil {
		t.Errorf("Marshaling %v failed: %v", src, err)
	} else if err := json.Unmarshal(b, &dst); err != nil {
		t.Errorf("Unmarshaling %q failed: %v", b, err)
	} else if !reflect.DeepEqual(dst, src) {
		t.Errorf("Round-trip failed: got %v, want %v", dst, src)
	}

	src.Date = time.Time{}
	dst = Song{}
	if b, err := json.Marshal(src); err != nil {
		t.Errorf("Marshaling %v failed: %v", src, err)
	} else if err := json.Unmarshal(b, &dst); err != nil {
		t.Errorf("Unmarshaling %q to song failed: %v", b, err)
	} else if !reflect.DeepEqual(dst, src) {
		t.Errorf("Round-trip failed: got %v, want %v", dst, src)
	} else {
		// Check that the JSON object doesn't include a "date" property.
		var obj map[string]interface{}
		if err := json.Unmarshal(b, &obj); err != nil {
			t.Errorf("Unmarshaling %q to map failed: %v", b, err)
		} else if _, ok := obj["date"]; ok {
			t.Errorf("Zero date is included in %q", b)
		}
	}
}

func TestSong_Update(t *testing.T) {
	t1 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Second)
	t3 := t1.Add(2 * time.Second)
	t4 := t1.Add(3 * time.Second)

	src := Song{
		SHA1:           "deadbeef",
		Filename:       "foo/bar.mp3",
		CoverFilename:  "cover.jpg",
		Artist:         "The Artist",
		Title:          "The Title",
		Album:          "The Album",
		AlbumArtist:    "AlbumArtist",
		DiscSubtitle:   "First Disc",
		AlbumID:        "album-id",
		Track:          13,
		Disc:           2,
		Date:           time.Date(2022, 4, 10, 13, 24, 45, 0, time.UTC),
		Length:         154.3,
		TrackGain:      -5.6,
		AlbumGain:      -7.2,
		PeakAmp:        1.1,
		Rating:         3,
		FirstStartTime: t1,
		LastStartTime:  t2,
		NumPlays:       2,
		Tags:           []string{"rock", "guitar", "rock"},
	}

	dst := Song{
		Keywords:       []string{"old", "keywords"}, // should be dropped
		Rating:         2,
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
	want.Keywords = []string{"album", "albumartist", "artist", "disc", "first", "the", "title"}

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
	want.RatingAtLeast3 = true
	want.RatingAtLeast2 = true
	want.RatingAtLeast1 = true
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
	p1 := NewPlay(time.Date(2022, 6, 5, 10, 15, 0, 0, time.UTC), "a")
	p2 := NewPlay(time.Date(2022, 6, 5, 10, 15, 0, 0, time.UTC), "b")
	p3 := NewPlay(time.Date(2022, 7, 13, 12, 10, 59, 450, time.UTC), "a")

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
