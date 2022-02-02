// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
)

const batchSize = 100 // updateSongs HTTP request batch size

// sendRequest sends the specified request to the server and returns the response body.
// r contains the request body and may be nil.
// ctype contains the value for the Content-Type header if non-empty.
func sendRequest(cfg *client.Config, method, path, query string,
	r io.Reader, ctype string) ([]byte, error) {
	u := cfg.GetURL(path)
	if query != "" {
		u.RawQuery = query
	}
	req, err := http.NewRequest(method, u.String(), r)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return b, err
	}
	if resp.StatusCode != http.StatusOK {
		return b, fmt.Errorf("got status %q", resp.Status)
	}
	return b, nil
}

// updateSongs reads all songs from ch and sends them to the server.
//
// If replaceUserData is true, then user data (e.g. rating, tags, plays)
// are replaced with data from ch; otherwise the user data on the server
// are preserved and only static fields (e.g. artist, title, album, etc.)
// are replaced.
func updateSongs(cfg *client.Config, ch chan db.Song, replaceUserData bool) error {
	var query string
	if replaceUserData {
		query = "replaceUserData=1"
	}
	sendFunc := func(r io.Reader) error {
		_, err := sendRequest(cfg, "POST", "/import", query, r, "text/plain")
		return err
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

// dumpSong dumps the song with the specified ID from the server.
// User data like ratings, tags, and plays are included.
func dumpSong(cfg *client.Config, songID int64) (db.Song, error) {
	b, err := sendRequest(cfg, "GET", "/dump_song", fmt.Sprintf("songId=%v", songID), nil, "")
	if err != nil {
		return db.Song{}, err
	}
	var s db.Song
	err = json.Unmarshal(b, &s)
	return s, err
}

// reindexSongs asks the server to reindex all songs' search data.
func reindexSongs(cfg *client.Config) error {
	var cursor string
	var scanned, updated int // totals
	for {
		var res struct {
			Scanned int    `json:"scanned"`
			Updated int    `json:"updated"`
			Cursor  string `json:"cursor"`
		}
		query := "cursor=" + url.QueryEscape(cursor)
		if b, err := sendRequest(cfg, "POST", "/reindex", query, nil, ""); err != nil {
			return err
		} else if err := json.Unmarshal(b, &res); err != nil {
			return err
		}
		scanned += res.Scanned
		updated += res.Updated
		log.Printf("Scanned %v songs, updated %v", scanned, updated)
		if cursor = res.Cursor; cursor == "" {
			return nil
		}
	}
}
