package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"erat.org/nup"
	"erat.org/nup/test"
)

func buildBinaries() {
	log.Printf("rebuilding binaries")
	for _, b := range []string{"dump_music", "update_music"} {
		p := filepath.Join("erat.org/nup", b)
		if _, stderr, err := runCommand("go", "install", p); err != nil {
			panic(stderr)
		}
	}
}

func compareQueryResults(expected, actual []nup.Song, compareOrder bool) error {
	actualCleaned := make([]nup.Song, len(actual))
	for i := range actual {
		s := actual[i]

		if len(s.SongId) == 0 {
			return fmt.Errorf("song %v (%v) has no ID", i, s.Url)
		}
		s.SongId = ""

		if len(s.Tags) == 0 {
			s.Tags = nil
		}
		if i < len(expected) && strings.HasSuffix(s.Url, expected[i].Filename) {
			s.Url = ""
			s.Filename = expected[i].Filename
		}
		actualCleaned[i] = s
	}

	expectedCleaned := make([]nup.Song, len(expected))
	for i := range expected {
		s := expected[i]
		s.Sha1 = ""
		expectedCleaned[i] = s
	}

	return test.CompareSongs(expectedCleaned, actualCleaned, compareOrder)
}

func main() {
	server := flag.String("server", "http://localhost:8080/", "URL of running dev_appengine server")
	binDir := flag.String("binary-dir", filepath.Join(os.Getenv("GOPATH"), "bin"), "Directory containing executables")
	flag.Parse()

	buildBinaries()

	t := newTester(*server, *binDir)
	defer os.RemoveAll(t.TempDir)

	log.Printf("clearing all data on %v", *server)
	t.DoPost("clear")
	time.Sleep(time.Second)

	log.Print("dumping initial songs")
	songs := t.DumpSongs(false)
	if len(songs) != 0 {
		log.Fatalf("server isn't empty; got %v song(s)", len(songs))
	}

	log.Print("importing 2 songs")
	test.CopySongsToTempDir(t.MusicDir, test.Song0s.Filename, test.Song1s.Filename)
	t.UpdateSongs()
	time.Sleep(time.Second)

	log.Print("running query")
	songs = t.QuerySongs("artist=" + url.QueryEscape(test.Song0s.Artist))
	if err := compareQueryResults([]nup.Song{test.Song0s}, songs, true); err != nil {
		log.Fatal(err)
	}

	log.Print("rating and tagging")
	id := songs[0].SongId
	ratedSong0s := test.Song0s
	ratedSong0s.Rating = 0.75
	ratedSong0s.Tags = []string{"electronic", "instrumental"}
	t.DoPost("rate_and_tag?songId=" + id + "&rating=0.75&tags=electronic+instrumental")
	time.Sleep(time.Second)
	songs = t.QuerySongs("album=" + url.QueryEscape(test.Song0s.Album))
	if err := compareQueryResults([]nup.Song{ratedSong0s, test.Song1s}, songs, true); err != nil {
		log.Fatal(err)
	}

	log.Print("clearing tags")
	ratedSong0s.Tags = nil
	t.DoPost("rate_and_tag?songId=" + id + "&tags=")
	time.Sleep(time.Second)
	songs = t.QuerySongs("album=" + url.QueryEscape(test.Song0s.Album))
	if err := compareQueryResults([]nup.Song{ratedSong0s, test.Song1s}, songs, true); err != nil {
		log.Fatal(err)
	}

	log.Print("importing another song")
	test.CopySongsToTempDir(t.MusicDir, test.Song5s.Filename)
	t.UpdateSongs()
	time.Sleep(time.Second)

	log.Print("dumping songs")
	songs = t.DumpSongs(true)
	if err := test.CompareSongs([]nup.Song{ratedSong0s, test.Song1s, test.Song5s}, songs, false); err != nil {
		log.Fatal(err)
	}
}
