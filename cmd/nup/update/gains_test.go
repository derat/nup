// Copyright 2022 Daniel Erat.
// All rights reserved.

package update

import (
	"path/filepath"
	"testing"

	"github.com/derat/nup/cmd/nup/mp3gain"
	"github.com/derat/nup/test"
)

func TestGainsCache(t *testing.T) {
	dir := t.TempDir()

	// Song0s and 1s are both from the same album, while 5s is from its own.
	s0, s1, s5 := test.Song0s, test.Song1s, test.Song5s
	if err := test.CopySongs(dir, s0.Filename, s1.Filename, s5.Filename); err != nil {
		t.Fatal(err)
	}
	p0 := filepath.Join(dir, s0.Filename)
	p1 := filepath.Join(dir, s1.Filename)
	p5 := filepath.Join(dir, s5.Filename)

	info := mp3gain.Info{
		TrackGain: -7.25,
		AlbumGain: -4.5,
		PeakAmp:   1.125,
	}
	mp3gain.SetInfoForTest(&info)
	defer mp3gain.SetInfoForTest(nil)

	gc, err := newGainsCache("", "")
	if err != nil {
		t.Fatal("newGainsCache failed: ", err)
	}

	// Compute adjustments for a song.
	if got, err := gc.get(p0, s0.Album, s0.AlbumID); err != nil {
		t.Fatalf("gc.get(%q, %q, %q) failed: %v", p0, s0.Album, s0.AlbumID, err)
	} else if got != info {
		t.Errorf("gc.get(%q, %q, %q) = %+v; want %+v", p0, s0.Album, s0.AlbumID, got, info)
	}

	// We should've also saved adjustments for the other song in the album.
	if sz := gc.cache.Size(); sz != 2 {
		t.Errorf("Computed gain adjustments for %v file(s); want 2", sz)
	} else if got, ok := gc.cache.GetIfExists(p1); !ok {
		t.Errorf("Didn't compute gain adjustment for %v", p1)
	} else if got != info {
		t.Errorf("Gain adjustment for %v is %+v; want %+v", p1, got, info)
	}

	// Now compute adjustments for a song in a different album.
	if got, err := gc.get(p5, s5.Album, s5.AlbumID); err != nil {
		t.Fatalf("gc.get(%q, %q, %q) failed: %v", p5, s5.Album, s5.AlbumID, err)
	} else if got != info {
		t.Errorf("gc.get(%q, %q, %q) = %+v; want %+v", p5, s5.Album, s5.AlbumID, got, info)
	}
	if sz := gc.cache.Size(); sz != 3 {
		t.Errorf("Computed gain adjustments for %v file(s); want 3", sz)
	}
}
