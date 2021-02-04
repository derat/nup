// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/derat/nup/internal/pkg/mp3gain"
	"github.com/derat/nup/internal/pkg/types"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage %v: [flag]...\n"+
			"Computes gain adjustments for songs.\n"+
			"Unmarshals \"dump_music\" song objects from stdin and\n"+
			"marshals updated objects to stdout.\n\n",
			os.Args[0])
		flag.PrintDefaults()
	}
	musicDir := flag.String("music-dir", filepath.Join(os.Getenv("HOME"), "music"),
		"Directory containing song files")
	flag.Parse()

	// Read the full song listing and group by album.
	albumSongs := make(map[string][]*types.Song) // album IDs to songs in album
	d := json.NewDecoder(os.Stdin)
	for {
		var s types.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal("Failed to read song: ", err)
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
			paths[i] = filepath.Join(*musicDir, s.Filename)
		}
		gains, err := mp3gain.ComputeAlbum(paths)
		if err != nil {
			log.Fatalf("Failed to compute gains for %v (%q): %v", id, paths[0], err)
		}
		for i, s := range songs {
			path := paths[i]
			info, ok := gains[path]
			if !ok {
				log.Fatalf("Didn't get gain info for %v", path)
			}
			s.TrackGain = info.TrackGain
			s.AlbumGain = info.AlbumGain
			s.PeakAmp = info.PeakAmp
			if err := enc.Encode(*s); err != nil {
				log.Fatalf("Failed encoding %v: %v", path, err)
			}
		}
	}
}
