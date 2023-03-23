// Copyright 2023 Daniel Erat.
// All rights reserved.

package scan

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/server/db"
	"github.com/google/go-cmp/cmp"
	"github.com/google/subcommands"
)

const (
	songChanSize = 64
)

type Command struct {
	Cfg *client.Config
	api *api

	rel   *release // last-fetched release
	relID string   // ID requested when fetching rel (differs from rel.ID if release was merged)
}

func (*Command) Name() string     { return "scan" }
func (*Command) Synopsis() string { return "scan songs for updated metadata" }
func (*Command) Usage() string {
	return `scan [flags] <song.mp3>...:
	Scan songs for updated metadata using MusicBrainz.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
}

func (cmd *Command) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if fs.NArg() == 0 && len(cmd.Cfg.MusicDir) == 0 {
		fmt.Fprintln(os.Stderr, "musicDir not set in config")
		return subcommands.ExitUsageError
	}

	cmd.api = newAPI("https://musicbrainz.org")

	type songOrErr struct {
		path string // relative to cmd.Cfg.MusicDir if no positional args were supplied
		song *db.Song
		err  error
	}
	ch := make(chan songOrErr, songChanSize)

	go func() {
		if fs.NArg() > 0 {
			for _, p := range fs.Args() {
				var song *db.Song
				fi, err := os.Stat(p)
				if err == nil {
					song, err = files.ReadSong(cmd.Cfg, p, fi, true /* onlyTags */, nil /* gc */)
				}
				ch <- songOrErr{p, song, err}
			}
		} else {
			if err := filepath.Walk(cmd.Cfg.MusicDir, func(p string, fi os.FileInfo, err error) error {
				if fi.Mode().IsRegular() && files.IsMusicPath(p) {
					song, err := files.ReadSong(cmd.Cfg, p, fi, true /* onlyTags */, nil /* gc */)
					ch <- songOrErr{p[len(cmd.Cfg.MusicDir)+1:], song, err}
				}
				return nil
			}); err != nil {
				ch <- songOrErr{"", nil, err}
			}
		}
		close(ch)
	}()

	for soe := range ch {
		if soe.err != nil {
			log.Printf("%v: %v", soe.path, soe.err)
			continue
		}
		orig := soe.song
		updated, err := cmd.getUpdates(ctx, orig)
		if err != nil {
			log.Printf("%v: %v", soe.path, err)
			continue
		}
		if !orig.MetadataEquals(updated) {
			fmt.Println(soe.path + "\n" + diffSongs(orig, updated) + "\n")
		}
	}

	return subcommands.ExitSuccess
}

// untaggedErr is returned by getUpdates if song.AlbumID and song.RecordingID are both empty.
var untaggedErr = errors.New("song is untagged")

// getUpdates fetches metadata for song from MusicBrainz and returns an updated copy.
func (cmd *Command) getUpdates(ctx context.Context, song *db.Song) (*db.Song, error) {
	updated := *song

	switch {
	case song.AlbumID != "":
		if song.RecordingID == "" {
			return nil, errors.New("no recording ID")
		}
		if cmd.rel == nil || (song.AlbumID != cmd.rel.ID && song.AlbumID != cmd.relID) {
			var err error
			cmd.relID = song.AlbumID
			if cmd.rel, err = cmd.api.getRelease(ctx, song.AlbumID); err != nil {
				cmd.relID = ""
				return nil, fmt.Errorf("release %v: %v", song.AlbumID, err)
			}
		}
		if updateSongFromRelease(&updated, cmd.rel) {
			return &updated, nil
		}

		// If we didn't find the recording in the release, it might've been
		// merged into a different recording. Look up the recording to try
		// to get an updated ID that might be in the release.
		rec, err := cmd.api.getRecording(ctx, song.RecordingID)
		if err != nil {
			return nil, fmt.Errorf("recording %v: %v", song.RecordingID, err)
		}
		updated.RecordingID = rec.ID
		if !updateSongFromRelease(&updated, cmd.rel) {
			return nil, fmt.Errorf("recording %v not in release %v", rec.ID, cmd.rel.ID)
		}
		return &updated, nil

	case song.RecordingID != "":
		rec, err := cmd.api.getRecording(ctx, song.RecordingID)
		if err != nil {
			return nil, fmt.Errorf("recording %v: %v", song.RecordingID, err)
		}
		updateSongFromRecording(&updated, rec)
		return &updated, nil
	}

	return nil, untaggedErr
}

// updateSongFromRelease updates fields in song using data from rel.
// false is returned if the recording isn't included in the release.
func updateSongFromRelease(song *db.Song, rel *release) bool {
	tr, med := rel.findTrack(song.RecordingID)
	if tr == nil {
		return false
	}

	song.Artist = joinArtistCredits(tr.Artists)
	song.Title = tr.Title
	song.Album = rel.Title
	song.DiscSubtitle = med.Title
	song.AlbumID = rel.ID
	song.Track = tr.Position
	song.Disc = med.Position
	song.Date = time.Time(rel.ReleaseGroup.FirstReleaseDate)

	// Only set the album artist if it differs from the song artist or if it was previously set.
	// Otherwise we're creating needless churn, since the update command won't send it to the server
	// if it's the same as the song artist.
	if aa := joinArtistCredits(rel.Artists); aa != song.Artist || song.AlbumArtist != "" {
		song.AlbumArtist = aa
	}

	return true
}

// updateSongFromRecording updates fields in song using data from rec.
// This should only be used for standalone recordings.
func updateSongFromRecording(song *db.Song, rec *recording) {
	song.Artist = joinArtistCredits(rec.Artists)
	song.Title = rec.Title
	song.Album = files.NonAlbumTracksValue
	song.Date = time.Time(rec.FirstReleaseDate) // always zero?
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

// Diff inexplicably sometimes uses U+00A0 (non-breaking space) instead of spaces.
const spaces = "[ \t\u00a0]*"

var (
	// diffChangeRegexp matches a line in cmp.Diff's output that should be preserved.
	diffChangeRegexp = regexp.MustCompile(`^(\+|-)` + spaces + `([A-Z][^:]+):` + spaces + `(.+)`)
	// diffDateRegexp matches the string representation of a time.Time in cmp.Diff's output.
	diffDateRegexp = regexp.MustCompile(`s"(\d{4}-\d{2}-\d{2}) 00:00:00 \+0000 UTC"`)
)
