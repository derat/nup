package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
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

func main() {
	server := flag.String("server", "http://localhost:8080/", "URL of running dev_appengine server")
	binDir := flag.String("binary-dir", filepath.Join(os.Getenv("GOPATH"), "bin"), "Directory containing executables")
	flag.Parse()

	buildBinaries()

	t := newTester(*server, *binDir)
	defer os.RemoveAll(t.TempDir)

	log.Print("dumping initial songs")
	songs := t.DumpSongs(false)
	if len(songs) != 0 {
		log.Fatalf("server at %v isn't empty; got %v song(s)", *server, len(songs))
	}

	log.Print("importing 2 songs")
	test.CopySongsToTempDir(t.MusicDir, test.Song0s.Filename, test.Song1s.Filename)
	t.UpdateSongs()

	log.Print("sleeping and dumping songs")
	time.Sleep(time.Second)
	songs = t.DumpSongs(true)
	if err := test.CompareSongs([]nup.Song{test.Song0s, test.Song1s}, songs, false); err != nil {
		log.Fatal(err)
	}

	log.Print("importing another song")
	test.CopySongsToTempDir(t.MusicDir, test.Song5s.Filename)
	t.UpdateSongs()

	log.Print("sleeping and dumping songs")
	time.Sleep(time.Second)
	songs = t.DumpSongs(true)
	if err := test.CompareSongs([]nup.Song{test.Song0s, test.Song1s, test.Song5s}, songs, false); err != nil {
		log.Fatal(err)
	}
}
