// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/mp3gain"
	"github.com/derat/nup/server/db"
	"github.com/google/subcommands"
)

type Command struct {
	Cfg *client.Config

	deleteSongID    int64  // ID of song to delete
	dryRun          bool   // print actions instead of doing anything
	dumpedGainsFile string // path to dump file containing pre-computed gains
	forceGlob       string // files to force updating
	importJSONFile  string // path to JSON file with Song objects to import
	importUserData  bool   // replace user data when using importJSONFile
	limit           int    // maximum number of songs to update
	mergeSongIDs    string // IDs of songs to merge, as "from:to"
	requireCovers   bool   // die if cover images are missing
	songPathsFile   string // path to list of songs to force updating
	testGainInfo    string // hardcoded gain info as "track:album:amp" for testing
}

func (*Command) Name() string     { return "update" }
func (*Command) Synopsis() string { return "send song updates to the server" }
func (*Command) Usage() string {
	return `update [flags]:
	Send song updates to the server.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.Int64Var(&cmd.deleteSongID, "delete-song", 0, "Delete song with given ID")
	f.BoolVar(&cmd.dryRun, "dry-run", false, "Only print what would be updated")
	f.StringVar(&cmd.dumpedGainsFile, "dumped-gains-file", "",
		"Path to dump file from which songs' gains will be read (instead of being computed)")
	f.StringVar(&cmd.forceGlob, "force-glob", "",
		"Glob pattern relative to music dir for files to scan and update even if they haven't changed")
	f.StringVar(&cmd.importJSONFile, "import-json-file", "",
		"If non-empty, path to JSON file containing a stream of Song objects to import")
	f.BoolVar(&cmd.importUserData, "import-user-data", true,
		"When importing from JSON, replace user data (ratings, tags, plays, etc.)")
	f.IntVar(&cmd.limit, "limit", 0,
		"If positive, limits the number of songs to update (for testing)")
	f.StringVar(&cmd.mergeSongIDs, "merge-songs", "",
		`Merge one song's user data into another song, with IDs as "from:to"`)
	f.BoolVar(&cmd.requireCovers, "require-covers", false,
		"Die if cover images aren't found for any songs that have album IDs")
	f.StringVar(&cmd.songPathsFile, "song-paths-file", "",
		"Path to a file containing one relative path per line for songs to force updating")
	f.StringVar(&cmd.testGainInfo, "test-gain-info", "",
		"Hardcoded gain info as \"track:album:amp\" (for testing)")
}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if countBools(cmd.deleteSongID > 0, cmd.importJSONFile != "", cmd.mergeSongIDs != "", cmd.songPathsFile != "") > 1 {
		fmt.Fprintln(os.Stderr, "-delete-song, -import-json-file, -merge-songs, and -song-paths-file "+
			"are mutually exclusive")
		return subcommands.ExitUsageError
	}

	if cmd.deleteSongID > 0 {
		return cmd.doDeleteSong()
	}
	if cmd.mergeSongIDs != "" {
		return cmd.doMergeSongs()
	}

	var err error
	var numSongs int
	var scannedDirs []string
	var replaceUserData, didFullScan bool
	readChan := make(chan songOrErr)
	startTime := time.Now()

	if len(cmd.testGainInfo) > 0 {
		var info mp3gain.Info
		if _, err := fmt.Sscanf(cmd.testGainInfo, "%f:%f:%f",
			&info.TrackGain, &info.AlbumGain, &info.PeakAmp); err != nil {
			fmt.Fprintln(os.Stderr, "Bad -test-gain-info (want \"track:album:amp\"):", err)
			return subcommands.ExitUsageError
		}
		mp3gain.SetInfoForTest(&info)
	}

	if len(cmd.importJSONFile) > 0 {
		if numSongs, err = readSongsFromJSONFile(cmd.importJSONFile, readChan); err != nil {
			fmt.Fprintln(os.Stderr, "Failed reading songs:", err)
			return subcommands.ExitFailure
		}
		replaceUserData = cmd.importUserData
	} else {
		if len(cmd.Cfg.MusicDir) == 0 {
			fmt.Fprintln(os.Stderr, "musicDir not set in config")
			return subcommands.ExitUsageError
		}

		// Not all these options will necessarily be used (e.g. readSongList doesn't need forceGlob
		// or logProgress), but it doesn't hurt to pass them.
		opts := scanOptions{
			computeGain:    cmd.Cfg.ComputeGain,
			forceGlob:      cmd.forceGlob,
			logProgress:    true,
			artistRewrites: cmd.Cfg.ArtistRewrites,
		}

		if len(cmd.dumpedGainsFile) > 0 {
			opts.dumpedGains, err = readDumpedGains(cmd.dumpedGainsFile)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed reading dumped gains:", err)
				return subcommands.ExitFailure
			}
		}

		if len(cmd.songPathsFile) > 0 {
			numSongs, err = readSongList(cmd.songPathsFile, cmd.Cfg.MusicDir, readChan, &opts)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed reading song list:", err)
				return subcommands.ExitFailure
			}
		} else {
			if len(cmd.Cfg.LastUpdateInfoFile) == 0 {
				fmt.Fprintln(os.Stderr, "lastUpdateInfoFile not set in config")
				return subcommands.ExitUsageError
			}
			info, err := readLastUpdateInfo(cmd.Cfg.LastUpdateInfoFile)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Unable to get last update info:", err)
				return subcommands.ExitFailure
			}
			log.Printf("Scanning for songs in %v updated since %v", cmd.Cfg.MusicDir, info.Time.Local())
			numSongs, scannedDirs, err = scanForUpdatedSongs(
				cmd.Cfg.MusicDir, info.Time, info.Dirs, readChan, &opts)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Scanning failed:", err)
				return subcommands.ExitFailure
			}
			didFullScan = true
		}
	}

	if cmd.limit > 0 && numSongs > cmd.limit {
		numSongs = cmd.limit
	}

	log.Printf("Sending %v song(s)", numSongs)

	// Look up covers and feed songs to the updater.
	updateChan := make(chan db.Song)
	errChan := make(chan error, 1)
	go func() {
		for i := 0; i < numSongs; i++ {
			soe := <-readChan
			if soe.err != nil {
				fn := "[unknown]"
				if soe.song != nil {
					fn = soe.song.Filename
				}
				errChan <- fmt.Errorf("%v: %v", fn, soe.err)
				return
			}
			s := *soe.song
			s.CoverFilename = getCoverFilename(cmd.Cfg.CoverDir, &s)
			if cmd.requireCovers && len(s.CoverFilename) == 0 &&
				(len(s.AlbumID) > 0 || len(s.CoverID) > 0 || len(s.RecordingID) > 0) {
				errChan <- fmt.Errorf("missing cover for %v (album=%v, cover=%v, recording=%v)",
					s.Filename, s.AlbumID, s.CoverID, s.RecordingID)
				return
			}
			s.RecordingID = ""

			log.Print("Sending ", s.Filename)
			updateChan <- s
		}
		close(updateChan)
		close(errChan)
	}()

	if cmd.dryRun {
		enc := json.NewEncoder(os.Stdout)
		for s := range updateChan {
			if err := enc.Encode(s); err != nil {
				fmt.Fprintln(os.Stderr, "Failed encoding song:", err)
				return subcommands.ExitFailure
			}
		}
	} else {
		if err := updateSongs(cmd.Cfg, updateChan, replaceUserData); err != nil {
			fmt.Fprintln(os.Stderr, "Failed updating songs:", err)
		}
		if didFullScan {
			if err := writeLastUpdateInfo(cmd.Cfg.LastUpdateInfoFile, lastUpdateInfo{
				Time: startTime,
				Dirs: scannedDirs,
			}); err != nil {
				fmt.Fprintln(os.Stderr, "Failed saving update info:", err)
				return subcommands.ExitFailure
			}
		}
	}

	if err := <-errChan; err != nil {
		fmt.Fprintln(os.Stderr, "Failed scanning song files:", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (cmd *Command) doDeleteSong() subcommands.ExitStatus {
	if cmd.dryRun {
		fmt.Fprintln(os.Stderr, "-dry-run is incompatible with -delete-song")
		return subcommands.ExitUsageError
	}
	if err := deleteSong(cmd.Cfg, cmd.deleteSongID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed deleting song %v: %v\n", cmd.deleteSongID, err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (cmd *Command) doMergeSongs() subcommands.ExitStatus {
	var fromID, toID int64
	if _, err := fmt.Sscanf(cmd.mergeSongIDs, "%d:%d", &fromID, &toID); err != nil {
		fmt.Fprintln(os.Stderr, `-merge-songs needs IDs to merge as "from:to"`)
		return subcommands.ExitUsageError
	}
	var err error
	var from, to db.Song
	if from, err = dumpSong(cmd.Cfg, fromID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed dumping song %v: %v\n", fromID, err)
		return subcommands.ExitFailure
	}
	if to, err = dumpSong(cmd.Cfg, toID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed dumping song %v: %v\n", toID, err)
		return subcommands.ExitFailure
	}
	to.Rating = from.Rating
	to.Tags = append(to.Tags, from.Tags...)
	to.Plays = append(to.Plays, from.Plays...)

	if cmd.dryRun {
		if err := json.NewEncoder(os.Stdout).Encode(to); err != nil {
			fmt.Fprintln(os.Stderr, "Failed encoding song:", err)
			return subcommands.ExitFailure
		}
	} else {
		ch := make(chan db.Song, 1)
		ch <- to
		close(ch)
		if err := updateSongs(cmd.Cfg, ch, true /* replaceUserData */); err != nil {
			fmt.Fprintf(os.Stderr, "Failed updating song %v: %v\n", toID, err)
			return subcommands.ExitFailure
		}
	}
	return subcommands.ExitSuccess
}

type songOrErr struct {
	song *db.Song
	err  error
}

func countBools(vals ...bool) int {
	var cnt int
	for _, v := range vals {
		if v {
			cnt++
		}
	}
	return cnt
}

// lastUpdateInfo contains information about the last full update that was performed.
// It is used to identify new music files.
type lastUpdateInfo struct {
	// Time contains the time at which the last update was started.
	Time time.Time `json:"time"`
	// Dirs contains all song-containing directories that were seen (relative to config.MusicDir).
	Dirs []string `json:"dirs"`
}

// readLastUpdateInfo JSON-unmarshals a lastUpdateInfo struct from the file at p.
func readLastUpdateInfo(p string) (info lastUpdateInfo, err error) {
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return info, nil
		}
		return info, err
	}
	defer f.Close()

	err = json.NewDecoder(f).Decode(&info)
	return info, err
}

// writeLastUpdateInfo JSON-marshals info to a file at p.
func writeLastUpdateInfo(p string, info lastUpdateInfo) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(info); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// getCoverFilename returns the relative path under dir for song's cover image.
func getCoverFilename(dir string, song *db.Song) string {
	for _, s := range []string{song.CoverID, song.AlbumID, song.RecordingID} {
		if len(s) > 0 {
			fn := s + ".jpg"
			if _, err := os.Stat(filepath.Join(dir, fn)); err == nil {
				return fn
			}
		}
	}
	return ""
}

// readDumpedGains reads gains from a song dump file at p.
// The returned map is keyed by song filename.
func readDumpedGains(p string) (map[string]mp3gain.Info, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gains := make(map[string]mp3gain.Info)
	d := json.NewDecoder(f)
	for {
		var s db.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if s.TrackGain == 0 && s.AlbumGain == 0 && s.PeakAmp == 0 {
			return nil, fmt.Errorf("missing gain info for %q", s.Filename)
		}
		gains[s.Filename] = mp3gain.Info{
			TrackGain: s.TrackGain,
			AlbumGain: s.AlbumGain,
			PeakAmp:   s.PeakAmp,
		}
	}
	return gains, nil
}
