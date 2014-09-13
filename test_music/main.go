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
	songs, err := t.DumpSongs(false)
	if err != nil {
		log.Fatalf("dumping songs failed: %v", err)
	} else if len(songs) != 0 {
		log.Fatalf("server at %v isn't empty; got %v song(s)", *server, len(songs))
	}

	log.Print("importing 2 songs")
	test.CopySongsToTempDir(t.MusicDir, test.Song0s.Filename, test.Song1s.Filename)
	if err = t.UpdateSongs(); err != nil {
		log.Fatalf("importing songs failed: %v", err)
	}

	log.Print("sleeping and dumping songs")
	time.Sleep(time.Second)
	if songs, err = t.DumpSongs(true); err != nil {
		log.Fatalf("dumping songs failed: %v", err)
	}
	if err = test.CompareSongs([]nup.Song{test.Song0s, test.Song1s}, songs, false); err != nil {
		log.Fatal(err)
	}
}
