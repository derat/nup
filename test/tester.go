// Copyright 2020 Daniel Erat.
// All rights reserved.

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
)

const (
	// Username and Password are used for basic HTTP authentication by Tester.
	// The server must be configured to accept these credentials.
	Username = "testuser"
	Password = "testpass"

	dumpBatchSize    = 2                // song/play batch size for 'nup dump'
	androidBatchSize = 1                // song batch size when exporting for Android
	serverTimeout    = 10 * time.Second // timeout for HTTP requests to server
	commandTimeout   = 10 * time.Second // timeout for 'nup' commands
)

// Tester helps tests send HTTP requests to a development server and run the nup executable.
type Tester struct {
	T        *testing.T // used to report errors (panic on errors if nil)
	MusicDir string     // dir containing songs for 'nup update'
	CoverDir string     // dir containing album art for 'nup update'

	tempDir    string // base dir for temp files
	configFile string // path to nup config file
	serverURL  string // base URL for dev server
	client     http.Client
}

// TesterConfig contains optional configuration for Tester.
type TesterConfig struct {
	// MusicDir is the directory 'nup update' will examine for song files.
	// If empty, a directory will be created within tempDir.
	MusicDir string
	// CoverDir is the directory 'nup update' will examine for album art image files.
	// If empty, a directory will be created within tempDir.
	CoverDir string
}

// NewTester creates a new tester for the development server at serverURL.
//
// The supplied testing.T object will be used to report errors.
// If nil (e.g. if sharing a Tester between multiple tests), log.Panic will be called instead.
// The T field can be modified as tests start and stop.
//
// The nup command must be in $PATH.
func NewTester(tt *testing.T, serverURL, tempDir string, cfg TesterConfig) *Tester {
	t := &Tester{
		T:         tt,
		MusicDir:  cfg.MusicDir,
		CoverDir:  cfg.CoverDir,
		tempDir:   tempDir,
		serverURL: serverURL,
		client:    http.Client{Timeout: serverTimeout},
	}

	if err := os.MkdirAll(t.tempDir, 0755); err != nil {
		t.fatal("Failed ensuring temp dir exists: ", err)
	}
	if t.MusicDir == "" {
		t.MusicDir = filepath.Join(t.tempDir, "music")
		if err := os.MkdirAll(t.MusicDir, 0755); err != nil {
			t.fatal("Failed creating music dir: ", err)
		}
	}
	if t.CoverDir == "" {
		t.CoverDir = filepath.Join(t.tempDir, "covers")
		if err := os.MkdirAll(t.CoverDir, 0755); err != nil {
			t.fatal("Failed creating cover dir: ", err)
		}
	}

	writeConfig := func(fn string, d interface{}) (path string) {
		path = filepath.Join(t.tempDir, fn)
		f, err := os.Create(path)
		if err != nil {
			t.fatal("Failed writing config: ", err)
		}
		defer f.Close()

		if err = json.NewEncoder(f).Encode(d); err != nil {
			t.fatal("Failed encoding config: ", err)
		}
		return path
	}

	t.configFile = writeConfig("nup_config.json", client.Config{
		ServerURL:          t.serverURL,
		Username:           Username,
		Password:           Password,
		CoverDir:           t.CoverDir,
		MusicDir:           t.MusicDir,
		LastUpdateInfoFile: filepath.Join(t.tempDir, "last_update_info.json"),
		ComputeGain:        true,
	})

	return t
}

// fatal fails the test or panics (if not in a test).
// args are formatted using fmt.Sprint, i.e. spaces are only inserted between non-string pairs.
func (t *Tester) fatal(args ...interface{}) {
	// testing.T.Fatal formats like testing.T.Log, which formats like fmt.Println,
	// which always adds spaces between args.
	//
	// log.Panic formats like log.Print, which formats like fmt.Print,
	// which only adds spaces between non-strings.
	//
	// I hate this.
	msg := fmt.Sprint(args...)
	if t.T != nil {
		t.T.Fatal(msg)
	}
	log.Panic(msg)
}

func (t *Tester) fatalf(format string, args ...interface{}) {
	t.fatal(fmt.Sprintf(format, args...))
}

