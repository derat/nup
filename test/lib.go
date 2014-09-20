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

	"erat.org/nup"
)

const (
	TestUsername = "testuser"
	TestPassword = "testpass"
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
		panic("unable to get runtime caller info")
	}
	return filepath.Join(filepath.Dir(p), "data")
}

func CopySongsToTempDir(dir string, filenames ...string) {
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

func RemoveFromTempDir(dir string, filenames ...string) {
	for _, fn := range filenames {
		if err := os.Remove(filepath.Join(dir, fn)); err != nil {
			panic(err)
		}
	}
}

type SongArray []nup.Song

func (a SongArray) Len() int           { return len(a) }
func (a SongArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SongArray) Less(i, j int) bool { return a[i].Filename < a[j].Filename }

type PlayArray []nup.Play

func (a PlayArray) Len() int           { return len(a) }
func (a PlayArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a PlayArray) Less(i, j int) bool { return a[i].StartTime.Before(a[j].StartTime) }

func CompareSongs(expected, actual []nup.Song, compareOrder bool) error {
	if !compareOrder {
		sort.Sort(SongArray(expected))
		sort.Sort(SongArray(actual))
	}

	m := make([]string, 0)

	for i := 0; i < len(expected); i++ {
		if i >= len(actual) {
			m = append(m, fmt.Sprintf("missing song at position %v; expected %q", i, expected[i].Filename))
		} else {
			stringify := func(s nup.Song) string {
				if s.Plays != nil {
					for j := range s.Plays {
						s.Plays[j].StartTime = s.Plays[j].StartTime.UTC()
					}
					sort.Sort(PlayArray(s.Plays))
				}
				b, err := json.Marshal(s)
				if err != nil {
					panic(err)
				}
				return string(b)
			}
			a := stringify(actual[i])
			e := stringify(expected[i])
			if a != e {
				m = append(m, fmt.Sprintf("song %v didn't match expected values:\nexpected: %v\n  actual: %v", i, e, a))
			}
		}
	}
	for i := len(expected); i < len(actual); i++ {
		m = append(m, fmt.Sprintf("got unexpected song %q at position %v", actual[i].Filename, i))
	}

	if len(m) > 0 {
		return fmt.Errorf("actual songs didn't match expected:\n%v", strings.Join(m, "\n"))
	}
	return nil
}
