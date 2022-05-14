// Copyright 2020 Daniel Erat.
// All rights reserved.

package db

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	// Datastore kinds of various objects.
	PlayKind        = "Play"
	SongKind        = "Song"
	DeletedPlayKind = "DeletedPlay"
	DeletedSongKind = "DeletedSong"
)

// Song represents an audio file and holds metadata and user-generated data.
//
// When adding fields, the Update method must be updated.
type Song struct {
	// SHA1 is a hash of the audio portion of the file.
	SHA1 string `datastore:"Sha1" json:"sha1,omitempty"`

	// SongID is the Song entity's key ID from Datastore. Only set in search results.
	SongID string `datastore:"-" json:"songId,omitempty"`

	// Filename is a relative path from the base of the music directory.
	// Clients can pass this to the server's /song endpoint to download the
	// song's music data.
	Filename string `json:"filename,omitempty"`

	// CoverFilename is a relative path from the base of the covers directory.
	// Must be escaped for Cloud Storage when constructing CoverURL.
	// Clients can pass this to the server's /cover endpoint to get a scaled
	// copy of the cover.
	CoverFilename string `datastore:",noindex" json:"coverFilename,omitempty"`

	// Canonical versions used for display.
	Artist string `datastore:",noindex" json:"artist"`
	Title  string `datastore:",noindex" json:"title"`
	Album  string `datastore:",noindex" json:"album"`

	// Lowercase versions used for searching and sorting.
	// Additional normalization is performed: see query.Normalize.
	ArtistLower string `json:"-"`
	TitleLower  string `json:"-"`
	AlbumLower  string `json:"-"`

	// AlbumArtist contains the album's artist if it isn't the same as Artist.
	// This corresponds to the TPE2 ID3 tag, which may hold the performer name
	// in the case of a classical album, or the remixer name in the case of an
	// album consisting of songs remixed by a single artist.
	AlbumArtist string `datastore:",noindex" json:"albumArtist,omitempty"`

	// Keywords contains words from ArtistLower, TitleLower, AlbumLower, and
	// AlbumArtist (after normalization). It is used for searching.
	Keywords []string `json:"-"`

	// AlbumID is an opaque ID uniquely identifying the album
	// (generally, a MusicBrainz release ID taken from a "MusicBrainz Album Id" ID3v2 tag).
	AlbumID string `datastore:"AlbumId" json:"albumId,omitempty"`

	// CoverID is an opaque ID from a "nup Cover Id" ID3v2 tag used to specify cover art.
	// If unset, AlbumID and then RecordingID is used when looking for art.
	CoverID string `datastore:"-" json:"-"`

	// RecordingID is an opaque ID uniquely identifying the recording
	// (generally, the MusicBrainz ID corresponding to the MusicBrainz recording entity,
	// taken from a UFID ID3v2 tag). Only used to find cover art if neither AlbumID nor CoverID
	// is set.
	RecordingID string `datastore:"-" json:"-"`

	// Track is the song's track number, or 0 if unset.
	Track int `json:"track"`
	// Disc is the song's disc number, or 0 if unset.
	Disc int `json:"disc"`

	// Length is the song's duration in seconds.
	Length float64 `json:"length"`

	// TrackGain is the song's dB gain adjustment independent of its album. More info:
	//  https://en.wikipedia.org/wiki/ReplayGain
	//  https://wiki.hydrogenaud.io/index.php?title=ReplayGain_specification
	//  https://productionadvice.co.uk/tidal-normalization-upgrade/
	TrackGain float64 `datastore:",noindex" json:"trackGain"`
	// AlbumGain is the song's dB gain adjustment relative to its album.
	AlbumGain float64 `datastore:",noindex" json:"albumGain"`
	// PeakAmp is the song's peak amplitude, with 1.0 representing the highest
	// amplitude that can be played without clipping.
	PeakAmp float64 `datastore:",noindex" json:"peakAmp"`

	// Rating is the song's rating in the range [1, 5], or 0 if unrated.
	// The server should call SetRating to additionally update the RatingAtLeast* fields.
	Rating int `json:"rating"`

	// RatingAtLeast* are true if Rating is at least the specified value.
	// These are maintained to sidestep Datastore's restriction against using multiple
	// inequality filters in a query.
	RatingAtLeast1 bool `json:"-"`
	RatingAtLeast2 bool `json:"-"`
	RatingAtLeast3 bool `json:"-"`
	RatingAtLeast4 bool `json:"-"`

	// FirstStartTime is the first time the song was played.
	FirstStartTime time.Time `json:"-"`
	// LastStartTime is the last time the song was played.
	LastStartTime time.Time `json:"-"`

	// NumPlays is the number of times the song has been played.
	NumPlays int `json:"-"`

	// Plays contains the song's playback history.
	// Only used for importing data -- in Datastore, Play is a descendant of Song.
	Plays []Play `datastore:"-" json:"plays,omitempty"`

	// Tags contains tags assigned to the song by the user.
	Tags []string `json:"tags"`

	// LastModifiedTime is the time that the song was modified.
	LastModifiedTime time.Time `json:"-"`
}

