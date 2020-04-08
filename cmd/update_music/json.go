package main

import (
	"encoding/json"
	"io"
	"os"

	"github.com/derat/nup/types"
)

func getSongsFromJSONFile(path string, updateChan chan types.SongOrErr) (numUpdates int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	d := json.NewDecoder(f)
	for true {
		s := types.Song{}
		if err = d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return 0, err
		}
		go func() { updateChan <- types.SongOrErr{&s, nil} }()
		numUpdates += 1
	}

	return numUpdates, nil
}
