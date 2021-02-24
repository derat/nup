// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/derat/nup/internal/pkg/types"
)

// readSongsFromJSONFile JSON-unmarshals types.Song objects from path and sends
// them to ch. The total number of sent songs is returned.
func readSongsFromJSONFile(path string, ch chan types.SongOrErr) (numSongs int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	for {
		s := types.Song{}
		if err = d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return 0, err
		}
		go func() { ch <- types.NewSongOrErr(&s, nil) }()
		numSongs++
	}
	return numSongs, nil
}