// MetadataEquals returns true if s and o have identical metadata.
// User data (ratings, plays, tags) and server-managed fields are not checked.
func (s *Song) MetadataEquals(o *Song) bool {
	return s.SHA1 == o.SHA1 &&
		s.Filename == o.Filename &&
		s.CoverFilename == o.CoverFilename &&
		s.Artist == o.Artist &&
		s.Title == o.Title &&
		s.Album == o.Album &&
		s.AlbumArtist == o.AlbumArtist &&
		s.AlbumID == o.AlbumID &&
		s.Track == o.Track &&
		s.Disc == o.Disc &&
		s.Length == o.Length &&
		s.TrackGain == o.TrackGain &&
		s.AlbumGain == o.AlbumGain &&
		s.PeakAmp == o.PeakAmp
}

// Update copies fields from src to dst.
//
// If copyUserData is true, the Rating*, FirstStartTime, LastStartTime,
// NupPlays, and Tags fields are also copied; otherwise they are left unchanged.
//
// ArtistLower, TitleLower, AlbumLower, and Keywords are also initialized in dst,
// and Clean is called.
func (dst *Song) Update(src *Song, copyUserData bool) error {
	dst.SHA1 = src.SHA1
	dst.Filename = src.Filename
	dst.CoverFilename = src.CoverFilename
	dst.Artist = src.Artist
	dst.Title = src.Title
	dst.Album = src.Album
	dst.AlbumArtist = src.AlbumArtist
	dst.AlbumID = src.AlbumID
	dst.Track = src.Track
	dst.Disc = src.Disc
	dst.Length = src.Length
	dst.TrackGain = src.TrackGain
	dst.AlbumGain = src.AlbumGain
	dst.PeakAmp = src.PeakAmp

	var err error
	if dst.ArtistLower, err = Normalize(dst.Artist); err != nil {
		return fmt.Errorf("normalizing %q: %v", src.Artist, err)
	}
	if dst.TitleLower, err = Normalize(dst.Title); err != nil {
		return fmt.Errorf("normalizing %q: %v", src.Title, err)
	}
	if dst.AlbumLower, err = Normalize(dst.Album); err != nil {
		return fmt.Errorf("normalizing %q: %v", src.Album, err)
	}

	// AlbumArtist is empty if it's the same as Artist. The normalized
	// version of it isn't stored, but it gets included in Keywords.
	albumArtistNorm, err := Normalize(dst.AlbumArtist)
	if err != nil {
		return fmt.Errorf("normalizing %q: %v", dst.AlbumArtist, err)
	}

	// Keywords are sorted and deduped in the later call to Clean.
	for _, str := range []string{dst.ArtistLower, dst.TitleLower, dst.AlbumLower, albumArtistNorm} {
		for _, w := range strings.FieldsFunc(str, func(c rune) bool {
			return !unicode.IsLetter(c) && !unicode.IsNumber(c)
		}) {
			dst.Keywords = append(dst.Keywords, w)
		}
	}

	if copyUserData {
		dst.SetRating(src.Rating)
		dst.FirstStartTime = src.FirstStartTime
		dst.LastStartTime = src.LastStartTime
		dst.NumPlays = src.NumPlays
		dst.Tags = append([]string(nil), src.Tags...)
	}

	dst.Clean()
	return nil
}

