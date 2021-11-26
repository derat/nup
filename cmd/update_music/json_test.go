// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
)

func TestJSON(t *testing.T) {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	songs := []db.Song{test.LegacySong1, test.LegacySong2}
	ch := make(chan songOrErr)
	num, err := readSongsFromJSONFile(test.WriteSongsToJSONFile(dir, songs), ch)
	if err != nil {
		t.Error(err)
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		t.Error(err)
	}
	if err = test.CompareSongs(songs, actual, test.IgnoreOrder); err != nil {
		t.Error(err)
	}
}
