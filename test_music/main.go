package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
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

	songs, err := t.DumpSongs()
	if err != nil {
		log.Fatalf("failed to dump songs: %v", err)
	} else if len(songs) != 0 {
		log.Fatalf("server at %v isn't empty; got %v songs", *server, len(songs))
	}

	/* scan and import
	   do a query?
	   add file, scan and import
	   export
	*/
}
