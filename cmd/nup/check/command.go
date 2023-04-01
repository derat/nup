// Copyright 2020 Daniel Erat.
// All rights reserved.

package check

import (
	"context"
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

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/server/cover"
	"github.com/derat/nup/server/db"
	"github.com/google/subcommands"
)

// checkSettings is a bitfield describing which checks to perform.
type checkSettings uint32

const (
	checkAlbumID checkSettings = 1 << iota
	checkCoverSize400
	checkCoverSize800
	checkImported
	checkMetadata
	checkSongCover
)

var checkInfos = map[string]struct { // keys are values for -check flag
	setting checkSettings
	desc    string // description for check flag
	def     bool   // on by default?
}{
	"album-id":       {checkAlbumID, "Songs have MusicBrainz album IDs", true},
	"cover-size-400": {checkCoverSize400, "Cover images are at least 400x400", false},
	"cover-size-800": {checkCoverSize800, "Cover images are at least 800x800", false},
	"imported":       {checkImported, "All songs have been imported", true},
	"metadata":       {checkMetadata, "Song metadata is the same in dumped songs and locally", false},
	"song-cover":     {checkSongCover, "Songs have cover files", true},
}

type Command struct {
	Cfg        *client.Config
	checksList string // comma-separated list of checks to perform
	checks     checkSettings
}

