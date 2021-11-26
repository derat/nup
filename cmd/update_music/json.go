// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/derat/nup/server/db"
)

// readSongsFromJSONFile JSON-unmarshals db.Song objects from path and sends
// them to ch. The total number of sent songs is returned.
func readSongsFromJSONFile(path string, ch chan songOrErr) (numSongs int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	for {
		s := db.Song{}
		if err = d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return 0, err
		}
		go func() { ch <- songOrErr{&s, nil} }()
		numSongs++
	}
	return numSongs, nil
}
