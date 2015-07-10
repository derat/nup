package lib

import (
	"path/filepath"
	"testing"

	"erat.org/nup/test"
)

func TestCoversBogusDir(t *testing.T) {
	cf, err := NewCoverFinder("bogus")
	if cf != nil || err == nil {
		t.Errorf("creation with bogus cover dir didn't fail")
	}
}

func TestCoversFindPath(t *testing.T) {
	cf, err := NewCoverFinder(filepath.Join(test.GetDataDir(), "covers"))
	if err != nil {
		t.Fatalf("creation failed")
	}

	for _, tc := range []struct{ artist, album, expected string }{
		{"Artist", "Album", "Artist-Album.jpg"},                             // One album, exact match.
		{"Foo", "Bar", "Foo-Bar.png"},                                       // Multiple albums, exact match.
		{"Foo feat. Foobar", "Bar", "Foo-Bar.png"},                          // Multiple albums, one has artist prefix.
		{"Baz", "Bar", "Baz-Bar.png"},                                       // Multiple albums, exact match.
		{"Baz feat. Someone Else", "Bar", "Baz-Bar.png"},                    // Multiple albums, one has artist prefix.
		{"Other", "Bar", ""},                                                // Multiple albums, no artist prefix.
		{"Pearl Jam", "Ten", "Pearl Jam-Ten.gif"},                           // Space in artist name.
		{"AC/DC", "Back In Black", "AC%DC-Back In Black.jpg"},               // Replace slashes.
		{"Other Artist", "Album", "Artist-Album.jpg"},                       // No exact match, but only one album.
		{"Coats", "Hats", "Coats-Hats.jpg"},                                 // Multiple albums, exact match.
		{"Jackets", "Hats", "Various Artists-Hats.jpg"},                     // Multiple albums, one has generic artist.
		{"Weird", "Band-We-Like-Hyphens", "Weird-Band-We-Like-Hyphens.jpg"}, // Split into all possible artist/album pairs.
		{"Weird-Band", "We-Like-Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Weird-Band-We", "Like-Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Weird-Band-We-Like", "Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Some Guy", "Hyphens", "Weird-Band-We-Like-Hyphens.jpg"},
		{"Albumless", "", "Albumless-.jpg"}, // Unset album.
		{"Not There", "At All", ""},         // No matches.
		{"Pearl Jam", "", ""},               // Artist matches but album doesn't.
		{"", "", ""},                        // Missing artist/album.
	} {
		actual := cf.FindPath(tc.artist, tc.album)
		if actual != tc.expected {
			t.Errorf("%q, %q: expected %q but got %q", tc.artist, tc.album, tc.expected, actual)
		}
	}
}