// runCommand synchronously runs the executable at p with args and returns its output.
func runCommand(p string, args ...string) (stdout, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p, args...)
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", "", err
	}
	if err = cmd.Start(); err != nil {
		return "", "", err
	}

	if outBytes, err := ioutil.ReadAll(outPipe); err != nil {
		return "", "", err
	} else if errBytes, err := ioutil.ReadAll(errPipe); err != nil {
		return string(outBytes), "", err
	} else {
		return string(outBytes), string(errBytes), cmd.Wait()
	}
}

type StripPolicy int // controls whether DumpSongs removes data from songs

const (
	StripIDs StripPolicy = iota // clear SongID
	KeepIDs                     // preserve SongID
)

// DumpSongs runs 'nup dump' with the supplied flags and returns unmarshaled songs.
func (t *Tester) DumpSongs(strip StripPolicy, flags ...string) []db.Song {
	args := append([]string{
		"-config=" + t.configFile,
		"dump",
		"-song-batch-size=" + strconv.Itoa(dumpBatchSize),
		"-play-batch-size=" + strconv.Itoa(dumpBatchSize),
	}, flags...)
	stdout, stderr, err := runCommand("nup", args...)
	if err != nil {
		t.fatalf("Failed dumping songs: %v\nstderr: %v", err, stderr)
	}
	songs := make([]db.Song, 0)

	if len(stdout) == 0 {
		return songs
	}

	for _, l := range strings.Split(strings.TrimSpace(stdout), "\n") {
		s := db.Song{}
		if err = json.Unmarshal([]byte(l), &s); err != nil {
			if err == io.EOF {
				break
			}
			t.fatalf("Failed unmarshaling song %q: %v", l, err)
		}
		if strip == StripIDs {
			s.SongID = ""
		}
		songs = append(songs, s)
	}
	return songs
}

// SongID dumps all songs from the server and returns the ID of the song with the
// supplied SHA1. The test is failed if the song is not found.
func (t *Tester) SongID(sha1 string) string {
	for _, s := range t.DumpSongs(KeepIDs) {
		if s.SHA1 == sha1 {
			return s.SongID
		}
	}
	t.fatalf("Failed finding ID for %v", sha1)
	return ""
}

const KeepUserDataFlag = "-import-user-data=false"
const UseFilenamesFlag = "-use-filenames"

func CompareDumpFileFlag(p string) string { return "-compare-dump-file=" + p }
func DumpedGainsFlag(p string) string     { return "-dumped-gains-file=" + p }
func ForceGlobFlag(glob string) string    { return "-force-glob=" + glob }

// UpdateSongs runs 'nup update' with the supplied flags.
func (t *Tester) UpdateSongs(flags ...string) {
	if _, stderr, err := t.UpdateSongsRaw(flags...); err != nil {
		t.fatalf("Failed updating songs: %v\nstderr: %v", err, stderr)
	}
}

// UpdateSongsRaw is similar to UpdateSongs but allows the caller to handle errors.
func (t *Tester) UpdateSongsRaw(flags ...string) (stdout, stderr string, err error) {
	return runCommand("nup", append([]string{
		"-config=" + t.configFile,
		"update",
		"-test-gain-info=" + fmt.Sprintf("%f:%f:%f", TrackGain, AlbumGain, PeakAmp),
	}, flags...)...)
}

// UpdateSongsFromList runs 'nup update' to import the songs listed in path.
func (t *Tester) UpdateSongsFromList(path string, flags ...string) {
	t.UpdateSongs(append(flags, "-song-paths-file="+path)...)
}

// ImportSongsFromJSON serializes the supplied songs to JSON and sends them
// to the server using 'nup update'.
func (t *Tester) ImportSongsFromJSONFile(songs []db.Song, flags ...string) {
	p, err := WriteSongsToJSONFile(t.tempDir, songs...)
	if err != nil {
		t.fatal("Failed writing songs to JSON file: ", err)
	}
	t.UpdateSongs(append(flags, "-import-json-file="+p)...)
}