func (*Command) Name() string     { return "check" }
func (*Command) Synopsis() string { return "check for issues in songs and cover images" }
func (*Command) Usage() string {
	return `check <flags>:
	Check for issues in dumped songs read from stdin.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
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
	f.StringVar(&cmd.checksList, "check", strings.Join(defaultChecks, ","),
		"Comma-separated list of checks to perform:\n"+strings.Join(checkDescs, ""))
}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if cmd.Cfg.MusicDir == "" || cmd.Cfg.CoverDir == "" {
		fmt.Fprintln(os.Stderr, "musicDir and coverDir must be set in config")
		return subcommands.ExitUsageError
	}

	for _, s := range strings.Split(cmd.checksList, ",") {
		info, ok := checkInfos[s]
		if !ok {
			fmt.Fprintf(os.Stderr, "Invalid -check value %q\n", s)
			return subcommands.ExitUsageError
		}
		cmd.checks |= info.setting
	}

	d := json.NewDecoder(os.Stdin)
	songs := make([]*db.Song, 0)
	for {
		var s db.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			fmt.Fprintln(os.Stderr, "Failed reading song:", err)
			return subcommands.ExitFailure
		}
		songs = append(songs, &s)
	}
	log.Printf("Read %d songs", len(songs))
	sort.Slice(songs, func(i, j int) bool { return songs[i].Filename < songs[j].Filename })

	if err := cmd.checkSongs(songs); err != nil {
		fmt.Fprintln(os.Stderr, "Failed checking songs:", err)
		return subcommands.ExitFailure
	}
	if err := cmd.checkCovers(songs); err != nil {
		fmt.Fprintln(os.Stderr, "Failed checking covers:", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (cmd *Command) checkSongs(songs []*db.Song) error {
	seenFilenames := make(map[string]string, len(songs))
	fs := [](func(s *db.Song) error){
		func(s *db.Song) error {
			if len(s.Filename) == 0 {
				return errors.New("no song filename")
			} else if _, err := os.Stat(filepath.Join(cmd.Cfg.MusicDir, s.Filename)); err != nil {
				return errors.New("missing song file")
			}
			if id, ok := seenFilenames[s.Filename]; ok {
				return fmt.Errorf("song %s uses same file", id)
			}
			seenFilenames[s.Filename] = s.SongID
			return nil
		},
	}

	if cmd.checks&checkAlbumID != 0 {
		fs = append(fs, func(s *db.Song) error {
			if len(s.AlbumID) == 0 && s.Album != files.NonAlbumTracksValue {
				return errors.New("missing MusicBrainz album")
			}
			return nil
		})
	}

	if cmd.checks&checkSongCover != 0 {
		fs = append(fs, func(s *db.Song) error {
			// Returns true if fn exists within the cover dir.
			fileExists := func(fn string) bool {
				_, err := os.Stat(filepath.Join(cmd.Cfg.CoverDir, fn))
				return err == nil
			}
			if len(s.CoverFilename) == 0 {
				if len(s.AlbumID) == 0 {
					return errors.New("no cover file set and no album ID")
				}
				fn := s.AlbumID + cover.OrigExt
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

	if cmd.checks&checkMetadata != 0 {
		fs = append(fs, func(s *db.Song) error {
			abs := filepath.Join(cmd.Cfg.MusicDir, s.Filename)
			local, err := files.ReadSong(cmd.Cfg, abs, nil, files.SkipAudioData, nil /* gc */)
			if err != nil {
				return err
			}
			dump := *s

			// Clear fields that aren't set when reading only tags or when dumping.
			local.CoverID = ""
			local.RecordingID = ""
			local.OrigAlbumID = ""
			local.OrigRecordingID = ""

			dump.SHA1 = ""
			dump.SongID = ""
			dump.CoverFilename = ""
			dump.Length = 0
			dump.TrackGain = 0
			dump.AlbumGain = 0
			dump.PeakAmp = 0
			dump.Rating = 0
			dump.Tags = nil
			dump.Plays = nil

			if diff := db.DiffSongs(&dump, local); diff != "" {
				return errors.New("dumped and local metadata differ:\n" + diff)
			}
			return nil
		})
	}

	for _, f := range fs {
		for _, s := range songs {
			if err := f(s); err != nil {
				fmt.Printf("%s (%s): %v\n", s.SongID, s.Filename, err)
			}
		}
	}

	if cmd.checks&checkImported != 0 {
		known := make(map[string]struct{}, len(songs))
		for _, s := range songs {
			known[s.Filename] = struct{}{}
		}
		if err := filepath.Walk(cmd.Cfg.MusicDir, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !fi.Mode().IsRegular() || !files.IsMusicPath(path) {
				return nil
			}
			pre := cmd.Cfg.MusicDir + "/"
			if !strings.HasPrefix(path, pre) {
				return fmt.Errorf("%v doesn't have expected prefix %v", path, pre)
			}
			path = path[len(pre):]
			if _, ok := known[path]; !ok {
				fmt.Printf("%v not imported\n", path)
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed walking %v: %v", cmd.Cfg.MusicDir, err)
		}
	}

	return nil
}

func (cmd *Command) checkCovers(songs []*db.Song) error {
	dir, err := os.Open(cmd.Cfg.CoverDir)
	if err != nil {
		return err
	}
	defer dir.Close()

	fns, err := dir.Readdirnames(0)
	if err != nil {
		return err
	}

	songFns := make(map[string]string) // values are "[artist] - [album]"
	for _, s := range songs {
		if len(s.CoverFilename) > 0 {
			songFns[s.CoverFilename] = s.Artist + " - " + s.Album
		}
	}

	fs := [](func(fn string) error){
		func(fn string) error {
			// Check for the original cover if this is a generated WebP image.
			fn = cover.OrigFilename(fn)
			if _, ok := songFns[fn]; !ok {
				return errors.New("unused cover")
			}
			return nil
		},
	}

	if cmd.checks&(checkCoverSize400|checkCoverSize800) != 0 {
		min := 400
		if cmd.checks&checkCoverSize800 != 0 {
			min = 800
		}
		fs = append(fs, func(fn string) error {
			p := filepath.Join(cmd.Cfg.CoverDir, fn)
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
			if b.Dx() < min || b.Dy() < min {
				return fmt.Errorf("cover is only %vx%v", b.Dx(), b.Dy())
			}
			return nil
		})
	}

	for _, f := range fs {
		for _, fn := range fns {
			if err := f(fn); err != nil {
				key := fn
				if s := songFns[fn]; s != "" {
					key += " (" + s + ")"
				}
				fmt.Printf("%s: %v\n", key, err)
			}
		}
	}
	return nil
}
