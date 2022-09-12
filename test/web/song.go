// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

// newSong creates a new db.Song containing the supplied data.
func newSong(artist, title, album string, fields ...songField) db.Song {
	s := db.Song{
		Artist:   artist,
		Title:    title,
		Album:    album,
		SHA1:     fmt.Sprintf("%s-%s-%s", artist, title, album),
		AlbumID:  artist + "-" + album,
		Filename: test.Song10s.Filename,
	}
	for _, f := range fields {
		f(&s)
	}
	// Gross hack: infer the length from the filename.
	if s.Length == 0 {
		for _, ks := range []db.Song{test.Song0s, test.Song1s, test.Song5s, test.Song10s} {
			if s.Filename == ks.Filename {
				s.Length = ks.Length
			}
		}
	}
	return s
}

// songField describes a field that should be set by newSong.
type songField func(*db.Song)

func withDate(t time.Time) songField  { return func(s *db.Song) { s.Date = t } }
func withDisc(d int) songField        { return func(s *db.Song) { s.Disc = d } }
func withFilename(f string) songField { return func(s *db.Song) { s.Filename = f } }
func withLength(l float64) songField  { return func(s *db.Song) { s.Length = l } }
func withRating(r int) songField      { return func(s *db.Song) { s.Rating = r } }
func withTags(t ...string) songField  { return func(s *db.Song) { s.Tags = t } }
func withTrack(t int) songField       { return func(s *db.Song) { s.Track = t } }
func withPlays(ts ...time.Time) songField {
	return func(s *db.Song) {
		for _, t := range ts {
			s.Plays = append(s.Plays, db.NewPlay(t, ""))
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
			panic(fmt.Sprintf("Invalid type %T (must be db.Song or []db.Song)", s))
		}
	}
	return all
}

// songInfo contains information about a song in the web interface or server.
//
// I've made a few attempts to get rid of the use of db.Song in this code
// (other than when posting songs to the server), but it's harder than it
// seems: for example, songs always need filenames when they're sent to the
// server, but we don't want to check filenames when we're inspecting the
// playlist or search results.
type songInfo struct {
	artist, title, album string // metadata from either <song-table> row or <play-view>

	active  *bool // song row is active/highlighted
	checked *bool // song row is checked
	menu    *bool // song row has context menu
	paused  *bool // audio element is paused
	ended   *bool // audio element is ended

	rating   *int    // rating from the cover image in [0, 5]
	filename *string // filename from audio element src attribute
	imgTitle *string // cover image title attr, e.g. "Rating: ★★★☆☆\nTags: guitar rock"
	timeStr  *string // displayed time, e.g. "0:00 / 0:05"

	srvRating *int           // server rating in [1, 5] or 0 for unrated
	srvTags   []string       // server tags in ascending order
	srvPlays  [][2]time.Time // server play time lower/upper bounds in ascending order

	timeout *time.Duration // hack for using longer timeouts in checks
}

// makeSongInfo constructs a basic songInfo using data from s.
func makeSongInfo(s db.Song) songInfo {
	return songInfo{
		artist: s.Artist,
		title:  s.Title,
		album:  s.Album,
	}
}

func (s *songInfo) String() string {
	if s == nil {
		return "nil"
	}

	str := fmt.Sprintf("%q %q %q", s.artist, s.title, s.album)

	// Describe optional bools.
	for _, f := range []struct {
		pos, neg string
		val      *bool
	}{
		{"active", "inactive", s.active},
		{"checked", "unchecked", s.checked},
		{"ended", "unended", s.ended},
		{"menu", "no-menu", s.menu},
		{"paused", "playing", s.paused},
	} {
		if f.val != nil {
			if *f.val {
				str += " " + f.pos
			} else {
				str += " " + f.neg
			}
		}
	}

	// Describe optional strings.
	for _, f := range []struct {
		name string
		val  *string
	}{
		{"filename", s.filename},
		{"time", s.timeStr},
		{"title", s.imgTitle},
	} {
		if f.val != nil {
			str += fmt.Sprintf(" %s=%q", f.name, *f.val)
		}
	}

	// Describe optional ints.
	for _, f := range []struct {
		name string
		val  *int
	}{
		{"rating", s.rating},
		{"srvRating", s.srvRating},
	} {
		if f.val != nil {
			str += fmt.Sprintf(" %s=%d", f.name, *f.val)
		}
	}

	// Add other miscellaneous junk.
	if s.srvTags != nil {
		str += fmt.Sprintf(" tags=%v", s.srvTags)
	}
	if s.srvPlays != nil {
		const tf = "2006-01-02-15:04:05"
		var ps []string
		for _, p := range s.srvPlays {
			if p[0].Equal(p[1]) {
				ps = append(ps, p[0].Local().Format(tf))
			} else {
				ps = append(ps, p[0].Local().Format(tf)+"/"+p[1].Local().Format(tf))
			}
		}
		str += fmt.Sprintf(" plays=[%s]", strings.Join(ps, " "))
	}

	return "[" + str + "]"
}

