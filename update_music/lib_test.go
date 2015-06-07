package main

import (
	"fmt"

	"erat.org/nup"
)

func getSongsFromChannel(ch chan SongOrErr, num int) ([]nup.Song, error) {
	songs := make([]nup.Song, 0)
	for i := 0; i < num; i++ {
		s := <-ch
		if s.Err != nil {
			return nil, fmt.Errorf("got error at position %v instead of song: %v", i, s.Err)
		}
		songs = append(songs, *s.Song)
	}
	return songs, nil
}
