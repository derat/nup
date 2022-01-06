// Copyright 2021 Daniel Erat.
// All rights reserved.

package gain

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/mp3gain"
	"github.com/derat/nup/server/db"
	"github.com/google/subcommands"
)

type Command struct {
	Cfg *client.Config
}

func (*Command) Name() string     { return "gain" }
func (*Command) Synopsis() string { return "compute gain adjustments for songs" }
func (*Command) Usage() string {
	return `gain:
	Compute gain adjustments for dumped songs from stdin using mp3gain.
	Writes updated JSON song objects to stdout.

`
}
func (cmd *Command) SetFlags(f *flag.FlagSet) {}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	// Read the full song listing and group by album.
	albumSongs := make(map[string][]*db.Song) // album IDs to songs in album
	d := json.NewDecoder(os.Stdin)
	for {
		var s db.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to read song:", err)
			return subcommands.ExitFailure
		}

		// Clear some unneeded data.
		s.Plays = nil
		s.Tags = nil

		id := s.AlbumID
		if id == "" {
			id = s.Filename
		}
		albumSongs[id] = append(albumSongs[id], &s)
	}

	// Sort album IDs, and also sort songs within each album.
	ids := make([]string, 0, len(albumSongs)) // album IDs sorted by path
	for id, songs := range albumSongs {
		ids = append(ids, id)
		sort.Slice(songs, func(i, j int) bool { return songs[i].Filename < songs[j].Filename })
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		return albumSongs[a][0].Filename < albumSongs[b][0].Filename
	})

	// For each album, compute gain adjustments and marshal songs to stdout.
	enc := json.NewEncoder(os.Stdout)
	for _, id := range ids {
		songs := albumSongs[id]
		paths := make([]string, len(songs))
		for i, s := range songs {
			paths[i] = filepath.Join(cmd.Cfg.MusicDir, s.Filename)
		}
		gains, err := mp3gain.ComputeAlbum(paths)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to compute gains for %v (%q): %v\n", id, paths[0], err)
			return subcommands.ExitFailure
		}
		for i, s := range songs {
			path := paths[i]
			info, ok := gains[path]
			if !ok {
				fmt.Fprintln(os.Stderr, "Didn't get gain info for", path)
				return subcommands.ExitFailure
			}
			s.TrackGain = info.TrackGain
			s.AlbumGain = info.AlbumGain
			s.PeakAmp = info.PeakAmp
			if err := enc.Encode(*s); err != nil {
				fmt.Fprintf(os.Stderr, "Failed encoding %v: %v\n", path, err)
				return subcommands.ExitFailure
			}
		}
	}
	return subcommands.ExitSuccess
}
