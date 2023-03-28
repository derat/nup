// Copyright 2023 Daniel Erat.
// All rights reserved.

package scan

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/server/db"
	"github.com/google/go-cmp/cmp"
	"github.com/google/subcommands"
)

type Command struct {
	Cfg  *client.Config
	opts processSongOptions

	rel   *release // last-fetched release
	relID string   // ID requested when fetching rel (differs from rel.ID if release was merged)
}

func (*Command) Name() string     { return "scan" }
func (*Command) Synopsis() string { return "scan songs for updated metadata" }
func (*Command) Usage() string {
	return `scan [flags] <song.mp3>...:
	Scan songs for updated metadata using MusicBrainz and writes override files
	to metadataDir. Without positional arguments, scans all songs in musicDir.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&cmd.opts.dryRun, "dry-run", false, "Don't write override files")
	f.BoolVar(&cmd.opts.printUpdates, "print", true, "Print updates to stdout")
}

func (cmd *Command) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if fs.NArg() == 0 && len(cmd.Cfg.MusicDir) == 0 {
		fmt.Fprintln(os.Stderr, "musicDir not set in config")
		return subcommands.ExitUsageError
	}

	api := newAPI("https://musicbrainz.org")

	var errMsgs []string
	if fs.NArg() > 0 {
		for _, p := range fs.Args() {
			if err := processSong(ctx, cmd.Cfg, api, p, nil /* fi */, &cmd.opts); err != nil {
				errMsgs = append(errMsgs, fmt.Sprintf("%v: %v", p, err))
			}
		}
	} else {
		if err := filepath.Walk(cmd.Cfg.MusicDir, func(p string, fi os.FileInfo, err error) error {
			if fi.Mode().IsRegular() && files.IsMusicPath(p) {
				if err := processSong(ctx, cmd.Cfg, api, p, fi, &cmd.opts); err != nil {
					rel := p[len(cmd.Cfg.MusicDir)+1:]
					errMsgs = append(errMsgs, fmt.Sprintf("%v: %v", rel, err))
				}
			}
			return nil
		}); err != nil {
			errMsgs = append(errMsgs, fmt.Sprintf("Failed walking music dir: %v", err))
		}
	}

	// Print the error messages last so they're easier to find.
	if len(errMsgs) > 0 {
		for _, msg := range errMsgs {
			fmt.Fprintln(os.Stderr, msg)
		}
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

// processSongOptions configures processSong's behavior.
type processSongOptions struct {
	dryRun       bool // don't write override files
	printUpdates bool // print song updates to stderr
}

// processSong reads the song file at p, fetches updated metadata using api,
// and writes a metadata override file if needed. p and fi are passed to files.ReadSong.
func processSong(ctx context.Context, cfg *client.Config, api *api,
	p string, fi os.FileInfo, opts *processSongOptions) error {
	if opts == nil {
		opts = &processSongOptions{}
	}
	orig, err := files.ReadSong(cfg, p, fi, true /* onlyTags */, nil /* gc */)
	if err != nil {
		return err
	}
	updated, err := getSongUpdates(ctx, orig, api)
	if err != nil {
		return err
	}
	if orig.MetadataEquals(updated) {
		return nil
	}

	if opts.printUpdates {
		fmt.Println(orig.Filename + "\n" + diffSongs(orig, updated) + "\n")
	}
	if opts.dryRun {
		return nil
	}
	over := files.GenMetadataOverride(orig, updated)
	return files.WriteMetadataOverride(cfg, orig.Filename, over)
}

// getSongUpdates fetches metadata for song using api and returns an updated copy.
func getSongUpdates(ctx context.Context, song *db.Song, api *api) (*db.Song, error) {
	updated := *song

	switch {
	// Some old standalone recordings have their album set to "[non-album tracks]" but also have a
	// non-empty, now-deleted album ID. I think that (pre-NGS?) MB used to have per-artist fake
	// "[non-album tracks]" albums.
	case song.AlbumID != "" && song.Album != files.NonAlbumTracksValue:
		if song.RecordingID == "" {
			return nil, errors.New("no recording ID")
		}
		rel, err := api.getRelease(ctx, song.AlbumID)
		if err != nil {
			return nil, fmt.Errorf("release %v: %v", song.AlbumID, err)
		}
		if updateSongFromRelease(&updated, rel) {
			return &updated, nil
		}

		// If we didn't find the recording in the release, it might've been
		// merged into a different recording. Look up the recording to try
		// to get an updated ID that might be in the release.
		rec, err := api.getRecording(ctx, song.RecordingID)
		if err != nil {
			return nil, fmt.Errorf("recording %v: %v", song.RecordingID, err)
		}
		updated.RecordingID = rec.ID
		if !updateSongFromRelease(&updated, rel) {
			return nil, fmt.Errorf("recording %v not in release %v", rec.ID, rel.ID)
		}
		return &updated, nil

	case song.RecordingID != "":
		rec, err := api.getRecording(ctx, song.RecordingID)
		if err != nil {
			return nil, fmt.Errorf("recording %v: %v", song.RecordingID, err)
		}
		updateSongFromRecording(&updated, rec)
		return &updated, nil
	}

	return nil, errors.New("song is untagged")
}

// diffSongs diffs orig and updated and returns a multiline string describing differences.
func diffSongs(orig, updated *db.Song) string {
	type line struct{ op, name, val string }
	var lines []line
	var maxName int
	for _, ln := range strings.Split(cmp.Diff(*orig, *updated), "\n") {
		if ms := diffChangeRegexp.FindStringSubmatch(ln); ms != nil {
			if n := len(ms[2]); n > maxName {
				maxName = n
			}
			lines = append(lines, line{ms[1], ms[2], ms[3]})
		}
	}

	format := "%s   %-" + strconv.Itoa(maxName+1) + "s %s"
	strs := make([]string, len(lines))
	for i, ln := range lines {
		ln.val = strings.TrimRight(ln.val, ",")
		ln.val = diffDateRegexp.ReplaceAllString(ln.val, "$1")
		strs[i] = fmt.Sprintf(format, ln.op, ln.name+":", ln.val)
	}
	return strings.Join(strs, "\n")
}

// cmp.Diff inexplicably sometimes uses U+00A0 (non-breaking space) instead of spaces.
const spaces = "[ \t\u00a0]*"

var (
	// diffChangeRegexp matches a line in cmp.Diff's output that should be preserved.
	diffChangeRegexp = regexp.MustCompile(`^(\+|-)` + spaces + `([A-Z][^:]+):` + spaces + `(.+)`)
	// diffDateRegexp matches the string representation of a time.Time in cmp.Diff's output.
	diffDateRegexp = regexp.MustCompile(`s"(\d{4}-\d{2}-\d{2}) 00:00:00 \+0000 UTC"`)
)
