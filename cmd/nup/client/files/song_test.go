// Copyright 2023 Daniel Erat.
// All rights reserved.

package files

import (
	"testing"
)

func TestExtractAlbumDisc(t *testing.T) {
	for _, tc := range []struct {
		orig      string
		album     string
		discNum   int
		discTitle string
	}{
		{"Abbey Road", "Abbey Road", 0, ""},
		{"The Beatles (disc 1)", "The Beatles", 1, ""},
		{"The Beatles (disc 2)", "The Beatles", 2, ""},
		{"The Beatles  (disc 200)", "The Beatles", 200, ""},
		{"The Fragile (disc 1: Left)", "The Fragile", 1, "Left"},
		{"The Fragile (disc 2: Right)", "The Fragile", 2, "Right"},
		{"Indiana Jones: The Soundtracks Collection (disc 1: Raiders of the Lost Ark)",
			"Indiana Jones: The Soundtracks Collection", 1, "Raiders of the Lost Ark"},
		{"Speakerboxxx / The Love Below (disc 2: The Love Below)",
			"Speakerboxxx / The Love Below", 2, "The Love Below"},
	} {
		album, discNum, discTitle := extractAlbumDisc(tc.orig)
		if album != tc.album || discNum != tc.discNum || discTitle != tc.discTitle {
			t.Errorf("extractAlbumDisc(%q) = %q, %d, %q; want %q, %d, %q",
				tc.orig, album, discNum, discTitle, tc.album, tc.discNum, tc.discTitle)
		}
	}
}
