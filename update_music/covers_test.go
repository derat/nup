package main

import (
	"testing"
)

func TestCoversBogusDir(t *testing.T) {
	cf, err := newCoverFinder("bogus")
	if cf != nil || err == nil {
		t.Errorf("creation with bogus cover dir didn't fail")
	}
}

func TestCoversFindPath(t *testing.T) {
	cf, err := newCoverFinder("testdata/covers")
	if err != nil {
		t.Fatalf("creation failed")
	}

	for _, tc := range []struct{ artist, album, expected string }{
		{"Artist", "Album", "Artist-Album.jpg"},
		{"Foo", "Bar", "Foo-Bar.png"},
		{"Pearl Jam", "Ten", "Pearl Jam-Ten.gif"},
		{"AC/DC", "Back In Black", "AC%DC-Back In Black.jpg"},
		{"Other Artist", "Album", "Artist-Album.jpg"},
		{"Weird", "Band-We-Like-Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Weird-Band", "We-Like-Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Weird-Band-We", "Like-Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Weird-Band-We-Like", "Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Some Guy", "Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Not There", "At All", ""},
		{"Pearl Jam", "", ""},
		{"", "", ""},
	} {
		actual := cf.findPath(tc.artist, tc.album)
		if actual != tc.expected {
			t.Errorf("%q, %q: expected %q but got %q", tc.artist, tc.album, tc.expected, actual)
		}
	}
}
