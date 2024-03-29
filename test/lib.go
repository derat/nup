// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package test contains common functionality and data used by tests.
package test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/derat/nup/server/db"
)

// Must aborts t if err is non-nil.
func Must(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Failed at %v: %v", Caller(), err)
	}
}

// SongsDir returns the test/data/songs directory containing sample song files.
func SongsDir() (string, error) {
	libDir, err := CallerDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(libDir, "data/songs"), nil
}

// CopySongs copies the provided songs (e.g. Song0s.Filename) from SongsDir into dir.
// The supplied directory is created if it doesn't already exist.
func CopySongs(dir string, filenames ...string) error {
	srcDir, err := SongsDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	for _, fn := range filenames {
		sp := filepath.Join(srcDir, fn)
		s, err := os.Open(sp)
		if err != nil {
			return err
		}
		defer s.Close()

		dp := filepath.Join(dir, fn)
		d, err := os.Create(dp)
		if err != nil {
			return err
		}
		if _, err := io.Copy(d, s); err != nil {
			d.Close()
			return err
		}
		if err := d.Close(); err != nil {
			return err
		}

		now := time.Now()
		if err := os.Chtimes(dp, now, now); err != nil {
			return err
		}
	}
	return nil
}

// DeleteSongs removes the provided songs (e.g. Song0s.Filename) from dir.
func DeleteSongs(dir string, filenames ...string) error {
	for _, fn := range filenames {
		if err := os.Remove(filepath.Join(dir, fn)); err != nil {
			return err
		}
	}
	return nil
}

// WriteSongsToJSONFile creates a file in dir containing JSON-marshaled songs.
// The file's path is returned.
func WriteSongsToJSONFile(dir string, songs ...db.Song) (string, error) {
	f, err := ioutil.TempFile(dir, "songs-json.")
	if err != nil {
		return "", err
	}
	e := json.NewEncoder(f)
	for _, s := range songs {
		if err = e.Encode(s); err != nil {
			f.Close()
			return "", err
		}
	}
	return f.Name(), f.Close()
}

// WriteSongPathsFile creates a file in dir listing filenames,
// suitable for passing to the `nup update -song-paths-file` flag.
func WriteSongPathsFile(dir string, filenames ...string) (string, error) {
	f, err := ioutil.TempFile(dir, "song-list.")
	if err != nil {
		return "", err
	}
	for _, fn := range filenames {
		if _, err := f.WriteString(fn + "\n"); err != nil {
			f.Close()
			return "", err
		}
	}
	return f.Name(), f.Close()
}

// OrderPolicy specifies whether CompareSongs requires that songs appear in the specified order.
type OrderPolicy int

const (
	CompareOrder OrderPolicy = iota
	IgnoreOrder
)

// CompareSongs compares expected against actual.
// A descriptive error is returned if the songs don't match.
// TODO: Returning a multi-line error seems dumb.
func CompareSongs(expected, actual []db.Song, order OrderPolicy) error {
	if order == IgnoreOrder {
		sort.Slice(expected, func(i, j int) bool { return expected[i].Filename < expected[j].Filename })
		sort.Slice(actual, func(i, j int) bool { return actual[i].Filename < actual[j].Filename })
	}

	m := make([]string, 0)

	stringify := func(s db.Song) string {
		if s.Plays != nil {
			for j := range s.Plays {
				s.Plays[j].StartTime = s.Plays[j].StartTime.UTC()
				// Ugly hack to handle IPv6 addresses.
				if s.Plays[j].IPAddress == "::1" {
					s.Plays[j].IPAddress = "127.0.0.1"
				}
			}
			sort.Sort(db.PlayArray(s.Plays))
		}
		b, err := json.Marshal(s)
		if err != nil {
			return "failed: " + err.Error()
		}
		return string(b)
	}

	for i := 0; i < len(expected); i++ {
		if i >= len(actual) {
			m = append(m, fmt.Sprintf("missing song at position %v; expected %v", i, stringify(expected[i])))
		} else {
			a := stringify(actual[i])
			e := stringify(expected[i])
			if a != e {
				m = append(m, fmt.Sprintf("song %v didn't match expected values:\nexpected: %v\n  actual: %v", i, e, a))
			}
		}
	}
	for i := len(expected); i < len(actual); i++ {
		m = append(m, fmt.Sprintf("unexpected song at position %v: %v", i, stringify(actual[i])))
	}

	if len(m) > 0 {
		return fmt.Errorf("actual songs didn't match expected:\n%v", strings.Join(m, "\n"))
	}
	return nil
}

// Date is a convenience wrapper around time.Date that constructs a time.Time in UTC.
// Hour, minute, second, and nanosecond values are taken from tm if present.
func Date(year int, month time.Month, day int, tm ...int) time.Time {
	get := func(idx int) int {
		if idx < len(tm) {
			return tm[idx]
		}
		return 0
	}
	return time.Date(year, month, day, get(0), get(1), get(2), get(3), time.UTC)
}
