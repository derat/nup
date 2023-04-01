// Copyright 2023 Daniel Erat.
// All rights reserved.

package files

import (
	"path/filepath"
	"testing"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/test"
	"github.com/google/go-cmp/cmp"
)

func TestReadSong_Override(t *testing.T) {
	dir := t.TempDir()
	cfg := &client.Config{
		MusicDir:    filepath.Join(dir, "music"),
		MetadataDir: filepath.Join(dir, "metadata"),
	}

	want := test.Song0s
	want.TrackGain = 0
	want.AlbumGain = 0
	want.PeakAmp = 0
	want.Artist = "Overridden Artist"
	test.Must(t, test.CopySongs(cfg.MusicDir, want.Filename))
	p := filepath.Join(cfg.MusicDir, want.Filename)

	// Test that metadata can be overridden.
	// The overriding logic is tested in more detail in override_test.go.
	if err := UpdateMetadataOverride(cfg, &want); err != nil {
		t.Fatal(err)
	}

	if got, err := ReadSong(cfg, p, nil /* fi */, 0, nil /* gc */); err != nil {
		t.Fatalf("ReadSong(cfg, %q, nil, 0, nil) failed: %v", p, err)
	} else if diff := cmp.Diff(want, *got); diff != "" {
		t.Errorf("ReadSong(cfg, %q, nil, 0, nil) returned bad data:\n%s", p, diff)
	}

	// Also check that the SHA1 and duration are omitted when SkipAudioData is passed.
	want.SHA1 = ""
	want.Length = 0
	if got, err := ReadSong(cfg, p, nil /* fi */, SkipAudioData, nil /* gc */); err != nil {
		t.Fatalf("ReadSong(cfg, %q, nil, SkipAudioData, nil) failed: %v", p, err)
	} else if diff := cmp.Diff(want, *got); diff != "" {
		t.Errorf("ReadSong(cfg, %q, nil, SkipAudioData, nil) returned bad data:\n%s", p, diff)
	}

	// Check that OnlyFileMetadata works too.
	want.Artist = test.Song0s.Artist
	if got, err := ReadSong(cfg, p, nil /* fi */, SkipAudioData|OnlyFileMetadata, nil /* gc */); err != nil {
		t.Fatalf("ReadSong(cfg, %q, nil, SkipAudioData|OnlyFileMetadata, nil) failed: %v", p, err)
	} else if diff := cmp.Diff(want, *got); diff != "" {
		t.Errorf("ReadSong(cfg, %q, nil, SkipAudioData|OnlyFileMetadata, nil) returned bad data:\n%s", p, diff)
	}
}

func TestReadSong_ID3V1(t *testing.T) {
	dir := t.TempDir()
	want := test.ID3V1Song
	want.TrackGain = 0
	want.AlbumGain = 0
	want.PeakAmp = 0
	test.Must(t, test.CopySongs(dir, want.Filename))
	p := filepath.Join(dir, want.Filename)

	cfg := client.Config{MusicDir: dir}
	if got, err := ReadSong(&cfg, p, nil /* fi */, 0, nil /* gc */); err != nil {
		t.Fatalf("ReadSong(cfg, %q, ...) failed: %v", p, err)
	} else if diff := cmp.Diff(want, *got); diff != "" {
		t.Errorf("ReadSong(cfg, %q, ...) returned bad data:\n%s", p, diff)
	}
}

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
