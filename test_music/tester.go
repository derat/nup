package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"erat.org/nup"
)

type Tester struct {
	TempDir    string
	ConfigFile string
	ServerUrl  string
	BinDir     string
}

func newTester(serverUrl, binDir string) *Tester {
	t := &Tester{
		ServerUrl: serverUrl,
		BinDir:    binDir,
	}

	var err error
	if t.TempDir, err = ioutil.TempDir("", "test_music."); err != nil {
		panic(err)
	}

	t.ConfigFile = filepath.Join(t.TempDir, "config.json")
	f, err := os.Create(t.ConfigFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if err = json.NewEncoder(f).Encode(struct {
		LastUpdateTimeFile string
		ServerUrl          string
	}{filepath.Join(t.TempDir, "last_update_time"), t.ServerUrl}); err != nil {
		panic(err)
	}

	return t
}

func (t *Tester) DumpSongs() ([]nup.Song, error) {
	stdout, stderr, err := runCommand(filepath.Join(t.BinDir, "dump_music"), "-config="+t.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("dumping songs failed: %v\nstderr: %v", err, stderr)
	}
	songs := make([]nup.Song, 0)

	if len(stdout) == 0 {
		return songs, nil
	}

	for _, l := range strings.Split(stdout, "\n") {
		s := nup.Song{}
		if err = json.Unmarshal([]byte(l), &s); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("unable to unmarshal song %q: %v", l, err)
		}
		songs = append(songs, s)
	}
	return songs, nil
}