// SetRating sets Rating to r and updates RatingAtLeast*.
func (s *Song) SetRating(r int) {
	s.Rating = r
	s.RatingAtLeast1 = r >= 1
	s.RatingAtLeast2 = r >= 2
	s.RatingAtLeast3 = r >= 3
	s.RatingAtLeast4 = r >= 4
}

// UpdatePlayStats updates NumPlays, FirstStartTime, and LastStartTime to
// reflect an additional play starting at startTime.
func (s *Song) UpdatePlayStats(startTime time.Time) {
	s.NumPlays++
	if s.FirstStartTime.IsZero() || startTime.Before(s.FirstStartTime) {
		s.FirstStartTime = startTime
	}
	if s.LastStartTime.IsZero() || startTime.After(s.LastStartTime) {
		s.LastStartTime = startTime
	}
}

// RebuildPlayStats regenerates NumPlays, FirstStartTime, and LastStartTime based
// on the supplied plays.
func (s *Song) RebuildPlayStats(plays []Play) {
	s.NumPlays = 0
	s.FirstStartTime = time.Time{}
	s.LastStartTime = time.Time{}
	for _, p := range plays {
		s.UpdatePlayStats(p.StartTime)
	}
}

// Clean sorts and removes duplicates from slice fields in s.
func (s *Song) Clean() {
	sort.Strings(s.Keywords)
	s.Keywords = dedupeSortedStrings(s.Keywords)

	sort.Strings(s.Tags)
	s.Tags = dedupeSortedStrings(s.Tags)

	sort.Sort(PlayArray(s.Plays))
	s.Plays = dedupeSortedPlays(s.Plays)
}

func dedupeSortedStrings(full []string) []string {
	var src, dst int
	for ; src < len(full); src++ {
		if src == 0 || full[src] != full[src-1] {
			full[dst] = full[src]
			dst++
		}
	}
	return full[:dst]
}

// https://go.dev/blog/normalization#performing-magic
var normalizer = transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)))

// Normalize normalizes s for searches.
//
// NFKD form is used. Unicode characters are decomposed (runes are broken into their components) and
// replaced for compatibility equivalence (characters that represent the same characters but have
// different visual representations, e.g. '9' and '⁹', are equal). Visually-similar characters from
// different alphabets will not be equal, however (e.g. Latin 'o', Greek 'ο', and Cyrillic 'о').
// See https://go.dev/blog/normalization for more details.
//
// Characters are also de-accented and lowercased, but punctuation is preserved.
func Normalize(s string) (string, error) {
	b := make([]byte, len(s))
	_, _, err := normalizer.Transform(b, []byte(s), true)
	if err != nil {
		return "", err
	}
	b = bytes.TrimRight(b, "\x00")
	return strings.ToLower(string(b)), nil
}

// Play represents one playback of a Song.
type Play struct {
	// StartTime is the time at which playback started.
	StartTime time.Time `json:"t"`
	// IPAddress is the IPv4 or IPv6 address of the client playing the song.
	IPAddress string `datastore:"IpAddress" json:"ip"`
}

func NewPlay(t time.Time, ip string) Play { return Play{t, ip} }

func (p *Play) Equal(o *Play) bool {
	return p.StartTime.Equal(o.StartTime) && p.IPAddress == o.IPAddress
}

// PlayDump is used when dumping data.
type PlayDump struct {
	// Song entity's key ID from Datastore.
	SongID string `json:"songId"`
	// Play information.
	Play Play `json:"play"`
}

type PlayArray []Play

func (a PlayArray) Len() int           { return len(a) }
func (a PlayArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a PlayArray) Less(i, j int) bool { return a[i].StartTime.Before(a[j].StartTime) }

func dedupeSortedPlays(plays []Play) []Play {
	var src, dst int
	for ; src < len(plays); src++ {
		if src == 0 || !plays[src].Equal(&plays[src-1]) {
			plays[dst] = plays[src]
			dst++
		}
	}
	return plays[:dst]
}