// getTimeout retuns s.timeout if non-nil or def otherwise.
func (s *songInfo) getTimeout(def time.Duration) time.Duration {
	if s.timeout != nil {
		return *s.timeout
	}
	return def
}

// songCheck specifies a check to perform on a song.
type songCheck func(*songInfo)

// See equivalently-named fields in songInfo for more info.
func isPaused(p bool) songCheck        { return func(i *songInfo) { i.paused = &p } }
func isEnded(e bool) songCheck         { return func(i *songInfo) { i.ended = &e } }
func hasFilename(f string) songCheck   { return func(i *songInfo) { i.filename = &f } }
func hasImgTitle(t string) songCheck   { return func(i *songInfo) { i.imgTitle = &t } }
func hasRating(r int) songCheck        { return func(i *songInfo) { i.rating = &r } }
func hasTimeStr(s string) songCheck    { return func(i *songInfo) { i.timeStr = &s } }
func hasSrvRating(r int) songCheck     { return func(i *songInfo) { i.srvRating = &r } }
func hasSrvTags(t ...string) songCheck { return func(i *songInfo) { i.srvTags = t } }

// hasSrvPlay should be called once for each play (in ascending order).
func hasSrvPlay(lower, upper time.Time) songCheck {
	return func(si *songInfo) {
		si.srvPlays = append(si.srvPlays, [2]time.Time{lower, upper})
	}
}

// hasNoSrvPlays asserts that there are no recorded plays.
func hasNoSrvPlays() songCheck { return func(i *songInfo) { i.srvPlays = [][2]time.Time{} } }

// useTimeout sets a custom timeout to use when waiting for the condition.
func useTimeout(t time.Duration) songCheck { return func(i *songInfo) { i.timeout = &t } }

// songInfosEqual returns true if want and got have the same artist, title, and album
// and any additional optional fields specified in want also match.
func songInfosEqual(want, got songInfo) bool {
	// Compare bools.
	for _, t := range []struct {
		want, got *bool
	}{
		{want.active, got.active},
		{want.checked, got.checked},
		{want.ended, got.ended},
		{want.menu, got.menu},
		{want.paused, got.paused},
	} {
		if t.want != nil && (t.got == nil || *t.got != *t.want) {
			return false
		}
	}

	// Compare strings.
	for _, t := range []struct {
		want, got *string
	}{
		{&want.artist, &got.artist},
		{&want.title, &got.title},
		{&want.album, &got.album},
		{want.filename, got.filename},
		{want.imgTitle, got.imgTitle},
		{want.timeStr, got.timeStr},
	} {
		if t.want != nil && (t.got == nil || *t.got != *t.want) {
			return false
		}
	}

	// Compare ints.
	for _, t := range []struct {
		want, got *int
	}{
		{want.rating, got.rating},
		{want.srvRating, got.srvRating},
	} {
		if t.want != nil && (t.got == nil || *t.got != *t.want) {
			return false
		}
	}

	if want.srvTags != nil && !reflect.DeepEqual(want.srvTags, got.srvTags) {
		return false
	}
	if want.srvPlays != nil {
		if len(want.srvPlays) != len(got.srvPlays) {
			return false
		}
		for i, bounds := range want.srvPlays {
			t := got.srvPlays[i][0]
			if t.Before(bounds[0]) || t.After(bounds[1]) {
				return false
			}
		}
	}

	return true
}

// songListCheck specifies a check to perform on a list of songs.
type songListCheck func([]songInfo)

// hasChecked checks that songs' checkboxes match vals.
func hasChecked(vals ...bool) songListCheck {
	return func(infos []songInfo) {
		for i := range infos {
			infos[i].checked = &vals[i]
		}
	}
}

// hasActive indicates that the song at idx should be active.
func hasActive(idx int) songListCheck {
	return func(infos []songInfo) {
		for i := range infos {
			v := i == idx
			infos[i].active = &v
		}
	}
}

// hasMenu indicates that a context menu should be shown for the song at idx.
func hasMenu(idx int) songListCheck {
	return func(infos []songInfo) {
		for i := range infos {
			v := i == idx
			infos[i].menu = &v
		}
	}
}

// songInfoSlicesEqual returns true if want and got are the same length
// and songInfosEqual returns true for corresponding elements.
func songInfoSlicesEqual(want, got []songInfo) bool {
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if !songInfosEqual(want[i], got[i]) {
			return false
		}
	}
	return true
}
