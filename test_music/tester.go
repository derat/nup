package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"erat.org/nup"
	"erat.org/nup/test"
)

type Tester struct {
	TempDir  string
	MusicDir string
	CoverDir string

	configFile string
	serverUrl  string
	binDir     string
}

func newTester(serverUrl, binDir string) *Tester {
	t := &Tester{
		TempDir:   test.CreateTempDir(),
		serverUrl: serverUrl,
		binDir:    binDir,
	}

	t.MusicDir = filepath.Join(t.TempDir, "music")
	t.CoverDir = filepath.Join(t.MusicDir, ".covers")
	if err := os.MkdirAll(t.CoverDir, 0700); err != nil {
		panic(err)
	}

	t.configFile = filepath.Join(t.TempDir, "config.json")
	f, err := os.Create(t.configFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	if err = json.NewEncoder(f).Encode(struct {
		LastUpdateTimeFile string
		ServerUrl          string
	}{filepath.Join(t.TempDir, "last_update_time"), t.serverUrl}); err != nil {
		panic(err)
	}

	return t
}

func (t *Tester) DumpSongs(stripIds bool) []nup.Song {
	stdout, stderr, err := runCommand(filepath.Join(t.binDir, "dump_music"), "-config="+t.configFile)
	if err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
	songs := make([]nup.Song, 0)

	if len(stdout) == 0 {
		return songs
	}

	for _, l := range strings.Split(strings.TrimSpace(stdout), "\n") {
		s := nup.Song{}
		if err = json.Unmarshal([]byte(l), &s); err != nil {
			if err == io.EOF {
				break
			}
			panic(fmt.Sprintf("unable to unmarshal song %q: %v", l, err))
		}
		if stripIds {
			s.SongId = ""
		}
		songs = append(songs, s)
	}
	return songs
}

func (t *Tester) UpdateSongs() {
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"), "-config="+t.configFile, "-music-dir="+t.MusicDir, "-cover-dir="+t.CoverDir); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) QuerySongs(params string) []nup.Song {
	resp, err := http.Get(t.serverUrl + "query?" + params)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		panic(resp.Status)
	}
	defer resp.Body.Close()

	songs := make([]nup.Song, 0)
	d := json.NewDecoder(resp.Body)
	if err = d.Decode(&songs); err != nil {
		panic(err)
	}
	return songs
}

func (t *Tester) DoPost(pathAndQueryParams string) {
	resp, err := http.Post(t.serverUrl+pathAndQueryParams, "text/plain", nil)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		panic(resp.Status)
	}
	resp.Body.Close()
}
