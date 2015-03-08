package main

import (
	"path/filepath"
	"testing"

	"erat.org/nup"
	"erat.org/nup/test"
)

func testLegacyQuery(expected []nup.Song, minId int64) error {
	ch := make(chan SongAndError)
	num, err := getSongsFromLegacyDb(filepath.Join(test.GetDataDir(), "legacy.db"), minId, ch)
	if err != nil {
		return err
	}
	actual, err := getSongsFromChannel(ch, num)
	if err != nil {
		return err
	}
	return test.CompareSongs(expected, actual, test.CompareOrder)
}

func TestLegacy(t *testing.T) {
	for _, tc := range []struct {
		MinId         int64
		ExpectedSongs []nup.Song
	}{
		{0, []nup.Song{test.LegacySong1, test.LegacySong2}},
		{1, []nup.Song{test.LegacySong1, test.LegacySong2}},
		{2, []nup.Song{test.LegacySong2}},
		{3, []nup.Song{}},
	} {
		if err := testLegacyQuery(tc.ExpectedSongs, tc.MinId); err != nil {
			t.Errorf("Min ID %v: %v", tc.MinId, err)
		}
	}
}
