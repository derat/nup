package main

import (
	"testing"
	"time"

	"erat.org/nup"
)

func TestLegacy(t *testing.T) {
	s0 := nup.Song{
		Sha1:     "1977c91fea860245695dcceea0805c14cede7559",
		Filename: "arovane/atol_scrap/thaem_nue.mp3",
		Artist:   "Arovane",
		Title:    "Thaem Nue",
		Album:    "Atol Scrap",
		Track:    3,
		Disc:     1,
		Length:   449,
		Rating:   0.75,
		Plays:    []nup.Play{{time.Unix(1276057170, 0), "127.0.0.1"}, {time.Unix(1297316913, 0), "1.2.3.4"}},
		Tags:     []string{"electronic", "instrumental"},
	}
	s1 := nup.Song{
		Sha1:     "b70984a4ac5084999b70478cdf163218b90cefdb",
		Filename: "gary_hoey/animal_instinct/motown_fever.mp3",
		Artist:   "Gary Hoey",
		Title:    "Motown Fever",
		Album:    "Animal Instinct",
		Track:    7,
		Disc:     1,
		Length:   182,
		Rating:   0.5,
		Plays:    []nup.Play{{time.Unix(1394773930, 0), "8.8.8.8"}},
		Tags:     []string{"instrumental", "rock"},
	}

	ch := make(chan SongAndError)
	num, err := getSongsFromLegacyDb("testdata/legacy.db", ch)
	if err != nil {
		t.Fatalf("getting songs failed: %v", err)
	}
	compareSongs(t, []nup.Song{s0, s1}, getSongsFromChannel(t, ch, num))
}