// DeleteSong deletes the specified song using 'nup update'.
func (t *Tester) DeleteSong(songID string) {
	if _, stderr, err := runCommand(
		"nup",
		"-config="+t.configFile,
		"update",
		"-delete-song="+songID,
	); err != nil {
		t.fatalf("Failed deleting song %v: %v\nstderr: %v", songID, err, stderr)
	}
}

const DeleteAfterMergeFlag = "-delete-after-merge"

// MergeSongs merges one song's user data into another song using 'nup update'.
func (t *Tester) MergeSongs(fromID, toID string, flags ...string) {
	args := append([]string{
		"-config=" + t.configFile,
		"update",
		fmt.Sprintf("-merge-songs=%s:%s", fromID, toID),
	}, flags...)
	if _, stderr, err := runCommand("nup", args...); err != nil {
		t.fatalf("Failed merging song %v into %v: %v\nstderr: %v", fromID, toID, err, stderr)
	}
}

// ReindexSongs asks the server to reindex all songs.
func (t *Tester) ReindexSongs() {
	if _, stderr, err := runCommand(
		"nup",
		"-config="+t.configFile,
		"update",
		"-reindex-songs",
	); err != nil {
		t.fatalf("Failed reindexing songs: %v\nstderr: %v", err, stderr)
	}
}

func (t *Tester) newRequest(method, path string, body io.Reader) *http.Request {
	req, err := http.NewRequest(method, t.serverURL+path, body)
	if err != nil {
		t.fatalf("Failed creating %v request to %v: %v", method, path, err)
	}
	req.SetBasicAuth(Username, Password)
	return req
}

func (t *Tester) sendRequest(req *http.Request) *http.Response {
	resp, err := t.client.Do(req)
	if err != nil {
		t.fatal("Failed sending request: ", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.fatal("Server reported error: ", resp.Status)
	}
	return resp
}

func (t *Tester) doPost(pathAndQueryParams string, body io.Reader) {
	req := t.newRequest("POST", pathAndQueryParams, body)
	req.Header.Set("Content-Type", "text/plain")
	resp := t.sendRequest(req)
	defer resp.Body.Close()
	if _, err := ioutil.ReadAll(resp.Body); err != nil {
		t.fatalf("POST %v failed: %v", pathAndQueryParams, err)
	}
}

// PingServer fails the test if the server isn't serving the main page.
func (t *Tester) PingServer() {
	resp, err := t.client.Do(t.newRequest("GET", "/", nil))
	if err != nil && err.(*url.Error).Timeout() {
		t.fatal("Server timed out (is the app crashing?)")
	} else if err != nil {
		t.fatal("Failed pinging server (is dev_appserver running?): ", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.fatal("Server replied with failure: ", resp.Status)
	}
}

// PostSongs posts the supplied songs directly to the server.
func (t *Tester) PostSongs(songs []db.Song, replaceUserData bool, updateDelay time.Duration) {
	var buf bytes.Buffer
	e := json.NewEncoder(&buf)
	for _, s := range songs {
		if err := e.Encode(s); err != nil {
			t.fatal("Encoding songs failed: ", err)
		}
	}
	path := fmt.Sprintf("import?updateDelayNsec=%v", int64(updateDelay*time.Nanosecond))
	if replaceUserData {
		path += "&replaceUserData=1"
	}
	t.doPost(path, &buf)
}

// QuerySongs issues a query with the supplied parameters to the server.
func (t *Tester) QuerySongs(params ...string) []db.Song {
	resp := t.sendRequest(t.newRequest("GET", "query?"+strings.Join(params, "&"), nil))
	defer resp.Body.Close()

	songs := make([]db.Song, 0)
	if err := json.NewDecoder(resp.Body).Decode(&songs); err != nil {
		t.fatal("Decoding songs failed: ", err)
	}
	return songs
}

// ClearData clears all songs from the server.
func (t *Tester) ClearData() {
	t.doPost("clear", nil)
}

// FlushType describes which caches should be flushed by FlushCache.
type FlushType string

const (
	FlushAll      FlushType = "" // also flush Datastore
	FlushMemcache FlushType = "?onlyMemcache=1"
)

// FlushCache flushes the specified caches in the app server.
func (t *Tester) FlushCache(ft FlushType) {
	t.doPost("flush_cache"+string(ft), nil)
}

// GetTags gets the list of known tags from the server.
func (t *Tester) GetTags(requireCache bool) string {
	path := "tags"
	if requireCache {
		path += "?requireCache=1"
	}
	resp := t.sendRequest(t.newRequest("GET", path, nil))
	defer resp.Body.Close()

	tags := make([]string, 0)
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		t.fatal("Decoding tags failed: ", err)
	}
	return strings.Join(tags, ",")
}

// RateAndTag sends a rating and/or tags update to the server.
// The rating is not sent if negative, and tags are not sent if nil.
func (t *Tester) RateAndTag(songID string, rating int, tags []string) {
	var args string
	if rating >= 0 {
		args += fmt.Sprintf("&rating=%d", rating)
	}
	if tags != nil {
		args += "&tags=" + url.QueryEscape(strings.Join(tags, " "))
	}
	if args != "" {
		t.doPost("rate_and_tag?songId="+songID+args, nil)
	}
}

// ReportPlayed sends a playback report to the server.
func (t *Tester) ReportPlayed(songID string, startTime time.Time) {
	t.doPost(fmt.Sprintf("played?songId=%v&startTime=%v", songID, startTime.Unix()), nil)
}

// GetNowFromServer queries the server for the current time.
func (t *Tester) GetNowFromServer() time.Time {
	resp := t.sendRequest(t.newRequest("GET", "now", nil))
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.fatal("Reading time from server failed: ", err)
	}
	nsec, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		t.fatal("Parsing time failed: ", err)
	} else if nsec <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nsec)
}

