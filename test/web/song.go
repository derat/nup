// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"time"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

var (
	// Pull some stuff into our namespace for convenience.
	song0s  = test.Song0s
	song1s  = test.Song1s
	song5s  = test.Song5s
	song10s = test.Song10s
)

func newSong(artist, title, album string, opts ...songOpt) db.Song {
	s := db.Song{
		Artist:   artist,
		Title:    title,
		Album:    album,
		SHA1:     fmt.Sprintf("%s-%s-%s", artist, title, album),
		AlbumID:  artist + "-" + album,
		Filename: song10s.Filename,
	}
	for _, opt := range opts {
		opt(&s)
	}
	// Gross hack: infer the length from the filename.
	if s.Length == 0 {
		for _, ks := range []db.Song{song0s, song1s, song5s, song10s} {
			if s.Filename == ks.Filename {
				s.Length = ks.Length
			}
		}
	}
	return s
}

type songOpt func(*db.Song)

func withDisc(d int) songOpt        { return func(s *db.Song) { s.Disc = d } }
func withFilename(f string) songOpt { return func(s *db.Song) { s.Filename = f } }
func withLength(l float64) songOpt  { return func(s *db.Song) { s.Length = l } }
func withRating(r float64) songOpt  { return func(s *db.Song) { s.Rating = r } }
func withTags(t ...string) songOpt  { return func(s *db.Song) { s.Tags = t } }
func withTrack(t int) songOpt       { return func(s *db.Song) { s.Track = t } }

func withPlays(times ...int64) songOpt {
	return func(s *db.Song) {
		for _, t := range times {
			s.Plays = append(s.Plays, db.NewPlay(time.Unix(t, 0), ""))
		}
	}
}

// joinSongs flattens songs, consisting of db.Song and []db.Song items, into a single slice.
func joinSongs(songs ...interface{}) []db.Song {
	var all []db.Song
	for _, s := range songs {
		if ts, ok := s.(db.Song); ok {
			all = append(all, ts)
		} else if ts, ok := s.([]db.Song); ok {
			all = append(all, ts...)
		} else {
			panic("Invalid type (must be db.Song or []db.Song)")
		}
	}
	return all
}

// songInfo contains information about a song in the web interface.
type songInfo struct {
	artist, title, album  string
	active, checked, menu bool // song row is active, checked, or has context menu
	paused, ended         bool // audio element is paused or ended
	src                   string
}

func (s songInfo) String() string {
	str := fmt.Sprintf("%q %q %q", s.artist, s.title, s.album)
	for _, f := range []struct {
		name string
		val  bool
	}{
		{"active", s.active},
		{"checked", s.checked},
		{"ended", s.ended},
		{"menu", s.menu},
		{"paused", s.paused},
	} {
		if f.val {
			str += " " + f.name
		}
	}
	return "[" + str + "]"
}

type songFlags uint32

const (
	songActive songFlags = 1 << iota
	songNotActive
	songChecked
	songNotChecked
	songEnded
	songNotEnded
	songMenu
	songNoMenu
	songPaused
	songNotPaused
)

func compareSongInfo(got songInfo, want db.Song, flags songFlags) bool {
	if got.artist != want.Artist || got.title != want.Title || got.album != want.Album {
		return false
	}
	for _, f := range []struct {
		pos, neg songFlags
		val      bool
	}{
		{songActive, songNotActive, got.active},
		{songChecked, songNotChecked, got.checked},
		{songEnded, songNotEnded, got.ended},
		{songMenu, songNoMenu, got.menu},
		{songPaused, songNotPaused, got.paused},
	} {
		if (flags&f.pos != 0 && !f.val) || (flags&f.neg != 0 && f.val) {
			return false
		}
	}
	return true
}

func compareSongInfos(got []songInfo, want []db.Song, checked []bool, active, menu int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		var flags songFlags
		if checked != nil {
			if checked[i] {
				flags |= songChecked
			} else {
				flags |= songNotChecked
			}
		}
		if active >= 0 {
			if active == i {
				flags |= songActive
			} else {
				flags |= songNotActive
			}
		}
		if menu == i {
			flags |= songMenu
		} else {
			flags |= songNoMenu
		}
	}
	return true
}