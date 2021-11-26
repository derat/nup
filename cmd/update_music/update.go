// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/derat/nup/types"
)

const batchSize = 100 // updateSongs HTTP request batch size

// updateSongs reads all songs from ch and sends them to the server.
//
// If replaceUserData is true, then user data (e.g. rating, tags, plays)
// are replaced with data from ch; otherwise the user data on the server
// are preserved and only static fields (e.g. artist, title, album, etc.)
// are replaced.
func updateSongs(cfg *config, ch chan types.Song, replaceUserData bool) error {
	u := cfg.GetURL("/import")
	if replaceUserData {
		u.RawQuery = "replaceUserData=1"
	}

	sendFunc := func(r io.Reader) error {
		req, err := http.NewRequest("POST", u.String(), r)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "text/plain")
		req.SetBasicAuth(cfg.Username, cfg.Password)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("got status %q", resp.Status)
		}
		return nil
	}

	// Ideally these results could just be streamed, but dev_appserver.py doesn't seem to support
	// chunked encoding: https://code.google.com/p/googleappengine/issues/detail?id=129
	// Might be for the best, as the max request duration could probably be hit otherwise.

	var numSongs int
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	for s := range ch {
		numSongs++
		if err := e.Encode(s); err != nil {
			return fmt.Errorf("failed to encode song: %v", err)
		}
		if numSongs%batchSize == 0 {
			if err := sendFunc(&buf); err != nil {
				return err
			}
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		if err := sendFunc(&buf); err != nil {
			return err
		}
	}
	return nil
}

// deleteSong sends a request to the server to delete the song with the specified ID.
func deleteSong(cfg *config, songID int64) error {
	u := cfg.GetURL("/delete_song")
	u.RawQuery = fmt.Sprintf("songId=%v", songID)
	req, err := http.NewRequest("POST", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.SetBasicAuth(cfg.Username, cfg.Password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got status %q", resp.Status)
	}
	return nil
}
