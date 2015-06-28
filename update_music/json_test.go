package main

import (
	"io/ioutil"
	"os"
	"testing"

	"erat.org/nup"
	"erat.org/nup/test"
)

func TestJson(t *testing.T) {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	songs := []nup.Song{test.LegacySong1, test.LegacySong2}
	ch := make(chan SongOrErr)
	num, err := getSongsFromJsonFile(test.WriteSongsToJsonFile(dir, songs), ch)
	if err != nil {
		t.Error(err)
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		t.Error(err)
	}
	if err = test.CompareSongs(songs, actual, test.CompareOrder); err != nil {
		t.Error(err)
	}
}
