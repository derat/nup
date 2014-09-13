package main

import (
	"fmt"

	"erat.org/nup"
)

func getSongsFromChannel(ch chan SongAndError, num int) ([]nup.Song, error) {
	songs := make([]nup.Song, 0)
	for i := 0; i < num; i++ {
		sae := <-ch
		if sae.Error != nil {
			return nil, fmt.Errorf("got error at position %v instead of song: %v", i, sae.Error)
		}
		songs = append(songs, *sae.Song)
	}
	return songs, nil
}
