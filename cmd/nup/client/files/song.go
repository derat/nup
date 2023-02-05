// Copyright 2022 Daniel Erat.
// All rights reserved.

// Package files contains client code for reading song files.
package files

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/derat/mpeg"
	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
	"github.com/derat/taglib-go/taglib"
)

const (
	albumIDTag          = "MusicBrainz Album Id"   // usually used as cover ID
	coverIDTag          = "nup Cover Id"           // can be set for non-MusicBrainz tracks
	recordingIDOwner    = "http://musicbrainz.org" // UFID for Song.RecordingID
	nonAlbumTracksValue = "[non-album tracks]"     // used by MusicBrainz/Picard for standalone recordings
)

// ReadSong reads the song file at p and creates a Song object.
//
// If fi is non-nil, it will be used; otherwise the file will be stat-ed by this function.
// If onlyTags is true, only fields derived from the file's MP3 tags will be filled
// (specifically, the song's SHA1, duration, and gain adjustments will not be computed).
// gc is only used if cfg.ComputeGains is true and onlyTags is false.
func ReadSong(cfg *client.Config, p string, fi os.FileInfo, onlyTags bool,
	gc *GainsCache) (*db.Song, error) {
	relPath, err := filepath.Rel(cfg.MusicDir, p)
	if err != nil {
		return nil, fmt.Errorf("%q isn't subpath of %q: %v", p, cfg.MusicDir, err)
	}

	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if fi == nil {
		if fi, err = f.Stat(); err != nil {
			return nil, err
		}
	}

	s := db.Song{Filename: relPath}

	var headerLen, footerLen int64
	if tag, err := mpeg.ReadID3v1Footer(f, fi); err != nil {
		return nil, err
	} else if tag != nil {
		footerLen = mpeg.ID3v1Length
		s.Artist = tag.Artist
		s.Title = tag.Title
		s.Album = tag.Album
		if year, err := strconv.Atoi(tag.Year); err == nil {
			s.Date = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		}
	}

	if tag, err := taglib.Decode(f, fi.Size()); err != nil {
		// Tolerate missing ID3v2 tags if we got an artist and title from ID3v1.
		if len(s.Artist) == 0 && len(s.Title) == 0 {
			return nil, err
		}
	} else {
		s.Artist = tag.Artist()
		s.Title = tag.Title()
		s.Album = tag.Album()
		s.AlbumID = tag.CustomFrames()[albumIDTag]
		s.CoverID = tag.CustomFrames()[coverIDTag]
		s.RecordingID = tag.UniqueFileIdentifiers()[recordingIDOwner]
		s.Track = int(tag.Track())
		s.Disc = int(tag.Disc())
		headerLen = int64(tag.TagSize())

		if date, err := getSongDate(tag); err != nil {
			return nil, err
		} else if !date.IsZero() {
			s.Date = date
		}

		// ID3 v2.4 defines TPE2 (Band/orchestra/accompaniment) as
		// "additional information about the performers in the recording".
		// Only save the album artist if it's different from the track artist.
		if aa, err := mpeg.GetID3v2TextFrame(tag, "TPE2"); err != nil {
			return nil, err
		} else if aa != s.Artist {
			s.AlbumArtist = aa
		}

		// TSST (Set subtitle) contains the disc's subtitle.
		// Most multi-disc albums don't have subtitles.
		if s.DiscSubtitle, err = mpeg.GetID3v2TextFrame(tag, "TSST"); err != nil {
			return nil, err
		}

		// Some old files might be missing the TPOS "part of set" frame.
		// Assume that they're from a single-disc album in that case:
		// https://github.com/derat/nup/issues/37
		if s.Disc == 0 && s.Track > 0 && s.Album != nonAlbumTracksValue {
			s.Disc = 1
		}
	}

	if repl, ok := cfg.ArtistRewrites[s.Artist]; ok {
		s.Artist = repl
	}

	if repl, ok := cfg.AlbumIDRewrites[s.AlbumID]; ok {
		// Look for a cover image corresponding to the original ID as well.
		// Don't bother setting this if the rewrite didn't actually change anything
		// (i.e. it was just defined to set the disc number).
		if s.CoverID == "" && s.AlbumID != repl {
			s.CoverID = s.AlbumID
		}
		s.AlbumID = repl

		// Extract the disc number and subtitle from the album name.
		if album, disc, subtitle := extractAlbumDisc(s.Album); disc != 0 {
			s.Album = album
			s.Disc = disc
			if s.DiscSubtitle == "" {
				s.DiscSubtitle = subtitle
			}
		}
	}

	if onlyTags {
		return &s, nil
	}

	s.SHA1, err = mpeg.ComputeAudioSHA1(f, fi, headerLen, footerLen)
	if err != nil {
		return nil, err
	}
	dur, _, _, err := mpeg.ComputeAudioDuration(f, fi, headerLen, footerLen)
	if err != nil {
		return nil, err
	}
	s.Length = dur.Seconds()

	if cfg.ComputeGain {
		gain, err := gc.get(p, s.Album, s.AlbumID)
		if err != nil {
			return nil, err
		}
		s.TrackGain = gain.TrackGain
		s.AlbumGain = gain.AlbumGain
		s.PeakAmp = gain.PeakAmp
	}

	return &s, nil
}

// extractAlbumDisc attempts to extract a disc number and optional title from an album name.
// "Some Album (disc 2: The Second Disc)" is split into "Some Album", 2, and "The Second Disc".
// If disc information cannot be extracted, the original album name and 0 are returned.
func extractAlbumDisc(orig string) (album string, discNum int, discTitle string) {
	ms := albumDiscRegexp.FindStringSubmatch(orig)
	if ms == nil {
		return orig, 0, ""
	}
	var err error
	if discNum, err = strconv.Atoi(ms[1]); err != nil {
		discNum = 0
	}
	return orig[:len(orig)-len(ms[0])], discNum, ms[2]
}

// albumDiscRegexp matches pre-NGS MusicBrainz album names used for multi-disc releases.
// The first subgroup contains the disc number, while the second subgroup contains
// the disc/medium title (if any).
var albumDiscRegexp = regexp.MustCompile(`\s+\(disc (\d+)(?::\s+([^)]+))?\)$`)

// getSongDate tries to extract a song's release or recording date.
func getSongDate(tag taglib.GenericTag) (time.Time, error) {
	for _, tt := range []mpeg.TimeType{
		mpeg.OriginalReleaseTime,
		mpeg.RecordingTime,
		mpeg.ReleaseTime,
	} {
		if tm, err := mpeg.GetID3v2Time(tag, tt); err != nil {
			return time.Time{}, err
		} else if !tm.Empty() {
			return tm.Time(), nil
		}
	}
	return time.Time{}, nil
}

// IsMusicPath returns true if path p has an extension suggesting that it's a music file.
func IsMusicPath(p string) bool {
	// TODO: Add support for other file types someday, maybe.
	// At the very least, ReadSong would need to be updated to understand non-MPEG files.
	return strings.ToLower(filepath.Ext(p)) == ".mp3"
}
