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

	// Keywords contains words from ArtistLower, TitleLower, and AlbumLower used for searching.
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

	// Rating is the song's rating in the range [0.0, 1.0], or -1 if unrated.
	// The server should call SetRating to additionally update the RatingAtLeast* fields.
	Rating float64 `json:"rating"`

	// RatingAtLeast* are true if Rating is at least 0.75, 0.5, 0.25, or 0.
	// These are maintained to sidestep Datastore's restriction against using multiple
	// inequality filters in a query.
	RatingAtLeast75 bool `json:"-"`
	RatingAtLeast50 bool `json:"-"`
	RatingAtLeast25 bool `json:"-"`
	RatingAtLeast0  bool `json:"-"`

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

// Update copies fields from src to s.
// If copyUserData is true, the Rating*, FirstStartTime, LastStartTime,
// NupPlays, and Tags fields are also copied; otherwise they are left unchanged.
func (s *Song) Update(src *Song, copyUserData bool) error {
	s.SHA1 = src.SHA1
	s.Filename = src.Filename
	s.CoverFilename = src.CoverFilename
	s.Artist = src.Artist
	s.Title = src.Title
	s.Album = src.Album
	s.AlbumID = src.AlbumID
	s.Track = src.Track
	s.Disc = src.Disc
	s.Length = src.Length
	s.TrackGain = src.TrackGain
	s.AlbumGain = src.AlbumGain
	s.PeakAmp = src.PeakAmp

	var err error
	if s.ArtistLower, err = Normalize(src.Artist); err != nil {
		return fmt.Errorf("normalizing %q: %v", src.Artist, err)
	}
	if s.TitleLower, err = Normalize(src.Title); err != nil {
		return fmt.Errorf("normalizing %q: %v", src.Title, err)
	}
	if s.AlbumLower, err = Normalize(src.Album); err != nil {
		return fmt.Errorf("normalizing %q: %v", src.Album, err)
	}

	keywords := make(map[string]bool)
	for _, str := range []string{s.ArtistLower, s.TitleLower, s.AlbumLower} {
		for _, w := range strings.FieldsFunc(str, func(c rune) bool {
			return !unicode.IsLetter(c) && !unicode.IsNumber(c)
		}) {
			keywords[w] = true
		}
	}
	s.Keywords = make([]string, len(keywords))
	i := 0
	for w := range keywords {
		s.Keywords[i] = w
		i++
	}
	sort.Strings(s.Keywords)

	if copyUserData {
		s.SetRating(src.Rating)
		s.FirstStartTime = src.FirstStartTime
		s.LastStartTime = src.LastStartTime
		s.NumPlays = src.NumPlays
		s.Tags = src.Tags
		sort.Strings(s.Tags)
	}
	return nil
}

// SetRating sets Rating to r and updates RatingAtLeast*.
func (s *Song) SetRating(r float64) {
	s.Rating = r
	s.RatingAtLeast75 = r >= 0.75
	s.RatingAtLeast50 = r >= 0.5
	s.RatingAtLeast25 = r >= 0.25
	s.RatingAtLeast0 = r >= 0
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
