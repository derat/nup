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
	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/cmd/nup/mp3gain"
	"github.com/derat/nup/server/cover"
	"github.com/derat/nup/server/db"
	"github.com/google/subcommands"
)

type Command struct {
	Cfg *client.Config

	compareDumpFile  string // path of file with song dumps to compare against
	deleteAfterMerge bool   // delete source song if mergeSongIDs is true
	deleteSongID     int64  // ID of song to delete
	dryRun           bool   // print actions instead of doing anything
	dumpedGainsFile  string // path to dump file with pre-computed gains
	forceGlob        string // files to force updating
	importJSONFile   string // path to JSON file with Song objects to import
	importUserData   bool   // replace user data when using importJSONFile
	limit            int    // maximum number of songs to update
	mergeSongIDs     string // IDs of songs to merge, as "from:to"
	printCoverID     string // path to song file whose cover ID should be printed
	reindexSongs     bool   // ask the server to reindex all songs
	requireCovers    bool   // die if cover images are missing
	songPathsFile    string // path to list of songs to force updating
	testGainInfo     string // hardcoded gain info as "track:album:amp" for testing
	useFilenames     bool   // use filenames instead of SHA1s to identify songs
}

func (*Command) Name() string     { return "update" }
func (*Command) Synopsis() string { return "send song updates to the server" }
func (*Command) Usage() string {
	return `update [flags]:
	Send song updates to the server.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.compareDumpFile, "compare-dump-file", "", "Path to JSON file with songs to compare updates against")
	f.BoolVar(&cmd.deleteAfterMerge, "delete-after-merge", false, "Delete source song if -merge-songs is true")
	f.Int64Var(&cmd.deleteSongID, "delete-song", 0, "Delete song with given ID")
	f.BoolVar(&cmd.dryRun, "dry-run", false, "Only print what would be updated")
	f.StringVar(&cmd.dumpedGainsFile, "dumped-gains-file", "",
		"Path to dump file from which songs' gains will be read (instead of being computed)")
	f.StringVar(&cmd.forceGlob, "force-glob", "",
		"Glob pattern relative to music dir for files to scan and update even if they haven't changed")
	f.StringVar(&cmd.importJSONFile, "import-json-file", "", "Path to JSON file with songs to import")
	f.BoolVar(&cmd.importUserData, "import-user-data", true,
		"When importing from JSON, replace user data (ratings, tags, plays, etc.)")
	f.IntVar(&cmd.limit, "limit", 0, "Limit the number of songs to update (for testing)")
	f.StringVar(&cmd.mergeSongIDs, "merge-songs", "",
		`Merge one song's user data into another song, with IDs as "src:dst"`)
	f.StringVar(&cmd.printCoverID, "print-cover-id", "", `Print cover ID for specified song file`)
	f.BoolVar(&cmd.reindexSongs, "reindex-songs", false,
		"Ask server to reindex all songs' search-related fields (not typically neaded)")
	f.BoolVar(&cmd.requireCovers, "require-covers", false,
		"Die if cover images aren't found for any songs that have album IDs")
	f.StringVar(&cmd.songPathsFile, "song-paths-file", "",
		"Path to file with one relative path per line for songs to force updating")
	f.StringVar(&cmd.testGainInfo, "test-gain-info", "",
		"Hardcoded gain info as \"track:album:amp\" (for testing)")
	f.BoolVar(&cmd.useFilenames, "use-filenames", false,
		"Identify songs by filename rather than audio data hash (useful when modifying files)")
}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if countBools(cmd.deleteSongID > 0, cmd.importJSONFile != "", cmd.mergeSongIDs != "",
		cmd.printCoverID != "", cmd.reindexSongs, cmd.songPathsFile != "") > 1 {
		fmt.Fprintln(os.Stderr, "-delete-song, -import-json-file, -merge-songs, -print-cover-id, "+
			"-reindex-songs, and -song-paths-file are mutually exclusive")
		return subcommands.ExitUsageError
	}

	// Handle flags that don't use the normal update process.
	switch {
	case cmd.deleteSongID > 0:
		return cmd.doDeleteSong()
	case cmd.mergeSongIDs != "":
		return cmd.doMergeSongs()
	case cmd.printCoverID != "":
		return cmd.doPrintCoverID()
	case cmd.reindexSongs:
		return cmd.doReindexSongs()
	}

	var err error
	var numSongs int
	var scannedDirs []string
	var replaceUserData, didFullScan bool
	var oldSongs map[string]*db.Song
	readChan := make(chan songOrErr)
	startTime := time.Now()

	if cmd.testGainInfo != "" {
		var info mp3gain.Info
		if _, err := fmt.Sscanf(cmd.testGainInfo, "%f:%f:%f",
			&info.TrackGain, &info.AlbumGain, &info.PeakAmp); err != nil {
			fmt.Fprintln(os.Stderr, "Bad -test-gain-info (want \"track:album:amp\"):", err)
			return subcommands.ExitUsageError
		}
		mp3gain.SetInfoForTest(&info)
	}

	if cmd.compareDumpFile != "" {
		if oldSongs, err = readDumpedSongs(cmd.compareDumpFile, cmd.useFilenames); err != nil {
			fmt.Fprintln(os.Stderr, "Failed reading songs from -compare-dump-file:", err)
			return subcommands.ExitFailure
		}
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
			forceGlob:       cmd.forceGlob,
			logProgress:     true,
			dumpedGainsPath: cmd.dumpedGainsFile,
		}

		if len(cmd.songPathsFile) > 0 {
			numSongs, err = readSongList(cmd.Cfg, cmd.songPathsFile, readChan, &opts)
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
			numSongs, scannedDirs, err = scanForUpdatedSongs(cmd.Cfg, info.Time, info.Dirs, readChan, &opts)
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

	log.Printf("Processing %v song(s)", numSongs)

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
				break
			}
			s := *soe.song
			s.CoverFilename = getCoverFilename(cmd.Cfg.CoverDir, &s)
			if cmd.requireCovers && len(s.CoverFilename) == 0 && (len(s.AlbumID) > 0 || len(s.CoverID) > 0) {
				errChan <- fmt.Errorf("missing cover for %v (album=%v, cover=%v)", s.Filename, s.AlbumID, s.CoverID)
				break
			}
			s.RecordingID = ""

			// Check that the metadata actually changed to avoid unnecessary datastore writes.
			key := s.SHA1
			if cmd.useFilenames {
				key = s.Filename
			}
			if old, ok := oldSongs[key]; ok && s.MetadataEquals(old) {
				log.Print("Skipping unchanged ", s.Filename)
				continue
			}

			// Don't send user data to the server if it would just throw it away.
			if !replaceUserData {
				s.Rating = 0
				s.Tags = nil
				s.Plays = nil
			}

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
	} else if err := updateSongs(cmd.Cfg, updateChan, replaceUserData, cmd.useFilenames); err != nil {
		fmt.Fprintln(os.Stderr, "Failed updating songs:", err)
	}

	if err := <-errChan; err != nil {
		fmt.Fprintln(os.Stderr, "Failed scanning song files:", err)
		return subcommands.ExitFailure
	}

	if !cmd.dryRun && didFullScan {
		if err := writeLastUpdateInfo(cmd.Cfg.LastUpdateInfoFile, lastUpdateInfo{
			Time: startTime,
			Dirs: scannedDirs,
		}); err != nil {
			fmt.Fprintln(os.Stderr, "Failed saving update info:", err)
			return subcommands.ExitFailure
		}
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
	var srcID, dstID int64
	if _, err := fmt.Sscanf(cmd.mergeSongIDs, "%d:%d", &srcID, &dstID); err != nil {
		fmt.Fprintln(os.Stderr, `-merge-songs needs IDs to merge as "src:dst"`)
		return subcommands.ExitUsageError
	}
	if srcID == dstID {
		fmt.Fprintf(os.Stderr, "Can't merge song %d into itself\n", srcID)
		return subcommands.ExitUsageError
	}

	var err error
	var src, dst db.Song
	if src, err = dumpSong(cmd.Cfg, srcID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed dumping song %v: %v\n", srcID, err)
		return subcommands.ExitFailure
	}
	if dst, err = dumpSong(cmd.Cfg, dstID); err != nil {
		fmt.Fprintf(os.Stderr, "Failed dumping song %v: %v\n", dstID, err)
		return subcommands.ExitFailure
	}
	if src.Rating > dst.Rating {
		dst.Rating = src.Rating
	}
	dst.Tags = append(dst.Tags, src.Tags...)
	dst.Plays = append(dst.Plays, src.Plays...)
	dst.Clean() // sort and dedupe Tags and Plays

	if cmd.dryRun {
		if err := json.NewEncoder(os.Stdout).Encode(dst); err != nil {
			fmt.Fprintln(os.Stderr, "Failed encoding song:", err)
			return subcommands.ExitFailure
		}
	} else {
		ch := make(chan db.Song, 1)
		ch <- dst
		close(ch)
		if err := updateSongs(cmd.Cfg, ch, true, /* replaceUserData */
			false /* useFilenames */); err != nil {
			fmt.Fprintf(os.Stderr, "Failed updating song %v: %v\n", dstID, err)
			return subcommands.ExitFailure
		}
		if cmd.deleteAfterMerge {
			if err := deleteSong(cmd.Cfg, srcID); err != nil {
				fmt.Fprintf(os.Stderr, "Failed deleting song %v: %v\n", srcID, err)
				return subcommands.ExitFailure
			}
		}
	}
	return subcommands.ExitSuccess
}

