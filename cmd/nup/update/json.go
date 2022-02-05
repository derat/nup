// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"encoding/json"
	"io"
	"os"

	"github.com/derat/nup/server/db"
)

// readSongsFromJSONFile JSON-unmarshals db.Song objects from path and
// asynchronously sends them to ch. The total number of songs is returned.
func readSongsFromJSONFile(path string, ch chan songOrErr) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// This interface is dumb. We need to return the total number of songs,
	// but we also need to send the songs to the channel asynchronously to
	// avoid blocking. Reading all the songs into memory seems like the
	// best way to handle this. This function previously started a goroutine
	// for each song, but that results in songs getting sent in an arbitrary
	// order instead of the order in the file.
	var songs []db.Song
	d := json.NewDecoder(f)
	for {
		var s db.Song
		if err = d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return 0, err
		}
		songs = append(songs, s)
	}

	go func() {
		for i := range songs {
			ch <- songOrErr{&songs[i], nil}
		}
	}()
	return len(songs), nil
}