type DeletionPolicy int // controls whether GetSongsForAndroid gets deleted songs

const (
	GetRegularSongs DeletionPolicy = iota // get only regular songs
	GetDeletedSongs                       // get only deleted songs
)

// GetSongsForAndroid exports songs from the server in a manner similar to
// that of the Android client.
func (t *Tester) GetSongsForAndroid(minLastModified time.Time, deleted DeletionPolicy) []db.Song {
	params := []string{
		"type=song",
		"max=" + strconv.Itoa(androidBatchSize),
		"omit=plays,sha1",
	}
	if deleted == GetDeletedSongs {
		params = append(params, "deleted=1")
	}
	if !minLastModified.IsZero() {
		params = append(params, fmt.Sprintf("minLastModifiedNsec=%d", minLastModified.UnixNano()))
	}

	songs := make([]db.Song, 0)
	var cursor string

	for {
		path := "export?" + strings.Join(params, "&")
		if cursor != "" {
			path += "&cursor=" + cursor
		}

		resp := t.sendRequest(t.newRequest("GET", path, nil))
		defer resp.Body.Close()

		// We receive a sequence of marshaled songs optionally followed by a cursor.
		cursor = ""
		dec := json.NewDecoder(resp.Body)
		for {
			var msg json.RawMessage
			if err := dec.Decode(&msg); err == io.EOF {
				break
			} else if err != nil {
				t.fatal("Decoding message failed: ", err)
			}

			var s db.Song
			if err := json.Unmarshal(msg, &s); err == nil {
				songs = append(songs, s)
			} else if err := json.Unmarshal(msg, &cursor); err == nil {
				break
			} else {
				t.fatal("Unmarshaling song failed: ", err)
			}
		}

		if cursor == "" {
			break
		}
	}

	return songs
}

// GetStats gets current stats from the server.
func (t *Tester) GetStats() db.Stats {
	resp := t.sendRequest(t.newRequest("GET", "stats", nil))
	defer resp.Body.Close()

	var stats db.Stats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.fatal("Decoding stats failed: ", err)
	}
	return stats
}

// UpdateStats instructs the server to update stats.
func (t *Tester) UpdateStats() {
	resp := t.sendRequest(t.newRequest("GET", "stats?update=1", nil))
	resp.Body.Close()
}

// ForceUpdateFailures configures the server to reject or allow updates.
func (t *Tester) ForceUpdateFailures(fail bool) {
	val := "0"
	if fail {
		val = "1"
	}
	t.doPost("config?forceUpdateFailures="+val, nil)
}
