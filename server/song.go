// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/derat/nup/server/config"
	"github.com/derat/nup/server/storage"

	"google.golang.org/appengine/v2/log"
)

const (
	// openSong saves song data in-memory if it's this many bytes or smaller.
	// Per https://cloud.google.com/appengine/docs/standard, F1 second-gen
	// runtimes have a 256 MB memory limit.
	maxSongMemSize = 32 * 1024 * 1024

	// Maximum range request size to service in a single response.
	// Per https://cloud.google.com/appengine/docs/standard/go/how-requests-are-handled,
	// App Engine permits 32 MB responses, but we need to reserve a bit of extra space
	// to make sure we don't go over the limit with headers.
	maxFileRangeSize = 32*1024*1024 - 32*1024
)

var (
	// These correspond to the Cloud Storage object that was last accessed via openSong.
	// Chrome can send multiple requests for a single file, so holding song data in memory lets us
	// avoid reading the same bytes from GCS multiple times. Returning stale objects hopefully isn't
	// a concern, since clients will probably already have a bad time if a song changes while
	// they're in the process of playing it.
	lastSongName    string     // name of object in lastSongData
	lastSongData    []byte     // contents of last object from openSong
	lastSongModTime time.Time  // object's last-modified time
	lastSongMutex   sync.Mutex // guards other lastSong variables
)

// getSongData atomically returns lastSongData and lastSongModTime if lastSongName matches name.
// nil and a zero time are returned if the names don't match.
func getSongData(name string) ([]byte, time.Time) {
	lastSongMutex.Lock()
	defer lastSongMutex.Unlock()
	if name == lastSongName {
		return lastSongData, lastSongModTime
	}
	return nil, time.Time{}
}

// setSongData atomically updates lastSongName, lastSongData, and lastSongModTime.
func setSongData(name string, data []byte, lastMod time.Time) {
	lastSongMutex.Lock()
	lastSongName = name
	lastSongData = data
	lastSongModTime = lastMod
	lastSongMutex.Unlock()
}

// songReader implements io.ReadSeekCloser along with methods needed by sendSong.
type songReader interface {
	Read(b []byte) (n int, err error)
	Seek(offset int64, whence int) (int64, error)
	Close() error
	Name() string
	LastMod() time.Time
	Size() int64
}

// byteSongReader implements songReader for a byte slice.
type bytesSongReader struct {
	r       *bytes.Reader
	name    string
	lastMod time.Time
}

func newBytesSongReader(b []byte, name string, lastMod time.Time) *bytesSongReader {
	return &bytesSongReader{bytes.NewReader(b), name, lastMod}
}
func (br *bytesSongReader) Read(b []byte) (int, error) { return br.r.Read(b) }
func (br *bytesSongReader) Seek(offset int64, whence int) (int64, error) {
	return br.r.Seek(offset, whence)
}
func (br *bytesSongReader) Close() error       { return nil }
func (br *bytesSongReader) Name() string       { return br.name }
func (br *bytesSongReader) LastMod() time.Time { return br.lastMod }
func (br *bytesSongReader) Size() int64        { return br.r.Size() }

var _ songReader = (*bytesSongReader)(nil) // verify that interface is implemented

// openSong opens the song at fn (using either Cloud Storage or HTTP).
// The returned reader will also implement songReader when reading from Cloud Storage
// or serving an in-memory song that was previously read from Cloud Storage.
// os.ErrNotExist is returned if the file is not present.
func openSong(ctx context.Context, cfg *config.Config, fn string) (io.ReadCloser, error) {
	switch {
	case cfg.SongBucket != "":
		// If we already have the song in memory, return it.
		if b, t := getSongData(fn); b != nil {
			log.Debugf(ctx, "Using in-memory copy of %q", fn)
			return newBytesSongReader(b, fn, t), nil
		}
		or, err := storage.NewObjectReader(ctx, cfg.SongBucket, fn)
		if err != nil {
			return nil, err
		} else if or.Size() > maxSongMemSize {
			return or, nil // too big to load into memory
		}
		log.Debugf(ctx, "Reading %q into memory", fn)
		defer or.Close()
		setSongData("", nil, time.Time{}) // clear old buffer
		b := make([]byte, or.Size())
		if _, err := io.ReadFull(or, b); err != nil {
			return nil, err
		}
		setSongData(fn, b, or.LastMod())
		return newBytesSongReader(b, fn, or.LastMod()), nil
	case cfg.SongBaseURL != "":
		u := cfg.SongBaseURL + fn
		log.Debugf(ctx, "Opening %v", u)
		if resp, err := http.Get(u); err != nil {
			return nil, err
		} else if resp.StatusCode >= 300 {
			resp.Body.Close()
			if resp.StatusCode == 404 {
				return nil, os.ErrNotExist
			}
			return nil, fmt.Errorf("server replied with %q", resp.Status)
		} else {
			return resp.Body, nil
		}
	default:
		return nil, errors.New("neither SongBucket nor SongBaseURL is set")
	}
}

// sendSong copies data from r to w, handling range requests and setting any necessary headers.
// If the request can't be satisfied, writes an HTTP error to w.
func sendSong(ctx context.Context, req *http.Request, w http.ResponseWriter, r songReader) error {
	// If the file fits within App Engine's limit, just use http.ServeContent,
	// which handles range requests and last-modified/conditional stuff.
	size := r.Size()
	if size <= maxFileRangeSize {
		var rng string
		if v := req.Header.Get("Range"); v != "" {
			rng = " (" + v + ")"
		}
		log.Debugf(ctx, "Sending file of size %d%v", size, rng)
		http.ServeContent(w, req, filepath.Base(r.Name()), r.LastMod(), r)
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
	if !ok || start >= size {
		// Fall back to using ServeContent for non-trivial requests. We may hit the 32 MB limit here.
		log.Debugf(ctx, "Unable to handle range %q for file of size %d", rng, size)
		http.ServeContent(w, req, filepath.Base(r.Name()), r.LastMod(), r)
		return nil
	}

	// Rewrite open-ended requests.
	if end == -1 {
		end = size - 1
	}
	// If the requested range is too large, limit it to the max response size.
	if end-start+1 > maxFileRangeSize {
		end = start + maxFileRangeSize - 1
	}
	log.Debugf(ctx, "Sending bytes %d-%d/%d for requested range %q", start, end, size, rng)

	if _, err := r.Seek(start, 0); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Last-Modified", r.LastMod().UTC().Format(time.RFC1123))
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
