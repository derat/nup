// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/derat/nup/types"
)

type checkSettings uint32

const (
	checkAlbumId checkSettings = 1 << iota
	checkCoverSize400
	checkSongCover
)

func checkSongs(songs []*types.Song, musicDir, coverDir string, settings checkSettings) {
	fs := [](func(s *types.Song) error){
		func(s *types.Song) error {
			if len(s.Filename) == 0 {
				return errors.New("no song filename")
			} else if _, err := os.Stat(filepath.Join(musicDir, s.Filename)); err != nil {
				return errors.New("missing song file")
			}
			return nil
		},
	}
	if settings&checkAlbumId != 0 {
		fs = append(fs, func(s *types.Song) error {
			if len(s.AlbumID) == 0 && s.Album != "[non-album tracks]" {
				return errors.New("missing MusicBrainz album")
			}
			return nil
		})
	}
	if settings&checkSongCover != 0 {
		fs = append(fs, func(s *types.Song) error {
			// Returns true if fn exists within coverDir.
			fileExists := func(fn string) bool {
				_, err := os.Stat(filepath.Join(coverDir, fn))
				return err == nil
			}
			if len(s.CoverFilename) == 0 {
				if len(s.AlbumID) == 0 {
					return errors.New("no cover file set and no album ID")
				}
				fn := s.AlbumID + ".jpg"
				if fileExists(fn) {
					return fmt.Errorf("no cover file set but %v exists", fn)
				}
				return fmt.Errorf("no cover file set; album %s", s.AlbumID)
			}
			if !fileExists(s.CoverFilename) {
				return fmt.Errorf("missing cover file %s", s.CoverFilename)
			}
			return nil
		})
	}
	for _, f := range fs {
		for _, s := range songs {
			if err := f(s); err != nil {
				log.Printf("%s (%s): %v", s.SongID, s.Filename, err)
			}
		}
	}
}

func checkCovers(songs []*types.Song, coverDir string, settings checkSettings) {
	dir, err := os.Open(coverDir)
	if err != nil {
		log.Fatal("Failed to open cover dir: ", err)
	}
	defer dir.Close()

	fns, err := dir.Readdirnames(0)
	if err != nil {
		log.Fatal("Failed to read cover dir: ", err)
	}

	songFns := make(map[string]struct{})
	for _, s := range songs {
		if len(s.CoverFilename) > 0 {
			songFns[s.CoverFilename] = struct{}{}
		}
	}

	fs := [](func(fn string) error){
		func(fn string) error {
			if _, ok := songFns[fn]; !ok {
				return errors.New("unused cover")
			}
			return nil
		},
	}
	if settings&checkCoverSize400 != 0 {
		fs = append(fs, func(fn string) error {
			p := filepath.Join(coverDir, fn)
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()

			img, _, err := image.Decode(f)
			if err != nil {
				return fmt.Errorf("failed to decode %v: %v", p, err)
			}
			b := img.Bounds()
			if b.Dx() < 400 || b.Dy() < 400 {
				return fmt.Errorf("cover is only %vx%v", b.Dx(), b.Dy())
			}
			return nil
		})
	}

	for _, f := range fs {
		for _, fn := range fns {
			if err := f(fn); err != nil {
				log.Printf("%s: %v", fn, err)
			}
		}
	}
}

func main() {
	const defaultCoverSubdir = ".covers" // used if -cover-dir is unset

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage %v: [flag]...\n"+
			"Check for issues in songs and cover images.\n"+
			"Reads \"dump_music -covers\" song objects from stdin.\n\n",
			os.Args[0])
		flag.PrintDefaults()
	}

	musicDir := flag.String("music-dir", filepath.Join(os.Getenv("HOME"), "music"), "Directory containing song files")
	coverDir := flag.String("cover-dir", "",
		fmt.Sprintf("Directory containing cover art (%q within -music-dir if unset)", defaultCoverSubdir))

	checkInfos := map[string]struct { // keys are values for -check flag
		setting checkSettings
		desc    string // description for check flag
		def     bool   // on by default?
	}{
		"album-id":       {checkAlbumId, "Songs have MusicBrainz album IDs", true},
		"cover-size-400": {checkCoverSize400, "Cover images are at least 400x400", false},
		"song-cover":     {checkSongCover, "Songs have cover files", true},
	}
	var defaultChecks []string
	var checkDescs []string
	for s, info := range checkInfos {
		if info.def {
			defaultChecks = append(defaultChecks, s)
		}
		checkDescs = append(checkDescs, fmt.Sprintf("  %-14s  %s\n", s, info.desc))
	}
	sort.Strings(defaultChecks)
	sort.Strings(checkDescs)
	checkStr := flag.String("check", strings.Join(defaultChecks, ","),
		"Comma-separated list of checks to perform:\n"+strings.Join(checkDescs, ""))

	flag.Parse()

	if len(*coverDir) == 0 {
		*coverDir = filepath.Join(*musicDir, defaultCoverSubdir)
	}

	var settings checkSettings
	for _, s := range strings.Split(*checkStr, ",") {
		info, ok := checkInfos[s]
		if !ok {
			log.Fatalf("Invalid -check value %q", s)
		}
		settings |= info.setting
	}

	d := json.NewDecoder(os.Stdin)
	songs := make([]*types.Song, 0)
	for {
		var s types.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal("Failed to read song: ", err)
		}
		songs = append(songs, &s)
	}
	log.Printf("Read %d songs", len(songs))
	sort.Slice(songs, func(i, j int) bool { return songs[i].Filename < songs[j].Filename })

	checkSongs(songs, *musicDir, *coverDir, settings)
	checkCovers(songs, *coverDir, settings)
}
