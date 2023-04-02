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
	"strings"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
)

const (
	tlsTimeout       = time.Minute
	importBatchSize  = 50 // max songs to import per HTTP request
	importTries      = 3
	importRetryDelay = 3 * time.Second
)

// I started seeing "net/http: TLS handshake timeout" errors when trying to import songs.
// I'm not sure if this is just App Engine flakiness or something else, but I didn't see
// the error again after increasing the timeout.
var httpClient = &http.Client{
	Transport: &http.Transport{TLSHandshakeTimeout: tlsTimeout},
}

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

	resp, err := httpClient.Do(req)
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

// importSongsFlag values can be masked together to configure importSongs's behavior.
type importSongsFlag uint32

const (
	// importReplaceUserData indicates that user data (e.g. rating, tags, plays) should be
	// replaced with data from ch; otherwise the existing data is preserved and only static
	// fields (e.g. artist, title, album, etc.) are replaced.
	importReplaceUserData importSongsFlag = 1 << iota
	// importUseFilenames indicates that the server should identify songs to import by their
	// filenames rather than by SHA1s of their audio data. This can be used to avoid creating
	// a new database object after deliberately modifying a song file's audio data.
	importUseFilenames
	// importNoRetryDelay indicates that importSongs should not sleep after a failed HTTP
	// request. This is just useful for unit tests.
	importNoRetryDelay
)

// importSongs reads all songs from ch and sends them to the server.
func importSongs(cfg *client.Config, ch chan db.Song, flags importSongsFlag) error {
	var args []string
	if flags&importReplaceUserData != 0 {
		args = append(args, "replaceUserData=1")
	}
	if flags&importUseFilenames != 0 {
		args = append(args, "useFilenames=1")
	}
	query := strings.Join(args, "&")

	sendFunc := func(body []byte) error {
		var err error
		for try := 1; try <= importTries; try++ {
			r := bytes.NewReader(body)
			if _, err = sendRequest(cfg, "POST", "/import", query, r, "text/plain"); err == nil {
				break
			} else if try < importTries {
				delay := importRetryDelay
				if flags&importNoRetryDelay != 0 {
					delay = 0
				}
				log.Printf("Sleeping %v before retrying after error: %v", delay, err)
				time.Sleep(delay)
			}
		}
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
		if numSongs%importBatchSize == 0 {
			// Pass the underlying bytes rather than an io.Reader so sendFunc() can re-read the
			// data if it needs to retry due to network issues or App Engine flakiness.
			if err := sendFunc(buf.Bytes()); err != nil {
				return err
			}
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		if err := sendFunc(buf.Bytes()); err != nil {
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

// deleteSong deletes the song with the specified ID from the server.
func deleteSong(cfg *client.Config, songID int64) error {
	params := fmt.Sprintf("songId=%v", songID)
	_, err := sendRequest(cfg, "POST", "/delete_song", params, nil, "text/plain")
	return err
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
