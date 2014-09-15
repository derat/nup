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

var Song0s nup.Song = nup.Song{
	Sha1:     "fefac74a1d5928316d7131747107c8a61b71ffe4",
	Filename: "0s.mp3",
	Artist:   "First Artist",
	Title:    "Zero Seconds",
	Album:    "First Album",
	Track:    1,
	Disc:     0,
	Length:   0.026,
	Rating:   -1,
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
	Rating:   -1,
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
	Rating:   -1,
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
	Rating:   -1,
}
var LegacySong1 nup.Song = nup.Song{
	Sha1:     "1977c91fea860245695dcceea0805c14cede7559",
	Filename: "arovane/atol_scrap/thaem_nue.mp3",
	Artist:   "Arovane",
	Title:    "Thaem Nue",
	Album:    "Atol Scrap",
	Track:    3,
	Disc:     1,
	Length:   449,
	Rating:   0.75,
	Plays:    []nup.Play{{time.Unix(1276057170, 0).UTC(), "127.0.0.1"}, {time.Unix(1297316913, 0).UTC(), "1.2.3.4"}},
	Tags:     []string{"electronic", "instrumental"},
}
var LegacySong2 nup.Song = nup.Song{
	Sha1:     "b70984a4ac5084999b70478cdf163218b90cefdb",
	Filename: "gary_hoey/animal_instinct/motown_fever.mp3",
	Artist:   "Gary Hoey",
	Title:    "Motown Fever",
	Album:    "Animal Instinct",
	Track:    7,
	Disc:     1,
	Length:   182,
	Rating:   0.5,
	Plays:    []nup.Play{{time.Unix(1394773930, 0).UTC(), "8.8.8.8"}},
	Tags:     []string{"instrumental", "rock"},
}

func CreateTempDir() string {
	dir, err := ioutil.TempDir("", "nup_test.")
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

type songArray []nup.Song

func (a songArray) Len() int           { return len(a) }
func (a songArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a songArray) Less(i, j int) bool { return a[i].Filename < a[j].Filename }

type playArray []nup.Play

func (a playArray) Len() int           { return len(a) }
func (a playArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a playArray) Less(i, j int) bool { return a[i].StartTime.Before(a[j].StartTime) }

func CompareSongs(expected, actual []nup.Song, compareOrder bool) error {
	if !compareOrder {
		sort.Sort(songArray(expected))
		sort.Sort(songArray(actual))
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
					sort.Sort(playArray(s.Plays))
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
