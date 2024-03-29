// Copyright 2023 Daniel Erat.
// All rights reserved.

package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/client/files"
	"github.com/derat/nup/server/db"
	"github.com/google/subcommands"
)

// maxSongLengthDiff is the maximum difference in length to allow between on-disk songs
// and MusicBrainz tracks when updating album IDs.
const maxSongLengthDiff = 5 * time.Second

type Command struct {
	Cfg          *client.Config
	opts         updateOptions
	print        bool   // print song metadata
	printFull    bool   // print song metadata with SHA1 and length
	scan         bool   // scan songs for updated metadata
	setAlbumID   string // release MBID to update songs to
	setNonAlbum  bool   // update songs to be non-album tracks
	recordingIDs string // comma-separated list of recording MBIDs
}

func (*Command) Name() string     { return "metadata" }
func (*Command) Synopsis() string { return "update song metadata" }
func (*Command) Usage() string {
	return `metadata <flags> <path>...:
	Fetch updated metadata from MusicBrainz and write override files.
	-scan updates the specified songs or all songs (without positional arguments).
	-set-album changes the album of the specified files or directories.
	-set-non-album updates the specified song file(s) to be non-album tracks.
	-print prints current on-disk metadata for the specified file(s).
	-print-full additionally includes SHA1s and lengths.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&cmd.opts.dryRun, "dry-run", false, "Don't write override files")
	f.BoolVar(&cmd.opts.logUpdates, "log-updates", true, "Log updates to stdout")
	f.BoolVar(&cmd.print, "print", false, "Print metadata from specified song file(s)")
	f.BoolVar(&cmd.printFull, "print-full", false, "Like -print, but include SHA1 and length (slower)")
	f.StringVar(&cmd.recordingIDs, "recordings", "", "Comma-separated list of recording ID overrides for -set-album")
	f.BoolVar(&cmd.scan, "scan", false, "Scan songs for updated metadata")
	f.StringVar(&cmd.setAlbumID, "set-album", "", "Update MusicBrainz release ID for specified files/dirs")
	f.BoolVar(&cmd.setNonAlbum, "set-non-album", false, "Update specified file(s) to be non-album tracks")
}

func (cmd *Command) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	api := newAPI("https://musicbrainz.org")
	cmd.Cfg.ComputeGain = false // no need to compute gains

	// TODO: Fail if multiple actions are specified.
	switch {
	case cmd.print, cmd.printFull:
		return cmd.doPrint(fs.Args())
	case cmd.scan:
		return cmd.doScan(ctx, api, fs.Args())
	case cmd.setAlbumID != "":
		return cmd.doSetAlbum(ctx, api, fs.Args())
	case cmd.setNonAlbum:
		return cmd.doSetNonAlbum(ctx, api, fs.Args())
	default:
		fmt.Fprintln(os.Stderr, "No action specified")
		return subcommands.ExitUsageError
	}
}

// doPrint prints metadata for the specified song files.
func (cmd *Command) doPrint(args []string) subcommands.ExitStatus {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "No song files specified")
		return subcommands.ExitUsageError
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	for _, p := range args {
		var flags files.ReadSongFlag
		if !cmd.printFull {
			flags |= files.SkipAudioData
		}
		s, err := files.ReadSong(cmd.Cfg, p, nil, flags, nil /* gc */)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed reading song:", err)
			return subcommands.ExitFailure
		}
		var date string
		if !s.Date.IsZero() {
			date = s.Date.Format("2006-01-02")
		}
		// Use a custom struct instead of db.Song so we can choose which fields get printed.
		enc.Encode(struct {
			SHA1            string  `json:"sha1,omitempty"`
			Filename        string  `json:"filename"`
			Artist          string  `json:"artist"`
			Title           string  `json:"title"`
			Album           string  `json:"album"`
			AlbumArtist     string  `json:"albumArtist"`
			DiscSubtitle    string  `json:"discSubtitle"`
			AlbumID         string  `json:"albumId"`
			OrigAlbumID     string  `json:"origAlbumId"`
			RecordingID     string  `json:"recordingId"`
			OrigRecordingID string  `json:"origRecordingId"`
			Track           int     `json:"track"`
			Disc            int     `json:"disc"`
			Date            string  `json:"date"`
			Length          float64 `json:"length,omitempty"`
		}{
			SHA1:            s.SHA1,
			Filename:        s.Filename,
			Artist:          s.Artist,
			Title:           s.Title,
			Album:           s.Album,
			AlbumArtist:     s.AlbumArtist,
			DiscSubtitle:    s.DiscSubtitle,
			AlbumID:         s.AlbumID,
			OrigAlbumID:     s.OrigAlbumID,
			RecordingID:     s.RecordingID,
			OrigRecordingID: s.OrigRecordingID,
			Track:           s.Track,
			Disc:            s.Disc,
			Date:            date,
			Length:          s.Length,
		})
	}
	return subcommands.ExitSuccess
}

// doScan scans for updated metadata with the supplied positional args.
func (cmd *Command) doScan(ctx context.Context, api *api, args []string) subcommands.ExitStatus {
	var errMsgs []string
	if len(args) > 0 {
		for _, p := range args {
			if err := scanSong(ctx, cmd.Cfg, api, p, nil /* fi */, &cmd.opts); err != nil {
				errMsgs = append(errMsgs, fmt.Sprintf("%v: %v", p, err))
			}
		}
	} else {
		if len(cmd.Cfg.MusicDir) == 0 {
			fmt.Fprintln(os.Stderr, "musicDir not set in config")
			return subcommands.ExitUsageError
		}
		if err := filepath.Walk(cmd.Cfg.MusicDir, func(p string, fi os.FileInfo, err error) error {
			if fi.Mode().IsRegular() && files.IsMusicPath(p) {
				if err := scanSong(ctx, cmd.Cfg, api, p, fi, &cmd.opts); err != nil {
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

func (cmd *Command) doSetAlbum(ctx context.Context, api *api, paths []string) subcommands.ExitStatus {
	// Read the songs from disk first.
	var songs []*db.Song
	for _, p := range paths {
		ps, err := readAlbumSongs(cmd.Cfg, p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed reading songs:", err)
			return subcommands.ExitFailure
		}
		songs = append(songs, ps...)
	}
	if len(songs) == 0 {
		fmt.Fprintln(os.Stderr, "No songs found")
		return subcommands.ExitUsageError
	}

	// Fetch the new release from MusicBrainz.
	rel, err := api.getRelease(ctx, cmd.setAlbumID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed fetching release %v: %v\n", cmd.setAlbumID, err)
		return subcommands.ExitFailure
	}

	// Map the songs to the album.
	var recordingIDs []string
	if cmd.recordingIDs != "" {
		recordingIDs = strings.Split(cmd.recordingIDs, ",")
	}
	updated, err := setAlbum(songs, rel, recordingIDs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed setting album:", err)
		return subcommands.ExitFailure
	}

	// Save the updates.
	for i, orig := range songs {
		up := updated[i]
		if orig.MetadataEquals(up) {
			continue
		}
		if cmd.opts.logUpdates {
			fmt.Println(orig.Filename + "\n" + db.DiffSongs(orig, up) + "\n")
		}
		if !cmd.opts.dryRun {
			if err := files.UpdateMetadataOverride(cmd.Cfg, up); err != nil {
				fmt.Fprintln(os.Stderr, "Failed writing override file:", err)
				return subcommands.ExitFailure
			}
		}
	}

	return subcommands.ExitSuccess
}

// doSetNonAlbum updates the specified song files to be non-album tracks.
func (cmd *Command) doSetNonAlbum(ctx context.Context, api *api, paths []string) subcommands.ExitStatus {
	for _, p := range paths {
		orig, err := files.ReadSong(cmd.Cfg, p, nil, files.SkipAudioData, nil /* gc */)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed reading songs:", err)
			return subcommands.ExitFailure
		}

		updated := *orig
		updated.Album = files.NonAlbumTracksValue
		updated.AlbumID = ""

		if orig.MetadataEquals(&updated) {
			continue
		}
		if cmd.opts.logUpdates {
			fmt.Println(orig.Filename + "\n" + db.DiffSongs(orig, &updated) + "\n")
		}
		if !cmd.opts.dryRun {
			if err := files.UpdateMetadataOverride(cmd.Cfg, &updated); err != nil {
				fmt.Fprintln(os.Stderr, "Failed writing override file:", err)
				return subcommands.ExitFailure
			}
		}
	}

	return subcommands.ExitSuccess
}

// updateOptions configures how songs are updated.
type updateOptions struct {
	dryRun     bool // don't actually write override files
	logUpdates bool // print song updates to stdout
}

// scanSong reads the song file at p, fetches updated metadata using api,
// and writes a metadata override file if needed. p and fi are passed to files.ReadSong.
func scanSong(ctx context.Context, cfg *client.Config, api *api,
	p string, fi os.FileInfo, opts *updateOptions) error {
	if opts == nil {
		opts = &updateOptions{}
	}
	orig, err := files.ReadSong(cfg, p, fi, files.SkipAudioData, nil /* gc */)
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

	if opts.logUpdates {
		fmt.Println(orig.Filename + "\n" + db.DiffSongs(orig, updated) + "\n")
	}
	if opts.dryRun {
		return nil
	}
	return files.UpdateMetadataOverride(cfg, updated)
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

// readAlbumSongs reads p, a song file or a directory containing song files from a
// single album. Directory entries are sorted by ascinding disc and track, and
// SHA1s and lengths are computed.
func readAlbumSongs(cfg *client.Config, p string) ([]*db.Song, error) {
	// If the path is a file, read it directly.
	if fi, err := os.Stat(p); err != nil {
		return nil, err
	} else if !fi.IsDir() {
		if s, err := files.ReadSong(cfg, p, nil, 0, nil /* gc */); err != nil {
			return nil, err
		} else {
			return []*db.Song{s}, nil
		}
	}

	// Otherwise, process all the songs in the directory.
	dir := p
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fis, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	var albumID string
	var songs []*db.Song
	for _, fi := range fis {
		p := filepath.Join(dir, fi.Name())
		if !fi.Mode().IsRegular() || !files.IsMusicPath(p) {
			continue
		}
		s, err := files.ReadSong(cfg, p, nil, 0, nil /* gc */)
		if err != nil {
			return nil, fmt.Errorf("%v: %v", p, err)
		}
		if s.RecordingID == "" {
			return nil, fmt.Errorf("%q lacks recording ID", s.Filename)
		} else if s.AlbumID == "" {
			return nil, fmt.Errorf("%q lacks album ID", s.Filename)
		} else if albumID == "" {
			albumID = s.AlbumID
		} else if s.AlbumID != albumID {
			return nil, fmt.Errorf("%q has album ID %v but saw %v in same dir", s.Filename, s.AlbumID, albumID)
		}
		songs = append(songs, s)
	}

	sort.Slice(songs, func(i, j int) bool {
		si, sj := songs[i], songs[j]
		if si.Disc < sj.Disc {
			return true
		} else if sj.Disc < si.Disc {
			return false
		}
		return si.Track < sj.Track
	})

	return songs, nil
}

// setAlbum returns a shallow copy of the supplied songs with their album (and other metadata)
// switched to rel. An error is returned if the songs can't be mapped to the new album.
// recordingIDs may be used to override songs' recording IDs before matching.
func setAlbum(songs []*db.Song, rel *release, recordingIDs []string) ([]*db.Song, error) {
	trackCountsMatch := len(songs) == rel.numTracks()
	updated := make([]*db.Song, len(songs))
	for i, s := range songs {
		cp := *s
		updated[i] = &cp

		// Override the song's recording ID if requested.
		if i < len(recordingIDs) && recordingIDs[i] != "" {
			cp.RecordingID = recordingIDs[i]
		}

		// First, try to match the song by recording ID.
		// TODO: Is this safe, or would it be better to also compare the lengths here?
		if updateSongFromRelease(&cp, rel) {
			continue
		}

		// Otherwise, use the track in the same position if it's around the same length.
		if !trackCountsMatch {
			return nil, fmt.Errorf("%q has unmatched recording %v", s.Filename, s.RecordingID)
		}
		tr := rel.getTrackByIndex(i) // should succeed since track counts match
		slen := time.Duration(s.Length * float64(time.Second))
		tlen := time.Duration(tr.Length) * time.Millisecond
		if absDur(slen-tlen) > maxSongLengthDiff {
			return nil, fmt.Errorf("%q length %v is too different from track %q length %v", s.Filename, slen, tr.Title, tlen)
		}
		cp.RecordingID = tr.Recording.ID
		if !updateSongFromRelease(&cp, rel) {
			return nil, fmt.Errorf("unable to find %q (recording %v) in new release", s.Filename, s.RecordingID)
		}
	}

	// Make sure that recordings don't get used for multiple songs.
	recs := make(map[string]string, len(updated)) // recording ID to filename
	for _, s := range updated {
		if fn, ok := recs[s.RecordingID]; ok {
			return nil, fmt.Errorf("recording %v used for both %q and %q", s.RecordingID, fn, s.Filename)
		}
		recs[s.RecordingID] = s.Filename
	}

	return updated, nil
}

// TODO: Use time.Duration.Abs once I can switch to the go119 runtime.
func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
