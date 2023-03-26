// Copyright 2023 Daniel Erat.
// All rights reserved.

package files

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
	"github.com/google/go-cmp/cmp"
)

func TestApplyMetadataOverride(t *testing.T) {
	orig := db.Song{
		Filename:     "some-song.mp3",
		Artist:       "Old Artist",
		Title:        "Old Title",
		Album:        "Old Album",
		AlbumArtist:  "Old AlbumArtist",
		DiscSubtitle: "Old DiscSubtitle",
		AlbumID:      "Old AlbumID",
		RecordingID:  "Old RecordingID",
		Track:        1,
		Disc:         2,
		Date:         time.Date(2023, 4, 26, 1, 2, 0, 0, time.UTC),
	}
	updated := db.Song{
		Filename:        "some-song.mp3",
		Artist:          "New Artist",
		Title:           "New Title",
		Album:           "New Album",
		AlbumArtist:     "New AlbumArtist",
		DiscSubtitle:    "New DiscSubtitle",
		AlbumID:         "New AlbumID",
		RecordingID:     "New RecordingID",
		OrigAlbumID:     orig.AlbumID,
		OrigRecordingID: orig.RecordingID,
		Track:           3,
		Disc:            4,
		Date:            time.Date(2021, 1, 2, 3, 4, 0, 0, time.UTC),
	}

	cfg := &client.Config{MetadataDir: t.TempDir()}
	mp := MetadataOverridePath(cfg, &orig)

	// Update all of the fields via the override file.
	if b, err := json.Marshal(MetadataOverride{
		Artist:       &updated.Artist,
		Title:        &updated.Title,
		Album:        &updated.Album,
		AlbumArtist:  &updated.AlbumArtist,
		DiscSubtitle: &updated.DiscSubtitle,
		AlbumID:      &updated.AlbumID,
		RecordingID:  &updated.RecordingID,
		Track:        &updated.Track,
		Disc:         &updated.Disc,
		Date:         &updated.Date,
	}); err != nil {
		t.Fatal(err)
	} else if err := ioutil.WriteFile(mp, b, 0644); err != nil {
		t.Fatal(err)
	}
	got := orig
	if err := applyMetadataOverride(cfg, &got); err != nil {
		t.Fatal("applyMetadataOverride with full override failed:", err)
	} else if diff := cmp.Diff(updated, got); diff != "" {
		t.Error("applyMetadataOverride with full override updated song incorrectly:\n" + diff)
	}

	// Nothing should happen if the override file contains an empty object.
	if b, err := json.Marshal(MetadataOverride{}); err != nil {
		t.Fatal(err)
	} else if err := ioutil.WriteFile(mp, b, 0644); err != nil {
		t.Fatal(err)
	}
	got = orig
	if err := applyMetadataOverride(cfg, &got); err != nil {
		t.Fatal("applyMetadataOverride with empty override failed:", err)
	} else if diff := cmp.Diff(orig, got); diff != "" {
		t.Error("applyMetadataOverride with empty override modified song:\n" + diff)
	}

	// Nothing should happen if the override file is missing, too.
	os.Remove(mp)
	got = orig
	if err := applyMetadataOverride(cfg, &got); err != nil {
		t.Fatal("applyMetadataOverride with missing override failed:", err)
	} else if diff := cmp.Diff(orig, got); diff != "" {
		t.Error("applyMetadataOverride with missing override modified song:\n" + diff)
	}
}
