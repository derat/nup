package main

import (
	"testing"
	"time"

	"erat.org/nup"
	"erat.org/nup/test"
)

func TestLegacy(t *testing.T) {
	ch := make(chan SongAndError)
	num, err := getSongsFromLegacyDb("../test/data/legacy.db", ch)
	if err != nil {
		t.Fatalf("getting songs failed: %v", err)
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		t.Fatal(err)
	}
	if err = test.CompareSongs([]nup.Song{test.LegacySong1, test.LegacySong2}, actual, true); err != nil {
		t.Error(err)
	}
}
