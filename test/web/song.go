// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

func newSong(artist, title, album string, fields ...songField) db.Song {
	s := db.Song{
		Artist:   artist,
		Title:    title,
		Album:    album,
		SHA1:     fmt.Sprintf("%s-%s-%s", artist, title, album),
		AlbumID:  artist + "-" + album,
		Filename: test.Song10s.Filename,
		Rating:   -1.0,
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

func withDisc(d int) songField        { return func(s *db.Song) { s.Disc = d } }
func withFilename(f string) songField { return func(s *db.Song) { s.Filename = f } }
func withLength(l float64) songField  { return func(s *db.Song) { s.Length = l } }
func withRating(r float64) songField  { return func(s *db.Song) { s.Rating = r } }
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
			panic("Invalid type (must be db.Song or []db.Song)")
		}
	}
	return all
}

// songInfo contains information about a song in the web interface.
//
// I've made a few attempts to get rid of the use of db.Song in this code
// (other than when posting songs to the server), but it's harder than it
// seems: for example, songs always need filenames when they're sent to the
// server, but we don't want to check filenames when we're inspecting the
// playlist or search results.
type songInfo struct {
	artist, title, album  string  // metadata from either <song-table> row or <music-player>
	active, checked, menu *bool   // song row is active, checked, or has context menu
	paused, ended         *bool   // audio element is paused or ended
	filename              *string // filename from audio element src attribute
	rating                *string // rating string from cover image, e.g. "★★★"
	imgTitle              *string // cover image title attr, e.g. "Rating: ★★★☆☆\nTags: guitar rock"
	time                  *string // displayed time, e.g. "[ 0:00 / 0:05 ]"
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
	if s.filename != nil {
		str += " filename=" + *s.filename
	}
	if s.time != nil {
		str += " time=" + *s.time
	}
	return "[" + str + "]"
}

// songCheck specifies a check to perform on a song.
type songCheck func(*songInfo)

// See equivalently-named fields in songInfo for more info.
func isPaused(p bool) songCheck      { return func(i *songInfo) { i.paused = &p } }
func isEnded(e bool) songCheck       { return func(i *songInfo) { i.ended = &e } }
func hasFilename(f string) songCheck { return func(i *songInfo) { i.filename = &f } }
func hasRating(r string) songCheck   { return func(i *songInfo) { i.rating = &r } }
func hasImgTitle(t string) songCheck { return func(i *songInfo) { i.imgTitle = &t } }
func hasTime(s string) songCheck     { return func(i *songInfo) { i.time = &s } }

// songInfosEqual returns true if want and got have the same artist, title, and album
// and any additional optional fields specified in want also match.
func songInfosEqual(want, got songInfo) bool {
	if want.artist != got.artist || want.title != got.title || want.album != got.album {
		return false
	}
	for _, t := range []struct {
		want *bool
		got  *bool
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
	if want.filename != nil && (got.filename == nil || *want.filename != *got.filename) {
		return false
	}
	if want.rating != nil && (got.rating == nil || *want.rating != *got.rating) {
		return false
	}
	if want.imgTitle != nil && (got.imgTitle == nil || *want.imgTitle != *got.imgTitle) {
		return false
	}
	if want.time != nil && (got.time == nil || *want.time != *got.time) {
		return false
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

// songUserData holds expected user data from the server for a single song.
type songUserData struct {
	rating float64
	tags   []string       // nil means don't check
	plays  [][2]time.Time // lower/upper bounds for timestamps, nil means don't check
}

func newSongUserData(rating float64, tags []string, plays ...[2]time.Time) songUserData {
	d := songUserData{rating, tags, plays}
	sort.Slice(d.plays, func(i, j int) bool { return d.plays[i][0].Before(d.plays[j][0]) })
	return d
}

const timeFmt = "2006-01-02-15:04:05"

func (d *songUserData) String() string {
	s := fmt.Sprintf("rating=%.2f", d.rating)
	if d.tags != nil {
		s += fmt.Sprintf(" tags=%q", d.tags)
	}
	if d.plays != nil {
		var ps []string
		for _, p := range d.plays {
			if p[0].Equal(p[1]) {
				ps = append(ps, p[0].Local().Format(timeFmt))
			} else {
				ps = append(ps, p[0].Local().Format(timeFmt)+"/"+p[1].Local().Format(timeFmt))
			}
		}
		s += fmt.Sprintf(" plays=[%s]", strings.Join(ps, " "))
	}
	return s
}

// checkServerUserData verifies that for every song in want (keyed by SHA1),
// the server contains the expected user data. The tested is failed if any
// mismatches are found.
func checkServerUserData(t *testing.T, want map[string]songUserData) {
	var got map[string]songUserData
	if err := wait(func() error {
		got = make(map[string]songUserData)
		for _, s := range tester.DumpSongs(test.KeepIDs) {
			var plays [][2]time.Time
			for _, p := range s.Plays {
				plays = append(plays, [2]time.Time{p.StartTime, p.StartTime})
			}
			data := newSongUserData(s.Rating, s.Tags, plays...)
			got[s.SHA1] = data
		}

		for sha1, wd := range want {
			gd, ok := got[sha1]
			if !ok {
				return fmt.Errorf("%v missing from server", sha1)
			}
			if gd.rating != wd.rating {
				return fmt.Errorf("%v rating is %.2f; want %.2f", sha1, gd.rating, wd.rating)
			}

			if wd.tags != nil {
				sort.Strings(gd.tags)
				sort.Strings(wd.tags)
				if !reflect.DeepEqual(gd.tags, wd.tags) {
					return fmt.Errorf("%v tags are %v; want %v", sha1, gd.tags, wd.tags)
				}
			}

			if wd.plays != nil {
				if len(gd.plays) != len(wd.plays) {
					return fmt.Errorf("%v has %v play(s); want %v", sha1, len(gd.plays), len(wd.plays))
				}
				for i := range gd.plays {
					if gp, wp := gd.plays[i], wd.plays[i]; gp[0].Before(wp[0]) || gp[0].After(wp[1]) {
						return fmt.Errorf("%v play %d has time %v outside of [%v, %v]",
							sha1, i, gp[0], wp[0], wp[1])
					}
				}
			}
		}
		return nil
	}); err != nil {
		msg := fmt.Sprintf("Bad server user data for %v: %v\n", test.Caller(), err)
		for sha1, wd := range want {
			msg += fmt.Sprintf("Song %q:\n", sha1)
			msg += "  Want: " + wd.String() + "\n"
			if gd, ok := got[sha1]; ok {
				msg += "  Got:  " + gd.String() + "\n"
			} else {
				msg += "  Got:  [missing]\n"
			}
		}
		t.Fatal(msg)
	}
}
