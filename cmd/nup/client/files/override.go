// Copyright 2023 Daniel Erat.
// All rights reserved.

package files

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
)

// MetadataOverride overrides metadata in a song file's tag.
//
// TODO: Add a way to reference another SHA1, filename, or song ID for
// https://github.com/derat/nup/issues/32? There should probably be a way
// to override AlbumGain too in that case.
//
// TODO: Add some way to prevent fields from being updated with bad/messy data in
// MusicBrainz? Maybe just a single Readonly bool field for the whole struct?
type MetadataOverride struct {
	Artist       *string    `json:"artist,omitempty"`
	Title        *string    `json:"title,omitempty"`
	Album        *string    `json:"album,omitempty"`
	AlbumArtist  *string    `json:"albumArtist,omitempty"`
	DiscSubtitle *string    `json:"discSubtitle,omitempty"`
	AlbumID      *string    `json:"albumId,omitempty"`
	RecordingID  *string    `json:"recordingId,omitempty"`
	Track        *int       `json:"track,omitempty"`
	Disc         *int       `json:"disc,omitempty"`
	Date         *time.Time `json:"date,omitempty"`
}

// MetadataOverridePath returns the path under cfg.MetadataDir for a JSON-marshaled
// MetadataOverride struct used to override song's metadata.
func MetadataOverridePath(cfg *client.Config, song *db.Song) string {
	// Not using a cutesy ".jsong" extension here is killing me.
	return filepath.Join(cfg.MetadataDir, song.Filename+".json")
}

// applyMetadataOverride looks for MetadataOverride JSON file in cfg.MetadataDir and
// updates specified fields in s. If no override file exists, nil is returned.
func applyMetadataOverride(cfg *client.Config, song *db.Song) error {
	p := MetadataOverridePath(cfg, song)
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()

	var over MetadataOverride
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&over); err != nil {
		return fmt.Errorf("%v: %v", p, err)
	}

	setString(&song.Artist, over.Artist)
	setString(&song.Title, over.Title)
	setString(&song.Album, over.Album)
	setString(&song.AlbumArtist, over.AlbumArtist)
	setString(&song.DiscSubtitle, over.DiscSubtitle)

	// Save the original values so they can be used to look up cover images.
	if over.AlbumID != nil && *over.AlbumID != song.AlbumID {
		song.OrigAlbumID = song.AlbumID
		song.AlbumID = *over.AlbumID
	}
	if over.RecordingID != nil && *over.RecordingID != song.RecordingID {
		song.OrigRecordingID = song.RecordingID
		song.RecordingID = *over.RecordingID
	}

	setInt(&song.Track, over.Track)
	setInt(&song.Disc, over.Disc)
	setTime(&song.Date, over.Date)

	return nil
}

// TODO: Sigh, use generics if dev_appserver.py ever supports modern Go.
func setString(dst, src *string) {
	if src != nil {
		*dst = *src
	}
}
func setInt(dst, src *int) {
	if src != nil {
		*dst = *src
	}
}
func setTime(dst, src *time.Time) {
	if src != nil {
		*dst = *src
	}
}
