// Copyright 2020 Daniel Erat.
// All rights reserved.

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

	"github.com/derat/nup/server/types"
)

const (
	dumpBatchSize    = 2
	androidBatchSize = 1
)

type userDataPolicy int

const (
	replaceUserData userDataPolicy = iota
	keepUserData
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
	serverURL        string
	binDir           string
	client           http.Client
}

func newTester(serverURL, binDir string) *Tester {
	t := &Tester{
		TempDir:   CreateTempDir(),
		serverURL: serverURL,
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

	// Corresponds to Config in cmd/update_music/main.go.
	t.updateConfigFile = writeConfig("update_config.json", struct {
		LastUpdateTimeFile string `json:"lastUpdateTimeFile"`
		ServerURL          string `json:"serverUrl"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		CoverDir           string `json:"coverDir"`
		MusicDir           string `json:"musicDir"`
	}{
		filepath.Join(t.TempDir, "last_update_time"),
		t.serverURL,
		types.TestUsername,
		types.TestPassword,
		t.CoverDir,
		t.MusicDir,
	})

	t.dumpConfigFile = writeConfig("dump_config.json", types.ClientConfig{
		ServerURL: t.serverURL,
		Username:  types.TestUsername,
		Password:  types.TestPassword,
	})

	return t
}

func (t *Tester) CleanUp() {
	os.RemoveAll(t.TempDir)
}

type stripPolicy int

const (
	stripIDs stripPolicy = iota
	keepIDs
)

type coverPolicy int

const (
	skipCovers coverPolicy = iota
	getCovers
)

func (t *Tester) DumpSongs(strip stripPolicy, covers coverPolicy) []types.Song {
	coversValue := "false"
	if covers == getCovers {
		coversValue = "true"
	}

	stdout, stderr, err := runCommand(filepath.Join(t.binDir, "dump_music"),
		"-config="+t.dumpConfigFile,
		"-song-batch-size="+strconv.Itoa(dumpBatchSize),
		"-play-batch-size="+strconv.Itoa(dumpBatchSize),
		"-covers="+coversValue)
	if err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
	songs := make([]types.Song, 0)

	if len(stdout) == 0 {
		return songs
	}

	for _, l := range strings.Split(strings.TrimSpace(stdout), "\n") {
		s := types.Song{}
		if err = json.Unmarshal([]byte(l), &s); err != nil {
			if err == io.EOF {
				break
			}
			panic(fmt.Sprintf("unable to unmarshal song %q: %v", l, err))
		}
		if strip == stripIDs {
			s.SongID = ""
		}
		songs = append(songs, s)
	}
	return songs
}

func (t *Tester) SongID(sha1 string) string {
	for _, s := range t.DumpSongs(keepIDs, skipCovers) {
		if s.SHA1 == sha1 {
			return s.SongID
		}
	}
	panic(fmt.Sprintf("failed to find ID for %v", sha1))
}

func (t *Tester) ImportSongsFromJSONFile(path string, policy userDataPolicy) {
	userDataValue := "false"
	if policy == replaceUserData {
		userDataValue = "true"
	}
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"),
		"-config="+t.updateConfigFile,
		"-import-json-file="+path,
		"-import-user-data="+userDataValue); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) UpdateSongs() {
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"),
		"-config="+t.updateConfigFile); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) DeleteSong(songID string) {
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"),
		"-config="+t.updateConfigFile,
		"-delete-song-id="+songID); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) NewRequest(method, path string, body io.Reader) *http.Request {
	req, err := http.NewRequest(method, t.serverURL+path, body)
	if err != nil {
		panic(err)
	}
	req.SetBasicAuth(types.TestUsername, types.TestPassword)
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

func (t *Tester) PingServer() error {
	if resp, err := t.client.Do(t.NewRequest("HEAD", "", nil)); err != nil {
		return err
	} else {
		resp.Body.Close()
		return nil
	}
}

func (t *Tester) DoPost(pathAndQueryParams string, body io.Reader) {
	req := t.NewRequest("POST", pathAndQueryParams, body)
	req.Header.Set("Content-Type", "text/plain")
	resp := t.SendRequest(req)
	defer resp.Body.Close()
	if _, err := ioutil.ReadAll(resp.Body); err != nil {
		panic(err)
	}
}

func (t *Tester) PostSongs(songs []types.Song, userData userDataPolicy, updateDelay time.Duration) {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	for _, s := range songs {
		if err := e.Encode(s); err != nil {
			panic(err)
		}
	}
	path := fmt.Sprintf("import?updateDelayNsec=%v", int64(updateDelay*time.Nanosecond))
	if userData == replaceUserData {
		path += "&replaceUserData=1"
	}
	t.DoPost(path, &buf)
}

func (t *Tester) QuerySongs(params ...string) []types.Song {
	resp := t.SendRequest(t.NewRequest("GET", "query?"+strings.Join(params, "&"), nil))
	defer resp.Body.Close()

	songs := make([]types.Song, 0)
	d := json.NewDecoder(resp.Body)
	if err := d.Decode(&songs); err != nil {
		panic(err)
	}
	return songs
}

func (t *Tester) GetTags(requireCache bool) string {
	path := "tags"
	if requireCache {
		path += "?requireCache=1"
	}
	resp := t.SendRequest(t.NewRequest("GET", path, nil))
	defer resp.Body.Close()

	tags := make([]string, 0)
	d := json.NewDecoder(resp.Body)
	if err := d.Decode(&tags); err != nil {
		panic(err)
	}
	return strings.Join(tags, ",")
}

func (t *Tester) GetNowFromServer() time.Time {
	resp := t.SendRequest(t.NewRequest("GET", "now", nil))
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

type deletionPolicy int

const (
	getRegularSongs deletionPolicy = iota
	getDeletedSongs
)

func (t *Tester) GetSongsForAndroid(minLastModified time.Time, deleted deletionPolicy) []types.Song {
	params := []string{
		"type=song",
		"max=" + strconv.Itoa(androidBatchSize),
		"omit=plays,sha1",
	}
	if deleted == getDeletedSongs {
		params = append(params, "deleted=1")
	}
	if !minLastModified.IsZero() {
		params = append(params, fmt.Sprintf("minLastModifiedNsec=%d", minLastModified.UnixNano()))
	}

	songs := make([]types.Song, 0)
	var cursor string

	for {
		path := "export?" + strings.Join(params, "&")
		if cursor != "" {
			path += "&cursor=" + cursor
		}

		resp := t.SendRequest(t.NewRequest("GET", path, nil))
		defer resp.Body.Close()

		// We receive a sequence of marshaled songs optionally followed by a cursor.
		cursor = ""
		dec := json.NewDecoder(resp.Body)
		for {
			var msg json.RawMessage
			if err := dec.Decode(&msg); err == io.EOF {
				break
			} else if err != nil {
				panic(err)
			}

			var s types.Song
			if err := json.Unmarshal(msg, &s); err == nil {
				songs = append(songs, s)
			} else if err := json.Unmarshal(msg, &cursor); err == nil {
				break
			} else {
				panic(err)
			}
		}

		if cursor == "" {
			break
		}
	}

	return songs
}
