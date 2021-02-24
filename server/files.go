// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/derat/nup/server/common"
	"github.com/derat/nup/server/storage"

	"google.golang.org/appengine/log"
)

const maxFileRangeSize = maxResponseSize - 32*1024 // save space for headers

// openSong opens the song at fn.
func openSong(ctx context.Context, fn string) (io.ReadCloser, error) {
	if cfg := common.Config(ctx); cfg.SongBucket != "" {
		return storage.NewObjectReader(ctx, cfg.SongBucket, fn)
	} else if cfg.SongBaseURL != "" {
		u := cfg.SongBaseURL + fn
		log.Debugf(ctx, "Opening %v", u)
		if resp, err := http.Get(u); err != nil {
			return nil, err
		} else {
			return resp.Body, nil
		}
	}
	return nil, errors.New("neither SongBucket nor SongBaseURL is set")
}

// sendObject copies data from r to w, handling range requests and setting any necesarry headers.
// If the request can't be satisfied writes an HTTP error to w.
func sendObject(ctx context.Context, req *http.Request, w http.ResponseWriter, r *storage.ObjectReader) error {
	// If the file fits within App Engine's limit, just use http.ServeContent,
	// which handles range requests and last-modified/conditional stuff.
	if r.Size <= maxFileRangeSize {
		log.Debugf(ctx, "Sending file of size %d", r.Size)
		http.ServeContent(w, req, filepath.Base(r.Name()), r.LastMod, r)
		return nil
	}

	// App Engine only permits responses up to 32 MB, so always send a partial response if the
	// requested range exceeds that: https://github.com/derat/nup/issues/10

	// TODO: I didn't see this called out in a skimming of https://tools.ietf.org/html/rfc7233, but
	// it's almost certainly bogus to send a 206 for a non-range request. I don't know what else we
	// can do, though. This implementation is also pretty broken (e.g. it doesn't look at
	// precondition fields).

	// Parse the Range header if one was supplied.
	rng := req.Header.Get("Range")
	start, end, ok := parseRangeHeader(rng)
	if !ok || start >= r.Size {
		// Fall back to using ServeContent for non-trivial requests. We may hit the 32 MB limit here.
		log.Debugf(ctx, "Unable to handle range %q for file of size %d", rng, r.Size)
		http.ServeContent(w, req, filepath.Base(r.Name()), r.LastMod, r)
		return nil
	}

	// Rewrite open-ended requests.
	if end == -1 {
		end = r.Size - 1
	}
	// If the requested range is too large, limit it to the max response size.
	if end-start+1 > maxFileRangeSize {
		end = start + maxFileRangeSize - 1
	}
	log.Debugf(ctx, "Sending bytes %d-%d/%d for requested range %q", start, end, r.Size, rng)

	if _, err := r.Seek(start, 0); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, r.Size))
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Last-Modified", r.LastMod.UTC().Format(time.RFC1123))
	w.WriteHeader(http.StatusPartialContent)

	_, err := io.CopyN(w, r, end-start+1)
	return err
}

var rangeRegexp = regexp.MustCompile(`^bytes=(\d+)-(\d+)?$`)

// parseRangeHeader parses an HTTP request Range header in the form "bytes=123-" or
// "bytes=123-456" and returns the inclusive start and ending offsets. The ending
// offset is -1 if it wasn't specified in the header, indicating the end of the file.
// Returns false if the header was empty, invalid, or doesn't match the above forms.
func parseRangeHeader(head string) (start, end int64, ok bool) {
	// If the header wasn't specified, they want the whole file.
	if head == "" {
		return 0, -1, true
	}

	// If the range is more complicated than a start with an optional end, give up.
	ms := rangeRegexp.FindStringSubmatch(head)
	if ms == nil {
		return 0, 0, false
	}

	// Otherwise, extract the offsets.
	start, err := strconv.ParseInt(ms[1], 10, 64)
	if start < 0 || err != nil {
		return 0, 0, false
	}
	if ms[2] == "" {
		end = -1 // end of file
	} else if end, err = strconv.ParseInt(ms[2], 10, 64); err != nil || end < start {
		return 0, 0, false
	}
	return start, end, true
}