func (cmd *Command) doPrintCoverID() subcommands.ExitStatus {
	// Just set the file's directory as the music dir so that ReadSong won't
	// fail when computing a relative path (which we don't use anyway).
	cmd.Cfg.MusicDir = filepath.Dir(cmd.printCoverID)
	s, err := files.ReadSong(cmd.Cfg, cmd.printCoverID, nil, true /* onlyTags */, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed reading song:", err)
		return subcommands.ExitFailure
	}
	ids := getCoverIDs(s)
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "Couldn't find cover ID in metadata")
		return subcommands.ExitFailure
	}
	fmt.Println(ids[0])
	return subcommands.ExitSuccess
}

func (cmd *Command) doReindexSongs() subcommands.ExitStatus {
	if cmd.dryRun {
		fmt.Fprintln(os.Stderr, "-dry-run is incompatible with -reindex-songs")
		return subcommands.ExitUsageError
	}
	if err := reindexSongs(cmd.Cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Failed reindexing songs:", err)
		return subcommands.ExitFailure
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
	// Time is the time at which the last update was started.
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

// getCoverIDs returns IDs for song's cover in their preferred order.
func getCoverIDs(song *db.Song) []string {
	var ids []string
	for _, id := range []string{song.CoverID, song.AlbumID, song.RecordingID} {
		if len(id) > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// getCoverFilename returns the relative path under dir for song's cover image.
func getCoverFilename(dir string, song *db.Song) string {
	for _, id := range getCoverIDs(song) {
		fn := id + cover.OrigExt
		if _, err := os.Stat(filepath.Join(dir, fn)); err == nil {
			return fn
		}
	}
	return ""
}

// readDumpedSongs JSON-unmarshals db.Song objects from p and returns them in a map.
// If useFilenames is true, the map is keyed by each song's Filename field; otherwise
// it is keyed by the SHA1 field.
func readDumpedSongs(p string, useFilenames bool) (map[string]*db.Song, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	songs := make(map[string]*db.Song)
	d := json.NewDecoder(f)
	for {
		var s db.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		if useFilenames {
			songs[s.Filename] = &s
		} else {
			songs[s.SHA1] = &s
		}
	}

	return songs, nil
}
