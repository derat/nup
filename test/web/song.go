// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"time"

	"github.com/derat/nup/server/db"
)

func newSong(artist, title, album string, opts ...songOpt) db.Song {
	s := db.Song{
		Artist:  artist,
		Title:   title,
		Album:   album,
		SHA1:    fmt.Sprintf("%s-%s-%s", artist, title, album),
		AlbumID: artist + "-" + album,
	}
	for _, opt := range opts {
		opt(&s)
	}
	return s
}

type songOpt func(*db.Song)

func withDisc(d int) songOpt       { return func(s *db.Song) { s.Disc = d } }
func withRating(r float64) songOpt { return func(s *db.Song) { s.Rating = r } }
func withTags(t ...string) songOpt { return func(s *db.Song) { s.Tags = t } }
func withTrack(t int) songOpt      { return func(s *db.Song) { s.Track = t } }
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
	active, checked, menu bool
}

func (s songInfo) String() string {
	str := fmt.Sprintf("%q %q %q", s.artist, s.title, s.album)
	if s.active {
		str += " active"
	}
	if s.checked {
		str += " checked"
	}
	if s.menu {
		str += " menu"
	}
	return "[" + str + "]"
}

func compareSongInfos(got []songInfo, want []db.Song, checked []bool) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].artist != want[i].Artist ||
			got[i].title != want[i].Title ||
			got[i].album != want[i].Album {
			return false
		}
		if checked != nil && got[i].checked != checked[i] {
			return false
		}
	}
	return true
}
