package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"erat.org/cloud"
	"erat.org/nup"
)

const (
	oauthScope = "https://www.googleapis.com/auth/userinfo.email"

	batchSize = 100

	// Server path to import songs and query params.
	importPath         = "import"
	importReplaceParam = "replaceUserData=1"
)

func getLastUpdateTime() (time.Time, error) {
	return time.Now(), nil
}

func setLastUpdateTime(t time.Time) error {
	return nil
}

func updateSongs(cfg nup.ClientConfig, ch chan nup.Song, numSongs int, replaceUserData bool) error {
	transport, err := cloud.CreateTransport(cfg.ClientId, cfg.ClientSecret, oauthScope, cfg.TokenCache)
	if err != nil {
		return err
	}
	if err = cloud.MaybeRefreshToken(transport); err != nil {
		return err
	}

	u, err := nup.GetServerUrl(cfg, importPath)
	if err != nil {
		return err
	}
	if replaceUserData {
		u.RawQuery = importReplaceParam
	}

	sendFunc := func(r io.Reader) error {
		resp, err := transport.Client().Post(u.String(), "text/plain", r)
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
