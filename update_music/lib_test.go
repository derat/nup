package main

import (
	"testing"

	"erat.org/nup"
)

func getSongsFromChannel(t *testing.T, ch chan SongAndError, num int) []nup.Song {
	songs := make([]nup.Song, 0)
	for i := 0; i < num; i++ {
		sae := <-ch
		if sae.Error != nil {
			t.Errorf("got error at position %v instead of song: %v", i, sae.Error)
		}
		songs = append(songs, *sae.Song)
	}
	return songs
}
