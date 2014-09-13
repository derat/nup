package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"erat.org/nup"
)

const (
	batchSize = 100

	// Server path to import songs and query params.
	importPath         = "import"
	importReplaceParam = "replaceUserData=1"
)

func updateSongs(client *http.Client, cfg Config, ch chan nup.Song, numSongs int, replaceUserData bool) error {
	u, err := nup.GetServerUrl(cfg.ClientConfig, importPath)
	if err != nil {
		return err
	}
	if replaceUserData {
		u.RawQuery = importReplaceParam
	}

	sendFunc := func(r io.Reader) error {
		resp, err := client.Post(u.String(), "text/plain", r)
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
