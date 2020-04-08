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

	"github.com/derat/nup/types"
)

type songCheckFunction func(s *types.Song) error
type coverCheckFunction func(coverFilename string) error

type songArray []*types.Song

func (a songArray) Len() int           { return len(a) }
func (a songArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a songArray) Less(i, j int) bool { return a[i].Filename < a[j].Filename }

func checkSongs(songs []*types.Song, musicDir, coverDir string, strict bool) {
	checkFile := func(s *types.Song) error {
		if len(s.Filename) == 0 {
			return fmt.Errorf("no filename")
		} else if _, err := os.Stat(filepath.Join(musicDir, s.Filename)); err != nil {
			return fmt.Errorf("missing file %s", s.Filename)
		}
		return nil
	}
	checkAlbumInfo := func(s *types.Song) error {
		if strict && len(s.AlbumId) == 0 && s.Album != "[non-album tracks]" {
			return fmt.Errorf("missing MusicBrainz album")
		}
		return nil
	}
	checkCover := func(s *types.Song) error {
		if len(s.CoverFilename) == 0 {
			if strict {
				return fmt.Errorf("no cover file")
			} else {
				return nil
			}
		} else if _, err := os.Stat(filepath.Join(coverDir, s.CoverFilename)); err != nil {
			return fmt.Errorf("missing cover file %s", s.CoverFilename)
		}
		return nil
	}
	for _, c := range []songCheckFunction{
		checkFile,
		checkAlbumInfo,
		checkCover,
	} {
		for _, s := range songs {
			if err := c(s); err != nil {
				log.Printf("%s (%s-%s): %v", s.SongId, s.Artist, s.Title, err)
			}
		}
	}
}

func checkCovers(songs []*types.Song, coverDir string) {
	dir, err := os.Open(coverDir)
	if err != nil {
		log.Fatal("Failed to open cover dir: ", err)
	}
	defer dir.Close()

	coverFilenames, err := dir.Readdirnames(0)
	if err != nil {
		log.Fatal("Failed to read cover dir: ", err)
	}

	songCoverFilenames := make(map[string]bool)
	for _, s := range songs {
		if len(s.CoverFilename) > 0 {
			songCoverFilenames[s.CoverFilename] = true
		}
	}

	checkUsed := func(coverFilename string) error {
		if _, ok := songCoverFilenames[coverFilename]; !ok {
			return fmt.Errorf("unused cover")
		}
		return nil
	}
	for _, c := range []coverCheckFunction{
		checkUsed,
	} {
		for _, fn := range coverFilenames {
			if err := c(fn); err != nil {
				log.Printf("%s: %v", fn, err)
			}
		}
	}
}

func main() {
	musicDir := flag.String("music-dir", filepath.Join(os.Getenv("HOME"), "music"), "Directory containing music files")
	coverDir := flag.String("cover-dir", "", "Directory containing cover art (\".covers\" within --music-dir if unset)")
	strict := flag.Bool("strict", false, "Check that all songs are tagged and have cover art)")
	flag.Parse()

	if len(*coverDir) == 0 {
		*coverDir = filepath.Join(*musicDir, ".covers")
	}

	d := json.NewDecoder(os.Stdin)
	songs := make([]*types.Song, 0)
	for {
		var s types.Song
		if err := d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			log.Fatal("Failed to read song: ", err)
		}
		songs = append(songs, &s)
	}
	log.Printf("Read %d songs", len(songs))

	sort.Sort(songArray(songs))

	checkSongs(songs, *musicDir, *coverDir, *strict)
	checkCovers(songs, *coverDir)
}
