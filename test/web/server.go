// Copyright 2022 Daniel Erat.
// All rights reserved.

package web

import (
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

// server is used by tests to interact with the server.
type server struct {
	t      *testing.T
	tester *test.Tester
}

// checkSong verifies that the server's data for song (identified by
// SHA1) matches the expected. This is used to verify user data: see
// hasSrvRating, hasSrvTags, and hasSrvPlay in song.go.
func (srv *server) checkSong(song db.Song, checks ...songCheck) {
	want := makeSongInfo(song)
	for _, c := range checks {
		c(&want)
	}

	var got *songInfo
	if err := waitFull(func() error {
		got = nil
		for _, s := range srv.tester.DumpSongs(test.KeepIDs) {
			if s.SHA1 != song.SHA1 {
				continue
			}
			si := makeSongInfo(s)
			si.srvRating = &s.Rating
			si.srvTags = s.Tags
			sort.Sort(db.PlayArray(s.Plays))
			for _, p := range s.Plays {
				si.srvPlays = append(si.srvPlays, [2]time.Time{p.StartTime, p.StartTime})
			}
			got = &si
			break
		}
		if got == nil || !songInfosEqual(want, *got) {
			return errors.New("songs don't match")
		}
		return nil
	}, want.getTimeout(waitTimeout), waitSleep); err != nil {
		srv.t.Fatal(fmt.Sprintf("Bad server %q data at %v:\n", song.SHA1, test.Caller()) +
			"  Want: " + want.String() + "\n" +
			"  Got:  " + got.String() + "\n")
	}
}
