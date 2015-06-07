package main

import (
	"path/filepath"
	"testing"

	"erat.org/nup"
	"erat.org/nup/test"
)

func TestJson(t *testing.T) {
	ch := make(chan SongOrErr)
	num, err := getSongsFromJsonFile(filepath.Join(test.GetDataDir(), "import.json"), ch)
	if err != nil {
		t.Error(err)
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		t.Error(err)
	}
	if err = test.CompareSongs([]nup.Song{test.LegacySong1, test.LegacySong2}, actual, test.CompareOrder); err != nil {
		t.Error(err)
	}
}
