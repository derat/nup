package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"erat.org/nup"
)

func runCommand(p string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command(p, args...)
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return
	}
	if err = cmd.Start(); err != nil {
		return
	}

	outBytes, err := ioutil.ReadAll(outPipe)
	if err != nil {
		return
	}
	errBytes, err := ioutil.ReadAll(errPipe)
	if err != nil {
		return
	}
	stdout = string(outBytes)
	stderr = string(errBytes)
	err = cmd.Wait()
	return
}

type Tester struct {
	TempDir  string
	MusicDir string
	CoverDir string

	configFile string
	serverUrl  string
	binDir     string
	client     http.Client
}

func newTester(serverUrl, binDir string) *Tester {
	t := &Tester{
		TempDir:   CreateTempDir(),
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
		Username           string
		Password           string
	}{
		filepath.Join(t.TempDir, "last_update_time"),
		t.serverUrl,
		TestUsername,
		TestPassword,
	}); err != nil {
		panic(err)
	}

	return t
}

func (t *Tester) WaitForUpdate() {
	time.Sleep(2 * time.Second)
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

func (t *Tester) ImportSongsFromLegacyDb(path string) {
	_, caller, _, ok := runtime.Caller(1)
	if !ok {
		panic("unable to get runtime caller info")
	}
	db := filepath.Join(filepath.Dir(caller), path)
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"), "-config="+t.configFile, "-import-db="+db, "-cover-dir="+t.CoverDir); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) UpdateSongs() {
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"), "-config="+t.configFile, "-music-dir="+t.MusicDir, "-cover-dir="+t.CoverDir); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) PostSongs(songs []nup.Song, replaceUserData bool) {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	for _, s := range songs {
		if err := e.Encode(s); err != nil {
			panic(err)
		}
	}
	path := "import"
	if replaceUserData {
		path += "?replaceUserData=1"
	}
	t.DoPost(path, &buf)
}

func (t *Tester) QuerySongs(params string) []nup.Song {
	req, err := http.NewRequest("GET", t.serverUrl+"query?"+params, nil)
	if err != nil {
		panic(err)
	}
	req.SetBasicAuth(TestUsername, TestPassword)

	resp, err := t.client.Do(req)
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

func (t *Tester) DoPost(pathAndQueryParams string, body io.Reader) {
	req, err := http.NewRequest("POST", t.serverUrl+pathAndQueryParams, body)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.SetBasicAuth(TestUsername, TestPassword)

	resp, err := t.client.Do(req)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		panic(resp.Status)
	}
	resp.Body.Close()
}
