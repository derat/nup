// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"

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

func withTrack(track int) songOpt     { return func(s *db.Song) { s.Track = track } }
func withTags(tags ...string) songOpt { return func(s *db.Song) { s.Tags = tags } }

type songInfo struct {
	artist, title, album  string
	active, menu, checked bool
}

func compareSongInfos(got []songInfo, want []db.Song) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].artist != want[i].Artist ||
			got[i].title != want[i].Title ||
			got[i].album != want[i].Album {
			return false
		}
	}
	return true
}
