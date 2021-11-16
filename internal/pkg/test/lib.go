// Copyright 2020 Daniel Erat.
// All rights reserved.

package test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/derat/nup/server/types"
)

func CreateTempDir() string {
	dir, err := ioutil.TempDir("", "nup_test.")
	if err != nil {
		panic(err)
	}
	return dir
}

// GetDataDir returns the test data dir relative to the caller.
func GetDataDir() string {
	_, p, _, ok := runtime.Caller(0)
	if !ok {
		panic("Unable to get runtime caller info")
	}
	return filepath.Join(filepath.Dir(p), "data")
}

// CopySongs copies the provided songs (e.g. Song0s.Filename) into dir.
// The supplied directory is created if it doesn't already exist.
func CopySongs(dir string, filenames ...string) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}

	for _, fn := range filenames {
		sp := filepath.Join(GetDataDir(), "music", fn)
		s, err := os.Open(sp)
		if err != nil {
			panic(err)
		}
		defer s.Close()

		dp := filepath.Join(dir, fn)
		d, err := os.Create(dp)
		if err != nil {
			panic(err)
		}
		defer d.Close()

		if _, err = io.Copy(d, s); err != nil {
			panic(err)
		}

		now := time.Now()
		if err = os.Chtimes(dp, now, now); err != nil {
			panic(err)
		}
	}
}

// DeleteSongs removes the provided songs (e.g. Song0s.Filename) from dir.
func DeleteSongs(dir string, filenames ...string) {
	for _, fn := range filenames {
		if err := os.Remove(filepath.Join(dir, fn)); err != nil {
			panic(err)
		}
	}
}

func WriteSongsToJSONFile(dir string, songs []types.Song) (path string) {
	f, err := ioutil.TempFile(dir, "songs-json.")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	e := json.NewEncoder(f)
	for _, s := range songs {
		if err = e.Encode(s); err != nil {
			panic(err)
		}
	}
	return f.Name()
}

type OrderPolicy int

const (
	CompareOrder OrderPolicy = iota
	IgnoreOrder
)

func CompareSongs(expected, actual []types.Song, order OrderPolicy) error {
	if order == IgnoreOrder {
		sort.Slice(expected, func(i, j int) bool { return expected[i].Filename < expected[j].Filename })
		sort.Slice(actual, func(i, j int) bool { return actual[i].Filename < actual[j].Filename })
	}

	m := make([]string, 0)

	stringify := func(s types.Song) string {
		if s.Plays != nil {
			for j := range s.Plays {
				s.Plays[j].StartTime = s.Plays[j].StartTime.UTC()
				// Ugly hack to handle IPv6 addresses.
				if s.Plays[j].IPAddress == "::1" {
					s.Plays[j].IPAddress = "127.0.0.1"
				}
			}
			sort.Sort(types.PlayArray(s.Plays))
		}
		b, err := json.Marshal(s)
		if err != nil {
			panic(err)
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

func GetSongsFromChannel(ch chan types.SongOrErr, num int) ([]types.Song, error) {
	songs := make([]types.Song, 0)
	for i := 0; i < num; i++ {
		s := <-ch
		if s.Err != nil {
			return nil, fmt.Errorf("got error at position %v instead of song: %v", i, s.Err)
		}
		songs = append(songs, *s.Song)
	}
	return songs, nil
}
