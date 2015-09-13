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

	"erat.org/nup"
)

type checker func(s *nup.Song) error

type songArray []*nup.Song

func (a songArray) Len() int           { return len(a) }
func (a songArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a songArray) Less(i, j int) bool { return a[i].Filename < a[j].Filename }

func main() {
	musicDir := flag.String("music-dir", filepath.Join(os.Getenv("HOME"), "music"), "Directory containing music files")
	coverDir := flag.String("cover-dir", "", "Directory containing cover art (\".covers\" within --music-dir if unset)")
	strict := flag.Bool("strict", false, "Check that all songs are tagged and have cover art)")
	flag.Parse()

	if len(*coverDir) == 0 {
		*coverDir = filepath.Join(*musicDir, ".covers")
	}

	d := json.NewDecoder(os.Stdin)
	songs := make([]*nup.Song, 0)
	for {
		var s nup.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal("Failed to read song: %v", err)
		}
		songs = append(songs, &s)
	}
	log.Printf("Read %d songs", len(songs))

	sort.Sort(songArray(songs))

	checkMusicFile := func(s *nup.Song) error {
		if len(s.Filename) == 0 {
			return fmt.Errorf("no filename")
		} else if _, err := os.Stat(filepath.Join(*musicDir, s.Filename)); err != nil {
			return fmt.Errorf("missing file %s", s.Filename)
		}
		return nil
	}
	checkAlbumInfo := func(s *nup.Song) error {
		if *strict && len(s.AlbumId) == 0 && s.Album != "[non-album tracks]" {
			return fmt.Errorf("missing MusicBrainz album")
		}
		return nil
	}
	checkCover := func(s *nup.Song) error {
		if len(s.CoverFilename) == 0 {
			if *strict {
				return fmt.Errorf("no cover file")
			} else {
				return nil
			}
		} else if _, err := os.Stat(filepath.Join(*coverDir, s.CoverFilename)); err != nil {
			return fmt.Errorf("missing cover file %s", s.CoverFilename)
		}
		return nil
	}

	checkers := []checker{
		checkMusicFile,
		checkAlbumInfo,
		checkCover,
	}

	for _, c := range checkers {
		for _, s := range songs {
			if err := c(s); err != nil {
				log.Printf("%s (%s-%s): %v", s.SongId, s.Artist, s.Title, err)
			}
		}
	}
}
