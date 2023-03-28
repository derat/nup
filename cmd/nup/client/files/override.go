// Copyright 2023 Daniel Erat.
// All rights reserved.

package files

import (
	"encoding/json"
	"errors"
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
// MetadataOverride struct used to override metadata for a song with the supplied
// Filename value.
func MetadataOverridePath(cfg *client.Config, songFilename string) (string, error) {
	if cfg.MetadataDir == "" {
		return "", errors.New("metadataDir not set in config")
	}
	// Not using a cutesy ".jsong" extension here is killing me.
	return filepath.Join(cfg.MetadataDir, songFilename+".json"), nil
}

// ReadMetadataOverride reads the JSON-marshaled metadata override file for the song with the
// specified Filename field. If no override exists for the song, a nil object and nil error are
// returned.
func ReadMetadataOverride(cfg *client.Config, songFilename string) (*MetadataOverride, error) {
	var p string
	var f *os.File
	var err error
	if p, err = MetadataOverridePath(cfg, songFilename); err != nil {
		return nil, nil // metadata dir unconfigured
	} else if f, err = os.Open(p); os.IsNotExist(err) {
		return nil, nil // override file doesn't exist
	} else if err != nil {
		return nil, err // some other problem opening override file
	}
	defer f.Close()

	var over MetadataOverride
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&over); err != nil {
		return nil, fmt.Errorf("%v: %v", p, err)
	}
	return &over, nil
}

// WriteMetadataOverride writes a JSON file containing overridden metadata for the song with the
// specified Filename field. The destination directory is created if it does not exist.
func WriteMetadataOverride(cfg *client.Config, songFilename string, over *MetadataOverride) error {
	p, err := MetadataOverridePath(cfg, songFilename)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(over); err != nil {
		f.Close()
		os.Remove(f.Name())
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}

// applyMetadataOverride looks for MetadataOverride JSON file in cfg.MetadataDir and
// updates specified fields in s. If no override file exists, nil is returned.
func applyMetadataOverride(cfg *client.Config, song *db.Song) error {
	over, err := ReadMetadataOverride(cfg, song.Filename)
	if err != nil {
		return err
	} else if over == nil {
		return nil
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
