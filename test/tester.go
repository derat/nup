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
	dumpBatchSize    = 2 // song/play batch size for dump_music
	androidBatchSize = 1 // song batch size when exporting for Android
)

// runCommand synchronously runs the executable at p with args and returns its output.
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

// Tester helps tests send HTTP requests to a development server and
// run commands like update_music and dump_music.
type Tester struct {
	// TempDir is the base temporary directory created for holding test-related data.
	TempDir string
	// MusicDir is created within TempDir for storing songs.
	MusicDir string
	// CoverDir is created within TempDir for storing album art.
	CoverDir string

	updateConfigFile string // path to update_music config file
	dumpConfigFile   string // path to dump_music config file
	serverURL        string // base URL for dev server
	binDir           string // directory where update_music and dump_music are located
	client           http.Client
}

// newTester creates a new Tester for the development server at serverURL.
// binDir is the directory containing the update_music and dump_music commands
// (e.g. $HOME/go/bin).
func newTester(serverURL, binDir string) *Tester {
	t := &Tester{
		serverURL: serverURL,
		binDir:    binDir,
	}

	var err error
	t.TempDir, err = ioutil.TempDir("", "nup_test.")
	if err != nil {
		panic(err)
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
		LastUpdateInfoFile string `json:"lastUpdateInfoFile"`
		ServerURL          string `json:"serverUrl"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		CoverDir           string `json:"coverDir"`
		MusicDir           string `json:"musicDir"`
		ComputeGain        bool   `json:"computeGain"`
	}{
		filepath.Join(t.TempDir, "last_update_info.json"),
		t.serverURL,
		types.TestUsername,
		types.TestPassword,
		t.CoverDir,
		t.MusicDir,
		true,
	})

	t.dumpConfigFile = writeConfig("dump_config.json", types.ClientConfig{
		ServerURL: t.serverURL,
		Username:  types.TestUsername,
		Password:  types.TestPassword,
	})

	return t
}

// Close deletes temporary files created by the test.
func (t *Tester) Close() {
	os.RemoveAll(t.TempDir)
}

type stripPolicy int // controls whether DumpSongs removes data from songs

const (
	stripIDs stripPolicy = iota // clear SongID
	keepIDs                     // preserve SongID
)

const dumpCoversFlag = "-covers=true"

func (t *Tester) DumpSongs(strip stripPolicy, flags ...string) []types.Song {
	args := append([]string{
		"-config=" + t.dumpConfigFile,
		"-song-batch-size=" + strconv.Itoa(dumpBatchSize),
		"-play-batch-size=" + strconv.Itoa(dumpBatchSize),
	}, flags...)
	stdout, stderr, err := runCommand(filepath.Join(t.binDir, "dump_music"), args...)
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
	for _, s := range t.DumpSongs(keepIDs) {
		if s.SHA1 == sha1 {
			return s.SongID
		}
	}
	panic(fmt.Sprintf("failed to find ID for %v", sha1))
}

const keepUserDataFlag = "-import-user-data=false"

func dumpedGainsFlag(p string) string  { return "-dumped-gains-file=" + p }
func forceGlobFlag(glob string) string { return "-force-glob=" + glob }

func (t *Tester) UpdateSongs(flags ...string) {
	args := append([]string{
		"-config=" + t.updateConfigFile,
		"-test-gain-info=" + fmt.Sprintf("%f:%f:%f", TrackGain, AlbumGain, PeakAmp),
	}, flags...)
	if _, stderr, err := runCommand(filepath.Join(t.binDir, "update_music"),
		args...); err != nil {
		panic(fmt.Sprintf("%v\nstderr: %v", err, stderr))
	}
}

func (t *Tester) UpdateSongsFromList(path string, flags ...string) {
	t.UpdateSongs(append(flags, "-song-paths-file="+path)...)
}

func (t *Tester) ImportSongsFromJSONFile(path string, flags ...string) {
	t.UpdateSongs(append(flags, "-import-json-file="+path)...)
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

func (t *Tester) PostSongs(songs []types.Song, replaceUserData bool, updateDelay time.Duration) {
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

type deletionPolicy int // controls whether GetSongsForAndroid gets deleted songs

const (
	getRegularSongs deletionPolicy = iota // get only regular songs
	getDeletedSongs                       // get only deleted songs
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
