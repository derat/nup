// Copyright 2023 Daniel Erat.
// All rights reserved.

package scan

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
}

func (*Command) Name() string     { return "scan" }
func (*Command) Synopsis() string { return "scan songs for updated metadata" }
func (*Command) Usage() string {
	return `scan [flags]:
	Scan songs for updated metadata using MusicBrainz.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
}

func (cmd *Command) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, cmd.Usage())
		return subcommands.ExitUsageError
	}
	if len(cmd.Cfg.MusicDir) == 0 {
		fmt.Fprintln(os.Stderr, "musicDir not set in config")
		return subcommands.ExitUsageError
	}

	api := newAPI("https://musicbrainz.org")

	type songOrErr struct {
		song *db.Song
		path string // relative to cmd.Cfg.MusicDir
		err  error
	}
	ch := make(chan songOrErr, songChanSize)

	go func() {
		if err := filepath.Walk(cmd.Cfg.MusicDir, func(p string, fi os.FileInfo, err error) error {
			if fi.Mode().IsRegular() && files.IsMusicPath(p) {
				song, err := files.ReadSong(cmd.Cfg, p, fi, true /* onlyTags */, nil /* gc */)
				ch <- songOrErr{song, p[len(cmd.Cfg.MusicDir)+1:], err}
			}
			return nil
		}); err != nil {
			ch <- songOrErr{nil, "", err}
		}
		close(ch)
	}()

	var rel *release
	for soe := range ch {
		if soe.err != nil {
			log.Printf("%v: %v", soe.path, soe.err)
			continue
		}

		song := soe.song
		mbSong := *song

		switch {
		case song.AlbumID != "":
			if rel == nil || song.AlbumID != rel.ID {
				var err error
				if rel, err = api.getRelease(ctx, song.AlbumID); err != nil {
					log.Printf("%v: %v", soe.path, err)
					continue
				}
			}
			if err := updateSongFromRelease(&mbSong, rel); err != nil {
				log.Printf("%v: %v", soe.path, err)
				continue
			}
		case song.RecordingID != "":
			rec, err := api.getRecording(ctx, song.RecordingID)
			if err != nil {
				log.Printf("%v: %v", soe.path, err)
				continue
			}
			updateSongFromRecording(&mbSong, rec)
		default:
			continue
		}

		if !song.MetadataEquals(&mbSong) {
			fmt.Println(soe.path + "\n" + cmp.Diff(*song, mbSong))
		}
	}

	return subcommands.ExitSuccess
}

// updateSongFromRelease updates fields in song using data from rel.
// An error is returned if the recording isn't included in the release.
func updateSongFromRelease(song *db.Song, rel *release) error {
	tr, med := rel.findTrack(song.RecordingID)
	if tr == nil {
		return fmt.Errorf("recording %q not found", song.RecordingID)
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

	return nil
}

// updateSongFromRecording updates fields in song using data from rec.
// This should only be used for standalone recordings.
func updateSongFromRecording(song *db.Song, rec *recording) {
	song.Artist = joinArtistCredits(rec.Artists)
	song.Title = rec.Title
	song.Album = files.NonAlbumTracksValue
	song.Date = time.Time(rec.FirstReleaseDate) // always zero?
}
