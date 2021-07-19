// Copyright 2020 Daniel Erat.
// All rights reserved.

package test

import (
	"time"

	"github.com/derat/nup/server/types"
)

var Song0s = types.Song{
	SHA1:        "fefac74a1d5928316d7131747107c8a61b71ffe4",
	Filename:    "0s.mp3",
	Artist:      "First Artist",
	Title:       "Zero Seconds",
	Album:       "First Album",
	AlbumID:     "1e477f68-c407-4eae-ad01-518528cedc2c",
	RecordingID: "392cea06-94c2-416b-80aa-f5b1e7d0fb1c",
	Track:       1,
	Disc:        0,
	Length:      0.026,
	Rating:      -1,
}

var Song0sUpdated = types.Song{
	SHA1:        "fefac74a1d5928316d7131747107c8a61b71ffe4",
	Filename:    "0s-updated.mp3",
	Artist:      "First Artist",
	Title:       "Zero Seconds (Remix)",
	Album:       "First Album",
	AlbumID:     "1e477f68-c407-4eae-ad01-518528cedc2c",
	RecordingID: "271a81af-6c2d-44cf-a0b8-a25ad74c82f9",
	Track:       1,
	Disc:        0,
	Length:      0.026,
	Rating:      -1,
}

var Song1s = types.Song{
	SHA1:        "c6e3230b4ed5e1f25d92dd6b80bfc98736bbee62",
	Filename:    "1s.mp3",
	Artist:      "Second Artist",
	Title:       "One Second",
	Album:       "First Album",
	AlbumID:     "1e477f68-c407-4eae-ad01-518528cedc2c",
	RecordingID: "5d7e41b2-ec4b-44dd-b25a-a576d7a08adb",
	Track:       2,
	Disc:        0,
	Length:      1.071,
	Rating:      -1,
}

var Song5s = types.Song{
	SHA1:     "63afdde2b390804562d54788865fff1bfd11cf94",
	Filename: "5s.mp3",
	Artist:   "Third Artist",
	Title:    "Five Seconds",
	Album:    "Another Album",
	AlbumID:  "a1d2405b-afe0-4e28-a935-b5b256f68131",
	Track:    1,
	Disc:     2,
	Length:   5.041,
	Rating:   -1,
}

var ID3V1Song = types.Song{
	SHA1:     "fefac74a1d5928316d7131747107c8a61b71ffe4",
	Filename: "id3v1.mp3",
	Artist:   "The Legacy Formats",
	Title:    "Give It Up For ID3v1",
	Album:    "UTF-8, Who Needs It?",
	Track:    0,
	Disc:     0,
	Length:   0.026,
	Rating:   -1,
}

var LegacySong1 = types.Song{
	SHA1:     "1977c91fea860245695dcceea0805c14cede7559",
	Filename: "arovane/atol_scrap/thaem_nue.mp3",
	Artist:   "Arovane",
	Title:    "Thaem Nue",
	Album:    "Atol Scrap",
	Track:    1,
	Disc:     1,
	Length:   449,
	Rating:   0.75,
	Plays: []types.Play{
		types.NewPlay(time.Unix(1276057170, 0).UTC(), "127.0.0.1"),
		types.NewPlay(time.Unix(1297316913, 0).UTC(), "1.2.3.4"),
	},
	Tags: []string{"electronic", "instrumental"},
}

var LegacySong2 = types.Song{
	SHA1:     "b70984a4ac5084999b70478cdf163218b90cefdb",
	Filename: "gary_hoey/animal_instinct/motown_fever.mp3",
	Artist:   "Gary Hoey",
	Title:    "Motown Fever",
	Album:    "Animal Instinct",
	Track:    7,
	Disc:     1,
	Length:   182,
	Rating:   0.5,
	Plays:    []types.Play{types.NewPlay(time.Unix(1394773930, 0).UTC(), "8.8.8.8")},
	Tags:     []string{"instrumental", "rock"},
}
