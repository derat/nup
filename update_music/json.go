package main

import (
	"encoding/json"
	"io"
	"os"

	"erat.org/nup"
)

func getSongsFromJsonFile(path string, updateChan chan nup.SongOrErr) (numUpdates int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	for true {
		s := nup.Song{}
		if err = d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return 0, err
		}
		go func() { updateChan <- nup.SongOrErr{&s, nil} }()
		numUpdates += 1
	}

	return numUpdates, nil
}
