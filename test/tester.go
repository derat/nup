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
	"strconv"
	"strings"
	"time"

	"erat.org/nup"
)

const (
	dumpBatchSize    = 2
	androidBatchSize = 1
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

	updateConfigFile string
	dumpConfigFile   string
	serverUrl        string
	binDir           string
	client           http.Client
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

	writeConfig := func(fn string, d interface{}) (path string) {
		path = filepath.Join(t.TempDir, fn)
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		if err = json.NewEncoder(f).Encode(d); err != nil {
			panic(err)
		}
		return path
	}

	t.updateConfigFile = writeConfig("update_config.json", struct {
		LastUpdateTimeFile string
		ServerUrl          string
		Username           string
		Password           string
		CoverDir           string
		MusicDir           string
	}{
		filepath.Join(t.TempDir, "last_update_time"),
		t.serverUrl,
		TestUsername,
		TestPassword,
		t.CoverDir,
		t.MusicDir,
	})

	t.dumpConfigFile = writeConfig("dump_config.json", struct {
		ServerUrl string
		Username  string
		Password  string
	}{
		t.serverUrl,
		TestUsername,
		TestPassword,
	})

	return t
}

func (t *Tester) DumpSongs(stripIds bool) []nup.Song {
	stdout, stderr, err := runCommand(filepath.Join(t.binDir, "dump_music"), "-config="+t.dumpConfigFile,
		"-song-batch-size="+strconv.Itoa(dumpBatchSize), "-play-batch-size="+strconv.Itoa(dumpBatchSize))
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

func (t *Tester) GetSongId(sha1 string) string {
	for _, s := range t.DumpSongs(false) {
		if s.Sha1 == sha1 {
			return s.SongId
		}
	}
	panic(fmt.Sprintf("failed to find ID for %v", sha1))
}

func (t *Tester) ImportSongsFromLegacyDb(path string) {
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"), "-config="+t.updateConfigFile, "-import-db="+path); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) UpdateSongs() {
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"), "-config="+t.updateConfigFile); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) NewRequest(method, path string, body io.Reader) *http.Request {
	req, err := http.NewRequest(method, t.serverUrl+path, body)
	if err != nil {
		panic(err)
	}
	req.SetBasicAuth(TestUsername, TestPassword)
	return req
}

func (t *Tester) SendRequest(req *http.Request) *http.Response {
	resp, err := t.client.Do(req)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != http.StatusOK {
		panic(resp.Status)
	}
	return resp
}

func (t *Tester) DoPost(pathAndQueryParams string, body io.Reader) {
	req := t.NewRequest("POST", pathAndQueryParams, body)
	req.Header.Set("Content-Type", "text/plain")
	resp := t.SendRequest(req)
	resp.Body.Close()
}

func (t *Tester) PostSongs(songs []nup.Song, replaceUserData bool, updateDelay time.Duration) {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	for _, s := range songs {
		if err := e.Encode(s); err != nil {
			panic(err)
		}
	}
	path := fmt.Sprintf("import?updateDelayNsec=%v", int64(updateDelay*time.Nanosecond))
	if replaceUserData {
		path += "&replaceUserData=1"
	}
	t.DoPost(path, &buf)
}

func (t *Tester) QuerySongs(params string) []nup.Song {
	resp := t.SendRequest(t.NewRequest("GET", "query?"+params, nil))
	defer resp.Body.Close()

	songs := make([]nup.Song, 0)
	d := json.NewDecoder(resp.Body)
	if err := d.Decode(&songs); err != nil {
		panic(err)
	}
	return songs
}

func (t *Tester) GetNowFromServer() time.Time {
	resp := t.SendRequest(t.NewRequest("GET", "now_nsec", nil))
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	nsec, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		panic(err)
	} else if nsec <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nsec)
}

func (t *Tester) GetSongsForAndroid(minLastModified time.Time) []nup.Song {
	var nsec int64
	if !minLastModified.IsZero() {
		nsec = minLastModified.UnixNano()
	}

	songs := make([]nup.Song, 0)
	var cursor string

	for {
		path := fmt.Sprintf("songs?minLastModifiedNsec=%d&max=%d", nsec, androidBatchSize)
		if len(cursor) > 0 {
			path += "&cursor=" + cursor
		}

		resp := t.SendRequest(t.NewRequest("GET", path, nil))
		defer resp.Body.Close()

		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		var items []json.RawMessage
		if err = json.Unmarshal(data, &items); err != nil && err != io.EOF {
			panic(err)
		}

		cursor = ""
		for _, item := range items {
			s := nup.Song{}
			if err = json.Unmarshal(item, &s); err == nil {
				songs = append(songs, s)
			} else if err := json.Unmarshal(item, &cursor); err != nil {
				panic(err)
			}
		}

		if len(cursor) == 0 {
			break
		}
	}

	return songs
}
