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

// GenMetadataOverride creates a MetadataOverride struct containing the differences
// between orig and updated. New values are allocated for the returned struct,
// so it is safe to modify orig and updated after calling this function.
func GenMetadataOverride(orig, updated *db.Song) *MetadataOverride {
	var over MetadataOverride
	for _, info := range []struct {
		before, after string
		dst           **string
	}{
		{orig.Artist, updated.Artist, &over.Artist},
		{orig.Title, updated.Title, &over.Title},
		{orig.Album, updated.Album, &over.Album},
		{orig.AlbumArtist, updated.AlbumArtist, &over.AlbumArtist},
		{orig.DiscSubtitle, updated.DiscSubtitle, &over.DiscSubtitle},
		{orig.AlbumID, updated.AlbumID, &over.AlbumID},
		{orig.RecordingID, updated.RecordingID, &over.RecordingID},
	} {
		if info.before != info.after {
			*info.dst = newString(info.after)
		}
	}
	if orig.Track != updated.Track {
		over.Track = newInt(updated.Track)
	}
	if orig.Disc != updated.Disc {
		over.Disc = newInt(updated.Disc)
	}
	if !orig.Date.Equal(updated.Date) {
		over.Date = newTime(updated.Date)
	}

	return &over
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
	var p string
	var f *os.File
	var err error
	if p, err = MetadataOverridePath(cfg, song.Filename); err != nil {
		return nil // metadata dir unconfigured
	} else if f, err = os.Open(p); os.IsNotExist(err) {
		return nil // override file doesn't exist
	} else if err != nil {
		return err // some other problem opening override file
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
func newString(v string) *string     { return &v }
func newInt(v int) *int              { return &v }
func newTime(v time.Time) *time.Time { return &v }

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
