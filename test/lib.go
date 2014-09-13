package test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"erat.org/nup"
)

var Song0s nup.Song = nup.Song{
	Sha1:     "fefac74a1d5928316d7131747107c8a61b71ffe4",
	Filename: "0s.mp3",
	Artist:   "First Artist",
	Title:    "Zero Seconds",
	Album:    "First Album",
	Track:    1,
	Disc:     0,
	Length:   0.026,
}
var Song0sUpdated nup.Song = nup.Song{
	Sha1:     "fefac74a1d5928316d7131747107c8a61b71ffe4",
	Filename: "0s-updated.mp3",
	Artist:   "First Artist",
	Title:    "Zero Seconds (Remix)",
	Album:    "First Album",
	Track:    1,
	Disc:     0,
	Length:   0.026,
}
var Song1s nup.Song = nup.Song{
	Sha1:     "c6e3230b4ed5e1f25d92dd6b80bfc98736bbee62",
	Filename: "1s.mp3",
	Artist:   "Second Artist",
	Title:    "One Second",
	Album:    "First Album",
	Track:    2,
	Disc:     0,
	Length:   1.071,
}
var Song5s nup.Song = nup.Song{
	Sha1:     "63afdde2b390804562d54788865fff1bfd11cf94",
	Filename: "5s.mp3",
	Artist:   "Third Artist",
	Title:    "Five Seconds",
	Album:    "Another Album",
	Track:    1,
	Disc:     2,
	Length:   5.041,
}

func CreateTempDir() string {
	dir, err := ioutil.TempDir("", "update_music.")
	if err != nil {
		panic(err)
	}
	return dir
}

func CopySongsToTempDir(dir string, filenames ...string) {
	_, td, _, ok := runtime.Caller(0)
	if !ok {
		panic("unable to get runtime caller info")
	}

	for _, fn := range filenames {
		sp := filepath.Join(td, "../data/music", fn)
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

func CompareSongs(expected, actual []nup.Song) error {
	m := make([]string, 0)

	for i := 0; i < len(expected); i++ {
		if i >= len(actual) {
			m = append(m, fmt.Sprintf("missing song at position %v; expected %q", i, expected[i].Filename))
		} else {
			e, err := json.Marshal(expected[i])
			if err != nil {
				panic(err)
			}
			a, err := json.Marshal(actual[i])
			if err != nil {
				panic(err)
			}
			if string(a) != string(e) {
				m = append(m, fmt.Sprintf("song %v didn't match expected values:\nexpected: %v\n  actual: %v", i, string(e), string(a)))
			}
		}
	}
	for i := len(expected); i < len(actual); i++ {
		m = append(m, fmt.Sprintf("got unexpected song %q at position %v", actual[i].Filename, i))
	}

	if len(m) > 0 {
		return fmt.Errorf("actual songs didn't match expected: %v", strings.Join(m, "\n"))
	}
	return nil
}
