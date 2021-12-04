// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"testing"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

func TestJSON(t *testing.T) {
	dir := t.TempDir()
	songs := []db.Song{test.LegacySong1, test.LegacySong2}
	ch := make(chan songOrErr)
	num, err := readSongsFromJSONFile(test.WriteSongsToJSONFile(dir, songs), ch)
	if err != nil {
		t.Error("Failed reading songs from JSON: ", err)
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		t.Error("Failed getting songs from channel: ", err)
	}
	if err = test.CompareSongs(songs, actual, test.IgnoreOrder); err != nil {
		t.Error("Bad songs: ", err)
	}
}
