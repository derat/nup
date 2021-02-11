// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/derat/nup/internal/pkg/cloudutil"
	"github.com/derat/nup/internal/pkg/types"
)

const (
	batchSize = 100 // updateSongs HTTP request batch size

	deletePath        = "delete_song" // server path to delete songs
	deleteSongIDParam = "songId"      // query param

	importPath         = "import"            // server path to import songs
	importReplaceParam = "replaceUserData=1" // query param
)

// updateSongs reads all songs from ch and sends them to the server.
//
// If replaceUserData is true, then user data (e.g. rating, tags, plays)
// are replaced with data from ch; otherwise the user data on the server
// are preserved and only static fields (e.g. artist, title, album, etc.)
// are replaced.
func updateSongs(cfg config, ch chan types.Song, replaceUserData bool) error {
	u, err := cloudutil.ServerURL(cfg.ServerURL, importPath)
	if err != nil {
		return err
	}
	if replaceUserData {
		u.RawQuery = importReplaceParam
	}

	client := http.Client{}

	sendFunc := func(r io.Reader) error {
		req, err := http.NewRequest("POST", u.String(), r)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "text/plain")
		req.SetBasicAuth(cfg.Username, cfg.Password)

		resp, err := client.Do(req)
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
		if err = e.Encode(s); err != nil {
			return fmt.Errorf("failed to encode song: %v", err)
		}
		if numSongs%batchSize == 0 {
			if err = sendFunc(&buf); err != nil {
				return err
			}
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		if err = sendFunc(&buf); err != nil {
			return err
		}
	}
	return nil
}

// deleteSong sends a request to the server to delete the song with the specified ID.
func deleteSong(cfg config, songID int64) error {
	u, err := cloudutil.ServerURL(cfg.ServerURL, deletePath)
	if err != nil {
		return err
	}
	u.RawQuery = deleteSongIDParam + "=" + strconv.FormatInt(songID, 10)
	req, err := http.NewRequest("POST", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	req.SetBasicAuth(cfg.Username, cfg.Password)

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got status %q", resp.Status)
	}
	return nil
}
