package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/derat/nup/cloudutil"
	"github.com/derat/nup/types"
)

const (
	batchSize = 100

	deletePath        = "delete_song"
	deleteSongIdParam = "songId"

	// Server path to import songs and query params.
	importPath         = "import"
	importReplaceParam = "replaceUserData=1"
)

func updateSongs(cfg Config, ch chan types.Song, numSongs int, replaceUserData bool) error {
	u, err := cloudutil.ServerURL(cfg.ServerUrl, importPath)
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
			return fmt.Errorf("Got non-OK status: %v", resp.Status)
		}
		return nil
	}

	// Ideally these results could just be streamed, but dev_appserver.py doesn't seem to support
	// chunked encoding: https://code.google.com/p/googleappengine/issues/detail?id=129
	// Might be for the best, as the max request duration could probably be hit otherwise.

	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	for i := 0; i < numSongs; i++ {
		if err = e.Encode(<-ch); err != nil {
			return fmt.Errorf("Failed to encode song: %v", err)
		}
		if (i+1)%batchSize == 0 {
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

func deleteSong(cfg Config, songId int64) error {
	u, err := cloudutil.ServerURL(cfg.ServerUrl, deletePath)
	if err != nil {
		return err
	}
	u.RawQuery = deleteSongIdParam + "=" + strconv.FormatInt(songId, 10)
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
		return fmt.Errorf("Got non-OK status: %v", resp.Status)
	}
	return nil
}
