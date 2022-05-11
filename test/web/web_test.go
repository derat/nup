// Copyright 2021 Daniel Erat.
// All rights reserved.

// Package web contains Selenium-based tests of the web interface.
package web

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/derat/nup/server/config"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"
	"golang.org/x/sys/unix"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
	slog "github.com/tebeka/selenium/log"
)

var (
	// Globals shared across all tests.
	webDrv         selenium.WebDriver // talks to browser using ChromeDriver
	appURL         string             // slash-terminated URL of App Engine server (if running app)
	tester         *test.Tester       // interacts with App Engine server (if running app)
	browserLog     io.Writer          // receives log messages from browser
	unitTestRegexp string             // regexp matching unit tests to run

	// Pull some stuff into our namespace for convenience.
	file0s  = test.Song0s.Filename
	file1s  = test.Song1s.Filename
	file5s  = test.Song5s.Filename
	file10s = test.Song10s.Filename
)

func TestMain(m *testing.M) {
	// Do everything in a function so that deferred calls run on failure.
	code, err := runTests(m)
	if err != nil {
		log.Print("Failed running tests: ", err)
	}
	os.Exit(code)
}

func runTests(m *testing.M) (res int, err error) {
	browserStderr := flag.Bool("browser-stderr", false, "Write browser log to stderr (default is -out-dir)")
	chromedriverPath := flag.String("chromedriver", "chromedriver", "Chromedriver executable ($PATH searched by default)")
	debugSelenium := flag.Bool("debug-selenium", false, "Write Selenium debug logs to stderr")
	headless := flag.Bool("headless", true, "Run Chrome headlessly using Xvfb")
	flag.StringVar(&unitTestRegexp, "unit-test-regexp", "", "Regexp matching unit tests to run (all other tests skipped)")
	flag.Parse()

	test.HandleSignals([]os.Signal{unix.SIGINT, unix.SIGTERM}, nil)

	// TODO: Find a better way to do this. There doesn't seem to be any way to use testing.M to
	// determine which tests are being run (probably by design), so we use -unit-test-regexp to
	// determine that we don't need to start the app for other tests. This is way faster when just
	// running unit tests.
	runApp := unitTestRegexp == ""

	outDir, keepOutDir, err := test.OutputDir("web_test")
	if err != nil {
		return -1, err
	}
	defer func() {
		// Also delete the dir if the browser logs are going to stderr and we're not running the
		// app, as everything interesting should be in the browser log in that case.
		if (res == 0 || (*browserStderr && !runApp)) && !keepOutDir {
			log.Print("Removing ", outDir)
			os.RemoveAll(outDir)
		}
	}()
	log.Print("Writing files to ", outDir)

	var musicDir string
	if runApp {
		// Serve music files in the background.
		musicDir = filepath.Join(outDir, "music")
		if err := os.MkdirAll(musicDir, 0755); err != nil {
			return -1, err
		}
		defer os.RemoveAll(musicDir)
		if err := test.CopySongs(musicDir, file0s, file1s, file5s, file10s); err != nil {
			return -1, fmt.Errorf("copying songs: %v", err)
		}
		musicSrv := test.ServeFiles(musicDir)
		defer musicSrv.Close()

		appLog, err := os.Create(filepath.Join(outDir, "app.log"))
		if err != nil {
			return -1, err
		}
		defer appLog.Close()

		cfg := &config.Config{
			GoogleUsers:    []string{testEmail},
			BasicAuthUsers: []config.BasicAuthInfo{{Username: test.Username, Password: test.Password}},
			SongBaseURL:    musicSrv.URL + "/",
			CoverBaseURL:   musicSrv.URL + "/.covers/", // bogus but required
			Presets:        presets,
		}
		storageDir := filepath.Join(outDir, "app_storage")
		appSrv, err := test.NewDevAppserver(cfg, storageDir, appLog)
		if err != nil {
			return -1, fmt.Errorf("dev_appserver: %v", err)
		}
		defer os.RemoveAll(storageDir)
		defer appSrv.Close()
		appURL = appSrv.URL()
		log.Print("dev_appserver is listening at ", appURL)
	}

	opts := []selenium.ServiceOption{}
	if *debugSelenium {
		selenium.SetDebug(true)
		opts = append(opts, selenium.Output(os.Stderr))
	}
	if *headless {
		opts = append(opts, selenium.StartFrameBuffer())
	}

	ports, err := test.FindUnusedPorts(1)
	if err != nil {
		return -1, fmt.Errorf("finding ports: %v", err)
	}
	chromeDrvPort := ports[0]
	svc, err := selenium.NewChromeDriverService(*chromedriverPath, chromeDrvPort, opts...)
	if err != nil {
		return -1, fmt.Errorf("ChromeDriver: %v", err)
	}
	defer svc.Stop()

	chromeArgs := []string{"--autoplay-policy=no-user-gesture-required"}
	if test.CloudBuild() {
		chromeArgs = append(chromeArgs,
			"--no-sandbox",            // actually get Chrome to run
			"--disable-dev-shm-usage", // prevent random crashes: https://stackoverflow.com/a/53970825/6882947
		)
	}
	caps := selenium.Capabilities{}
	caps.AddChrome(chrome.Capabilities{Args: chromeArgs})
	caps.SetLogLevel(slog.Browser, slog.All)
	webDrv, err = selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d/wd/hub", chromeDrvPort))
	if err != nil {
		return -1, fmt.Errorf("Selenium: %v", err)
	}
	defer webDrv.Quit()

	if *browserStderr {
		browserLog = os.Stderr
	} else {
		// Create a file containing messages logged by the web interface.
		f, err := os.Create(filepath.Join(outDir, "browser.log"))
		if err != nil {
			return -1, err
		}
		defer f.Close()
		browserLog = f
	}
	defer copyBrowserLogs()

	if runApp {
		writeLogHeader("Running web tests against " + appURL)
		testerDir := filepath.Join(outDir, "tester")
		tester = test.NewTester(nil, appURL, testerDir, test.TesterConfig{MusicDir: musicDir})
		defer os.RemoveAll(testerDir)
	}

	res = m.Run()
	return res, nil
}

// writeLogHeader writes s and a line of dashes to browserLog.
func writeLogHeader(s string) {
	fmt.Fprintf(browserLog, "%s\n%s\n", s, strings.Repeat("-", 80))
}

// Log messages usually look like this:
//  http://localhost:8080/music-searcher.js 478:18 "Got response with 1 song(s)"
// This regexp matches the filename, line number, and message.
var logRegexp = regexp.MustCompile(`(?s)^https?://[^ ]+/([^ /]+\.js) (\d+):\d+ (.*)$`)

// copyBrowserLogs gets new log messages from the browser and writes them to browserLog.
func copyBrowserLogs() {
	msgs, err := webDrv.Log(slog.Browser)
	if err != nil {
		fmt.Fprintf(browserLog, "Failed getting browser logs: %v\n", err)
		return
	}
	for _, msg := range msgs {
		// Try to make logs more readable by dropping the server URL from the
		// beginning of the filename and lining up the actual messages.
		text := msg.Message
		if ms := logRegexp.FindStringSubmatch(text); ms != nil {
			if u, err := strconv.Unquote(ms[3]); err == nil {
				ms[3] = u
			}
			text = fmt.Sprintf("%-24s %s", ms[1]+":"+ms[2], ms[3])
		}
		ts := msg.Timestamp.Format("15:04:05.000")
		fmt.Fprintf(browserLog, "%s %-7s %s\n", ts, msg.Level, text)
	}
}

// initWebTest should be called at the beginning of each test.
// The returned page object is used to interact with the web interface via Selenium.
func initWebTest(t *testing.T) (p *page, done func()) {
	// Huge hack: skip the test if we're only running unit tests.
	if unitTestRegexp != "" && t.Name() != "TestUnit" {
		t.SkipNow() // calls runtime.Goexit
	}

	// Copy any browser logs from the previous test and write a header.
	copyBrowserLogs()
	io.WriteString(browserLog, "\n")
	writeLogHeader(t.Name())

	if appURL == "" {
		return nil, func() {}
	}

	tester.T = t
	tester.PingServer()
	tester.ClearData()
	tester.ForceUpdateFailures(false)
	return newPage(t, webDrv, appURL), func() { tester.T = nil }
}

// MaxPlays needs to be explicitly set to -1 here since these structs are
// declared literally rather than being unmarshaled from JSON by config.Parse
// (which assigns a default of -1 if the field isn't specified).
var presets = []config.SearchPreset{
	{
		Name:       "instrumental old",
		Tags:       "instrumental",
		MinRating:  4,
		LastPlayed: 6,
		MaxPlays:   -1,
		Shuffle:    true,
		Play:       true,
	},
	{
		Name:      "mellow",
		Tags:      "mellow",
		MinRating: 4,
		MaxPlays:  -1,
		Shuffle:   true,
		Play:      true,
	},
	{
		Name:      "played once",
		MinRating: 4,
		MaxPlays:  1,
		Shuffle:   true,
		Play:      true,
	},
	{
		Name:        "new albums",
		MinRating:   -1,
		FirstPlayed: 3,
		MaxPlays:    -1,
		FirstTrack:  true,
	},
	{
		Name:      "unrated",
		MinRating: -1,
		Unrated:   true,
		MaxPlays:  -1,
		Play:      true,
	},
}

// importSongs posts the supplied db.Song or []db.Song args to the server.
func importSongs(songs ...interface{}) {
	tester.PostSongs(joinSongs(songs...), true, 0)
}

func TestKeywordQuery(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	album1 := joinSongs(
		newSong("ar1", "ti1", "al1", withTrack(1)),
		newSong("ar1", "ti2", "al1", withTrack(2)),
		newSong("ar1", "ti3", "al1", withTrack(3)),
	)
	album2 := joinSongs(
		newSong("ar2", "ti1", "al2", withTrack(1)),
		newSong("ar2", "ti2", "al2", withTrack(2)),
	)
	album3 := joinSongs(
		newSong("artist with space", "ti1", "al3", withTrack(1)),
	)
	importSongs(album1, album2, album3)

	for _, tc := range []struct {
		kw   string
		want []db.Song
	}{
		{"album:al1", album1},
		{"album:al2", album2},
		{"artist:ar1", album1},
		{"artist:\"artist with space\"", album3},
		{"ti2", joinSongs(album1[1], album2[1])},
		{"AR2 ti1", joinSongs(album2[0])},
		{"ar1 bogus", nil},
	} {
		page.setStage(tc.kw)
		page.setText(keywordsInput, tc.kw)
		page.click(searchButton)
		page.checkSearchResults(tc.want)
	}
}

func TestTagQuery(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song1 := newSong("ar1", "ti1", "al1", withTags("electronic", "instrumental"))
	song2 := newSong("ar2", "ti2", "al2", withTags("rock", "guitar"))
	song3 := newSong("ar3", "ti3", "al3", withTags("instrumental", "rock"))
	importSongs(song1, song2, song3)

	for _, tc := range []struct {
		tags string
		want []db.Song
	}{
		{"electronic", joinSongs(song1)},
		{"guitar rock", joinSongs(song2)},
		{"instrumental", joinSongs(song1, song3)},
		{"instrumental -electronic", joinSongs(song3)},
	} {
		page.setStage(tc.tags)
		page.setText(tagsInput, tc.tags)
		page.click(searchButton)
		page.checkSearchResults(tc.want)
	}
}

func TestRatingQuery(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song1 := newSong("a", "t", "al1", withRating(0.0))
	song2 := newSong("a", "t", "al2", withRating(0.25))
	song3 := newSong("a", "t", "al3", withRating(0.5))
	song4 := newSong("a", "t", "al4", withRating(0.75))
	song5 := newSong("a", "t", "al5", withRating(1.0))
	song6 := newSong("a", "t", "al6", withRating(-1.0))
	allSongs := joinSongs(song1, song2, song3, song4, song5, song6)
	importSongs(allSongs)

	page.setStage("unset")
	page.setText(keywordsInput, "t") // need to set at least one search term
	page.click(searchButton)
	page.checkSearchResults(allSongs)

	page.click(resetButton)
	for _, tc := range []struct {
		option string
		want   []db.Song
	}{
		{oneStar, joinSongs(song1, song2, song3, song4, song5)},
		{twoStars, joinSongs(song2, song3, song4, song5)},
		{threeStars, joinSongs(song3, song4, song5)},
		{fourStars, joinSongs(song4, song5)},
		{fiveStars, joinSongs(song5)},
	} {
		page.setStage(tc.option)
		page.clickOption(minRatingSelect, tc.option)
		page.click(searchButton)
		page.checkSearchResults(tc.want)
	}

	page.setStage("unrated")
	page.click(resetButton)
	page.click(unratedCheckbox)
	page.click(searchButton)
	page.checkSearchResults(joinSongs(song6))
}

func TestFirstTrackQuery(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	album1 := joinSongs(
		newSong("ar1", "ti1", "al1", withTrack(1), withDisc(1)),
		newSong("ar1", "ti2", "al1", withTrack(2), withDisc(1)),
		newSong("ar1", "ti3", "al1", withTrack(3), withDisc(1)),
	)
	album2 := joinSongs(
		newSong("ar2", "ti1", "al2", withTrack(1), withDisc(1)),
		newSong("ar2", "ti2", "al2", withTrack(2), withDisc(1)),
	)
	importSongs(album1, album2)

	page.click(firstTrackCheckbox)
	page.click(searchButton)
	page.checkSearchResults(joinSongs(album1[0], album2[0]))
}

func TestOrderByLastPlayedQuery(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	t1, t2, t3 := time.Unix(1, 0), time.Unix(2, 0), time.Unix(3, 0)
	song1 := newSong("ar1", "ti1", "al1", withPlays(t2, t3))
	song2 := newSong("ar2", "ti2", "al2", withPlays(t1))
	song3 := newSong("ar3", "ti3", "al3", withPlays(t1, t2))
	importSongs(song1, song2, song3)

	page.click(orderByLastPlayedCheckbox)
	page.click(searchButton)
	page.checkSearchResults(joinSongs(song2, song3, song1))
}

func TestMaxPlaysQuery(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	t1, t2, t3 := time.Unix(1, 0), time.Unix(2, 0), time.Unix(3, 0)
	song1 := newSong("ar1", "ti1", "al1", withPlays(t1, t2))
	song2 := newSong("ar2", "ti2", "al2", withPlays(t1, t2, t3))
	song3 := newSong("ar3", "ti3", "al3")
	importSongs(song1, song2, song3)

	for _, tc := range []struct {
		plays string
		want  []db.Song
	}{
		{"2", joinSongs(song1, song3)},
		{"3", joinSongs(song1, song2, song3)},
		{"0", joinSongs(song3)},
	} {
		page.setStage(tc.plays)
		page.setText(maxPlaysInput, tc.plays)
		page.click(searchButton)
		page.checkSearchResults(tc.want)
	}
}

func TestPlayTimeQuery(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	now := time.Now()
	song1 := newSong("ar1", "ti1", "al1", withPlays(now.Add(-5*24*time.Hour)))
	song2 := newSong("ar2", "ti2", "al2", withPlays(now.Add(-90*24*time.Hour)))
	importSongs(song1, song2)

	for _, tc := range []struct {
		first, last string
		want        []db.Song
	}{
		{oneDay, unsetTime, nil},
		{oneWeek, unsetTime, joinSongs(song1)},
		{oneYear, unsetTime, joinSongs(song1, song2)},
		{unsetTime, oneYear, nil},
		{unsetTime, oneMonth, joinSongs(song2)},
		{unsetTime, oneDay, joinSongs(song1, song2)},
	} {
		page.setStage(fmt.Sprintf("%s / %s", tc.first, tc.last))
		page.clickOption(firstPlayedSelect, tc.first)
		page.clickOption(lastPlayedSelect, tc.last)
		page.click(searchButton)
		page.checkSearchResults(tc.want)
	}
}

func TestSearchResultCheckboxes(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	songs := joinSongs(
		newSong("a", "t1", "al", withTrack(1)),
		newSong("a", "t2", "al", withTrack(2)),
		newSong("a", "t3", "al", withTrack(3)),
	)
	importSongs(songs)

	// All songs should be selected by default after a search.
	page.setText(keywordsInput, songs[0].Artist)
	page.click(searchButton)
	page.checkSearchResults(songs, hasChecked(true, true, true))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked)

	// Click the top checkbox to deselect all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, hasChecked(false, false, false))
	page.checkCheckbox(searchResultsCheckbox, 0)

	// Click it again to select all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, hasChecked(true, true, true))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked)

	// Click the first song to deselect it.
	page.clickSongRowCheckbox(searchResultsTable, 0, "")
	page.checkSearchResults(songs, hasChecked(false, true, true))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked|checkboxTransparent)

	// Click the top checkbox to deselect all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, hasChecked(false, false, false))
	page.checkCheckbox(searchResultsCheckbox, 0)

	// Click the first and second songs individually to select them.
	page.clickSongRowCheckbox(searchResultsTable, 0, "")
	page.clickSongRowCheckbox(searchResultsTable, 1, "")
	page.checkSearchResults(songs, hasChecked(true, true, false))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked|checkboxTransparent)

	// Click the third song to select it as well.
	page.clickSongRowCheckbox(searchResultsTable, 2, "")
	page.checkSearchResults(songs, hasChecked(true, true, true))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked)

	// Shift-click from the first to third song to select all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, hasChecked(false, false, false))
	page.clickSongRowCheckbox(searchResultsTable, 0, selenium.ShiftKey)
	page.clickSongRowCheckbox(searchResultsTable, 2, selenium.ShiftKey)
	page.checkSearchResults(songs, hasChecked(true, true, true))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked)
}

func TestAddToPlaylist(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song1 := newSong("a", "t1", "al1", withTrack(1))
	song2 := newSong("a", "t2", "al1", withTrack(2))
	song3 := newSong("a", "t3", "al2", withTrack(1))
	song4 := newSong("a", "t4", "al2", withTrack(2))
	song5 := newSong("a", "t5", "al3", withTrack(1))
	song6 := newSong("a", "t6", "al3", withTrack(2))
	importSongs(song1, song2, song3, song4, song5, song6)

	page.setText(keywordsInput, "al1")
	page.click(searchButton)
	page.checkSearchResults(joinSongs(song1, song2))
	page.click(appendButton)
	page.checkPlaylist(joinSongs(song1, song2), hasActive(0))

	// Pause so we don't advance through the playlist mid-test.
	page.checkSong(song1, isPaused(false))
	page.click(playPauseButton)
	page.checkSong(song1, isPaused(true))

	// Inserting should leave the current track paused.
	page.setText(keywordsInput, "al2")
	page.click(searchButton)
	page.checkSearchResults(joinSongs(song3, song4))
	page.click(insertButton)
	page.checkPlaylist(joinSongs(song1, song3, song4, song2), hasActive(0))
	page.checkSong(song1, isPaused(true))

	// Replacing should result in the new first track being played.
	page.setText(keywordsInput, "al3")
	page.click(searchButton)
	page.checkSearchResults(joinSongs(song5, song6))
	page.click(replaceButton)
	page.checkPlaylist(joinSongs(song5, song6), hasActive(0))
	page.checkSong(song5, isPaused(false))

	// Appending should leave the first track playing.
	page.setText(keywordsInput, "al1")
	page.click(searchButton)
	page.checkSearchResults(joinSongs(song1, song2))
	page.click(appendButton)
	page.checkPlaylist(joinSongs(song5, song6, song1, song2), hasActive(0))
	page.checkSong(song5, isPaused(false))

	// The "I'm feeling lucky" button should replace the current playlist and
	// start playing the new first song.
	page.setText(keywordsInput, "al2")
	page.click(luckyButton)
	page.checkPlaylist(joinSongs(song3, song4), hasActive(0))
	page.checkSong(song3, isPaused(false))
}

func TestPlaybackButtons(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	// Using a 10-second song here makes this test slow, but I've seen flakiness when using the
	// 5-second song.
	song1 := newSong("artist", "track1", "album", withTrack(1), withFilename(file10s))
	song2 := newSong("artist", "track2", "album", withTrack(2), withFilename(file1s))
	importSongs(song1, song2)

	// We should start playing automatically when the 'lucky' button is clicked.
	page.setText(keywordsInput, song1.Artist)
	page.click(luckyButton)
	page.checkSong(song1, isPaused(false), hasFilename(song1.Filename))
	page.checkPlaylist(joinSongs(song1, song2), hasActive(0))

	// Pausing and playing should work.
	page.click(playPauseButton)
	page.checkSong(song1, isPaused(true))
	page.checkPlaylist(joinSongs(song1, song2), hasActive(0))
	page.click(playPauseButton)
	page.checkSong(song1, isPaused(false))

	// Clicking the 'next' button should go to the second song.
	page.click(nextButton)
	page.checkSong(song2, isPaused(false), hasFilename(song2.Filename))
	page.checkPlaylist(joinSongs(song1, song2), hasActive(1))

	// Clicking it again shouldn't do anything.
	page.click(nextButton)
	page.checkSong(song2)
	page.checkPlaylist(joinSongs(song1, song2), hasActive(1))

	// Clicking the 'prev' button should go back to the first song.
	page.click(prevButton)
	page.checkSong(song1, isPaused(false))
	page.checkPlaylist(joinSongs(song1, song2), hasActive(0))

	// Clicking it again shouldn't do anything.
	page.click(prevButton)
	page.checkSong(song1, isPaused(false))
	page.checkPlaylist(joinSongs(song1, song2), hasActive(0))

	// We should eventually play through to the second song.
	// Use a long timeout since I saw a failure on GCP where we were somehow still only at position
	// 0:09 in the 10-second song after waiting for 15 seconds.
	page.checkSong(song2, isPaused(false), useTimeout(20*time.Second))
	page.checkPlaylist(joinSongs(song1, song2), hasActive(1))
}

func TestContextMenu(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song1 := newSong("a", "t1", "al", withTrack(1))
	song2 := newSong("a", "t2", "al", withTrack(2))
	song3 := newSong("a", "t3", "al", withTrack(3))
	song4 := newSong("a", "t4", "al", withTrack(4))
	song5 := newSong("a", "t5", "al", withTrack(5))
	songs := joinSongs(song1, song2, song3, song4, song5)
	importSongs(songs)

	page.setText(keywordsInput, song1.Album)
	page.click(luckyButton)
	page.checkSong(song1, isPaused(false))
	page.checkPlaylist(songs, hasActive(0))

	page.rightClickSongRow(playlistTable, 3)
	page.checkPlaylist(songs, hasMenu(3))
	page.click(menuPlay)
	page.checkSong(song4, isPaused(false))
	page.checkPlaylist(songs, hasActive(3))

	page.rightClickSongRow(playlistTable, 2)
	page.checkPlaylist(songs, hasMenu(2))
	page.click(menuPlay)
	page.checkSong(song3, isPaused(false))
	page.click(playPauseButton) // make sure we don't advance mid-test
	page.checkSong(song3, isPaused(true))
	page.checkPlaylist(songs, hasActive(2))

	page.rightClickSongRow(playlistTable, 0)
	page.checkPlaylist(songs, hasMenu(0))
	page.click(menuRemove)
	page.checkSong(song3, isPaused(true))
	page.checkPlaylist(joinSongs(song2, song3, song4, song5), hasActive(1))

	page.rightClickSongRow(playlistTable, 1)
	page.checkPlaylist(joinSongs(song2, song3, song4, song5), hasMenu(1))
	page.click(menuTruncate)
	page.checkSong(song2, isPaused(true))
	page.checkPlaylist(joinSongs(song2), hasActive(0))
}

func TestDisplayTimeWhilePlaying(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song := newSong("ar", "t", "al", withFilename(file5s))
	importSongs(song)

	page.setText(keywordsInput, song.Artist)
	page.click(luckyButton)

	// TODO: This can be flaky when the checks happen to run slowly.
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:00 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:01 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:02 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:03 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:04 / 0:05 ]"))
	page.checkSong(song, isEnded(true), isPaused(true), hasTimeStr("[ 0:05 / 0:05 ]"))
}

func TestReportPlayed(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song1 := newSong("a", "t1", "al", withTrack(1), withFilename(file5s))
	song2 := newSong("a", "t2", "al", withTrack(2), withFilename(file1s))
	importSongs(song1, song2)

	// Skip the first song early on, but listen to all of the second song.
	page.setText(keywordsInput, song1.Artist)
	page.click(luckyButton)
	page.checkSong(song1, isPaused(false))
	page.click(playPauseButton)
	song2Lower := time.Now()
	page.click(nextButton)
	page.checkSong(song2, isEnded(true))
	song2Upper := time.Now()

	// Only the second song should've been reported.
	checkServerSong(t, song2, hasSrvPlay(song2Lower, song2Upper))
	checkServerSong(t, song1, hasNoSrvPlays())

	// Go back to the first song but pause it immediately.
	song1Lower := time.Now()
	page.click(prevButton)
	page.checkSong(song1, isPaused(false))
	song1Upper := time.Now()
	page.click(playPauseButton)
	page.checkSong(song1, isPaused(true))

	// After more than half of the first song has played, it should be reported.
	page.click(playPauseButton)
	page.checkSong(song1, isPaused(false))
	checkServerSong(t, song1, hasSrvPlay(song1Lower, song1Upper))
	checkServerSong(t, song2, hasSrvPlay(song2Lower, song2Upper))
}

func TestReportReplay(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song := newSong("a", "t1", "al", withFilename(file1s))
	importSongs(song)

	// Play the song to completion.
	page.setText(keywordsInput, song.Artist)
	firstLower := time.Now()
	page.click(luckyButton)
	page.checkSong(song, isEnded(true))

	// Replay the song.
	secondLower := time.Now()
	page.click(playPauseButton)

	// Both playbacks should be reported.
	checkServerSong(t, song, hasSrvPlay(firstLower, secondLower),
		hasSrvPlay(secondLower, secondLower.Add(2*time.Second)))
}

func TestRateAndTag(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song := newSong("ar", "t1", "al", withRating(0.5), withTags("rock", "guitar"))
	importSongs(song)

	page.setText(keywordsInput, song.Artist)
	page.click(luckyButton)
	page.checkSong(song, isPaused(false))
	page.click(playPauseButton)
	page.checkSong(song, isPaused(true), hasRatingStr(threeStars),
		hasImgTitle("Rating: ★★★☆☆\nTags: guitar rock"))

	page.click(coverImage)
	page.click(ratingFourStars)
	page.click(updateCloseImage)
	page.checkSong(song, hasRatingStr(fourStars),
		hasImgTitle("Rating: ★★★★☆\nTags: guitar rock"))
	checkServerSong(t, song, hasSrvRating(0.75), hasSrvTags("guitar", "rock"))

	page.click(coverImage)
	page.sendKeys(editTagsTextarea, " +metal", false)
	page.click(updateCloseImage)
	page.checkSong(song, hasRatingStr(fourStars),
		hasImgTitle("Rating: ★★★★☆\nTags: guitar metal rock"))
	checkServerSong(t, song, hasSrvRating(0.75), hasSrvTags("guitar", "metal", "rock"))
}

func TestRetryUpdates(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song := newSong("ar", "t1", "al", withFilename(file1s),
		withRating(0.5), withTags("rock", "guitar"))
	importSongs(song)

	// Configure the server to reject updates and play the song.
	tester.ForceUpdateFailures(true)
	page.setText(keywordsInput, song.Artist)
	firstLower := time.Now()
	page.click(luckyButton)
	page.checkSong(song, isEnded(true))
	firstUpper := time.Now()

	// Change the song's rating and tags.
	page.click(coverImage)
	page.click(ratingFourStars)
	page.setText(editTagsTextarea, "+jazz +mellow")
	page.click(updateCloseImage)

	// Wait a bit to let the updates fail and then let them succeed.
	time.Sleep(time.Second)
	tester.ForceUpdateFailures(false)
	checkServerSong(t, song, hasSrvRating(0.75), hasSrvTags("jazz", "mellow"),
		hasSrvPlay(firstLower, firstUpper))

	// Queue some more failed updates.
	tester.ForceUpdateFailures(true)
	secondLower := time.Now()
	page.click(playPauseButton)
	page.checkSong(song, isEnded(false))
	page.checkSong(song, isEnded(true))
	secondUpper := time.Now()
	page.click(coverImage)
	page.click(ratingTwoStars)
	page.setText(editTagsTextarea, "+lively +soul")
	page.click(updateCloseImage)
	time.Sleep(time.Second)

	// The queued updates should be sent if the page is reloaded.
	page.reload()
	tester.ForceUpdateFailures(false)
	checkServerSong(t, song, hasSrvRating(0.25), hasSrvTags("lively", "soul"),
		hasSrvPlay(firstLower, firstUpper), hasSrvPlay(secondLower, secondUpper))

	// In the case of multiple queued updates, the last one should take precedence.
	tester.ForceUpdateFailures(true)
	page.setText(keywordsInput, song.Artist)
	page.click(luckyButton)
	page.checkSong(song)
	for _, r := range [][]loc{ratingThreeStars, ratingFourStars, ratingFiveStars} {
		page.click(coverImage)
		page.checkDisplayed(updateCloseImage, true)
		page.click(r)
		page.click(updateCloseImage)
		page.checkDisplayed(updateCloseImage, false)
	}
	tester.ForceUpdateFailures(false)
	checkServerSong(t, song, hasSrvRating(1.0))
}

func TestEditTagsAutocomplete(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	song1 := newSong("ar", "t1", "al", withTags("a0", "a1", "b"))
	song2 := newSong("ar", "t2", "al", withTags("c0", "c1", "d", "long"))
	importSongs(song1, song2)

	page.refreshTags()
	page.setText(keywordsInput, song1.Title)
	page.click(luckyButton)
	page.checkSong(song1)

	page.click(coverImage)
	page.checkAttr(editTagsTextarea, "value", "a0 a1 b ")

	page.sendKeys(editTagsTextarea, "d"+selenium.TabKey, false)
	page.checkAttr(editTagsTextarea, "value", "a0 a1 b d ")

	page.sendKeys(editTagsTextarea, "c"+selenium.TabKey, false)
	page.checkAttr(editTagsTextarea, "value", "a0 a1 b d c")
	page.checkText(editTagsSuggester, `^\s*c0\s*c1\s*$`)

	page.sendKeys(editTagsTextarea, "1"+selenium.TabKey, false)
	page.checkAttr(editTagsTextarea, "value", "a0 a1 b d c1 ")

	// Position the caret at the beginning of the "c1" tag and complete "long".
	// The caret strangely seems to get moved to the end of the textarea for each
	// sendKeys call, so do this all in one go.
	page.sendKeys(editTagsTextarea, strings.Repeat(selenium.LeftArrowKey, 3)+"l"+selenium.TabKey, false)
	page.checkAttr(editTagsTextarea, "value", "a0 a1 b d long c1 ")
}

func TestDragSongs(t *testing.T) {
	page, done := initWebTest(t)
	defer done()

	s1 := newSong("a", "t1", "al", withTrack(1))
	s2 := newSong("a", "t2", "al", withTrack(2))
	s3 := newSong("a", "t3", "al", withTrack(3))
	s4 := newSong("a", "t4", "al", withTrack(4))
	s5 := newSong("a", "t5", "al", withTrack(5))
	importSongs(joinSongs(s1, s2, s3, s4, s5))

	page.setText(keywordsInput, s1.Artist)
	page.click(searchButton)
	page.checkSearchResults(joinSongs(s1, s2, s3, s4, s5))
	page.clickSongRowCheckbox(searchResultsTable, 2, "")
	page.checkSearchResults(
		joinSongs(s1, s2, s3, s4, s5),
		hasChecked(true, true, false, true, true))

	// Drag the middle song up to the second song's position.
	page.dragSongRow(searchResultsTable, 2, 1, -10)
	page.checkSearchResults(
		joinSongs(s1, s3, s2, s4, s5),
		hasChecked(true, false, true, true, true))

	// Now drag it to the end of the list.
	page.dragSongRow(searchResultsTable, 1, 4, 10)
	page.checkSearchResults(
		joinSongs(s1, s2, s4, s5, s3),
		hasChecked(true, true, true, true, false))

	// Enqueue the songs.
	page.click(appendButton)
	page.checkSong(s1, isPaused(false))
	page.checkPlaylist(joinSongs(s1, s2, s4, s5), hasActive(0))
	page.click(playPauseButton)

	// Drag the second song in the playlist above the first song.
	page.dragSongRow(playlistTable, 1, 0, -10)
	page.checkSong(s1, isPaused(true))
	page.checkPlaylist(joinSongs(s2, s1, s4, s5), hasActive(1))

	// Switch songs to check that the underlying playlist was updated.
	page.click(prevButton)
	page.checkSong(s2, isPaused(false))
	page.click(playPauseButton)
	page.checkPlaylist(joinSongs(s2, s1, s4, s5), hasActive(0))

	// Now drag the active song to the end of the playlist.
	page.dragSongRow(playlistTable, 0, 3, 10)
	page.checkSong(s2, isPaused(true))
	page.checkPlaylist(joinSongs(s1, s4, s5, s2), hasActive(3))
}

func TestOptions(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	show := func() { page.emitKeyDown("o", 79, true /* alt */) }

	show()
	page.checkAttr(gainTypeSelect, "value", gainAutoValue)
	// TODO: This somehow fails sometimes due to the option not being found (despite clickOption()
	// finding the correct number of options). Hopefully additional logging in clickOption() will
	// shed light on why it isn't found.
	page.clickOption(gainTypeSelect, gainTrack)
	page.checkAttr(gainTypeSelect, "value", gainTrackValue)

	// I *think* that this clicks the middle of the range. This might be a
	// no-op since it should be 0, which is the default. :-/
	page.click(preAmpRange)
	origPreAmp := page.getAttrOrFail(page.getOrFail(preAmpRange), "value", false)

	page.click(optionsOKButton)
	page.checkGone(optionsOKButton)

	// The dialog should also be available via the menu, and Escape should dismiss it.
	page.click(menuButton)
	page.click(menuOptions)
	page.getOrFail(optionsOKButton)
	page.sendKeys(body, selenium.EscapeKey, false)
	page.checkGone(optionsOKButton)

	page.reload()
	show()
	page.checkAttr(gainTypeSelect, "value", gainTrackValue)
	page.checkAttr(preAmpRange, "value", origPreAmp)
	// TODO: For reasons that are unclear to me, clicking the OK button ocasionally fails at this
	// point with "element not visible: element not interactable", so I'm speculatively dismissing
	// the dialog with the escape key instead.
	page.sendKeys(body, selenium.EscapeKey, false)
	page.checkGone(optionsOKButton)
}

func TestSongInfo(t *testing.T) {
	page, done := initWebTest(t)
	defer done()

	song1 := newSong("a", "t1", "al1", withTrack(1), withLength(123),
		withRating(1.0), withTags("guitar", "instrumental"))
	song2 := newSong("a", "t2", "al2", withTrack(5), withLength(52))
	importSongs(song1, song2)

	page.setText(keywordsInput, "a")
	page.click(luckyButton)
	page.checkPlaylist(joinSongs(song1, song2), hasActive(0))
	page.click(playPauseButton)
	page.checkSong(song1, isPaused(true))

	page.emitKeyDown("i", 73, true /* alt */)
	page.checkText(infoArtist, song1.Artist)
	page.checkText(infoTitle, song1.Title)
	page.checkText(infoAlbum, song1.Album)
	page.checkText(infoTrack, strconv.Itoa(song1.Track))
	page.checkText(infoLength, "2:03")
	page.checkText(infoRating, "★★★★★")
	page.checkText(infoTags, strings.Join(song1.Tags, " "))
	page.click(infoDismissButton)
	page.checkGone(infoDismissButton)

	page.rightClickSongRow(playlistTable, 1)
	page.click(menuInfo)
	page.checkText(infoArtist, song2.Artist)
	page.checkText(infoTitle, song2.Title)
	page.checkText(infoAlbum, song2.Album)
	page.checkText(infoTrack, strconv.Itoa(song2.Track))
	page.checkText(infoLength, "0:52")
	page.checkText(infoRating, "Unrated")
	page.checkText(infoTags, "")
	page.click(infoDismissButton)
	page.checkGone(infoDismissButton)
}

func TestPresets(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	now := time.Now()
	now2 := now.Add(-5 * time.Minute)
	old := now.Add(-2 * 365 * 24 * time.Hour)
	old2 := old.Add(-5 * time.Minute)
	song1 := newSong("a", "t1", "unrated")
	song2 := newSong("a", "t1", "new", withRating(0.25), withTrack(1), withDisc(1), withPlays(now, now2))
	song3 := newSong("a", "t2", "new", withRating(1.0), withTrack(2), withDisc(1), withPlays(now, now2))
	song4 := newSong("a", "t1", "old", withRating(0.75), withPlays(old, old2))
	song5 := newSong("a", "t2", "old", withRating(0.75), withTags("instrumental"), withPlays(old, old2))
	song6 := newSong("a", "t1", "mellow", withRating(0.75), withTags("mellow"))
	importSongs(song1, song2, song3, song4, song5, song6)

	page.clickOption(presetSelect, presetInstrumentalOld)
	page.checkSong(song5)
	page.clickOption(presetSelect, presetMellow)
	page.checkSong(song6)
	page.clickOption(presetSelect, presetNewAlbums)
	page.checkSearchResults(joinSongs(song2))
	page.clickOption(presetSelect, presetUnrated)
	page.checkSong(song1)
	page.clickOption(presetSelect, presetPlayedOnce)
	page.checkSong(song6)

	if active, err := page.wd.ActiveElement(); err != nil {
		t.Error("Failed getting active element: ", err)
	} else if reflect.DeepEqual(active, page.getOrFail(presetSelect)) {
		t.Error("Preset select still focused after click")
	}
}

func TestPresentation(t *testing.T) {
	page, done := initWebTest(t)
	defer done()
	show := func() { page.emitKeyDown("v", 86, true /* alt */) }
	next := func() { page.emitKeyDown("n", 78, true /* alt */) }
	rate := func() { page.emitKeyDown("r", 82, true /* alt */) }

	song1 := newSong("artist", "track1", "album1", withTrack(1))
	song2 := newSong("artist", "track2", "album1", withTrack(2))
	song3 := newSong("artist", "track1", "album2", withTrack(1))
	importSongs(song1, song2, song3)

	// Enqueue song1 and song2 and check that they're displayed.
	page.setText(keywordsInput, "album:"+song1.Album)
	page.click(luckyButton)
	page.checkPlaylist(joinSongs(song1, song2), hasActive(0))
	show()
	page.checkPresentation(&song1, &song2)
	page.sendKeys(body, selenium.EscapeKey, false)
	page.checkPresentation(nil, nil)

	// Insert song3 after song1 and check that it's displayed as the next song.
	page.setText(keywordsInput, "album:"+song3.Album)
	page.click(searchButton)
	page.checkSearchResults(joinSongs(song3))
	page.click(insertButton)
	page.checkPlaylist(joinSongs(song1, song3, song2), hasActive(0))
	show()
	page.checkPresentation(&song1, &song3)

	// Skip to the next song.
	next()
	page.checkPresentation(&song3, &song2)

	// Skip to the last song. Displaying the update window should hide the presentation layer.
	next()
	page.checkPresentation(&song2, nil)
	rate()
	page.checkPresentation(nil, nil)

	// The presentation layer should be displayable via the menu too, and clicking on the layer
	// should hide it.
	page.click(menuButton)
	page.click(menuPresent)
	page.checkPresentation(&song2, nil)
	page.click(presentationLayer)
	page.checkPresentation(nil, nil)
}

func TestStats(t *testing.T) {
	page, done := initWebTest(t)
	defer done()

	song1 := newSong("artist", "track1", "album1", withRating(0.5), withLength(7200),
		withPlays(time.Unix(86400, 0)))
	song2 := newSong("artist", "track2", "album1", withRating(1.0), withLength(201))
	importSongs(song1, song2)
	tester.UpdateStats()

	page.click(menuButton)
	page.click(menuStats)
	for _, tc := range []struct{ name, val string }{
		{"Songs", "2"},
		{"★★★★★", "1"},
		{"★★★★", "0"},
		{"★★★", "1"},
		{"★★", "0"},
		{"★", "0"},
		{"Unrated", "0"},
		{"Albums", "1"},
		{"Duration", "0.1 days"},
		{"1970", "1"},
	} {
		page.checkText(statsDialog, fmt.Sprintf(`\s+%s\s+%s\s+`, tc.name, tc.val))
	}
}

func TestUnit(t *testing.T) {
	// We don't care about initializing the page object, but we want to write a header
	// to the browser log.
	_, done := initWebTest(t)
	defer done()

	// Start an HTTP server that serves both the web interface and the unit test files.
	fs := unionFS{[]http.Dir{http.Dir("unit"), http.Dir("../../web")}}
	srv := httptest.NewServer(http.FileServer(fs))
	defer srv.Close()
	if err := webDrv.Get(srv.URL); err != nil {
		t.Fatalf("Failed navigating to %v: %v", srv.URL, err)
	}

	// WebDriver apparently blocks internally while executing scripts, so it seems like we
	// unfortunately can't just start a goroutine to stream logs via copyBrowserLogs.
	out, err := webDrv.ExecuteScriptAsyncRaw(
		// ExecuteScriptAsync injects a 'done' callback as the final argument to the called code.
		fmt.Sprintf(`const done = arguments[0];
		const results = await window.runTests(%q);
		done(results);`, unitTestRegexp), nil)
	if err != nil {
		t.Fatalf("Failed running tests: %v", err)
	}

	// The outer object with a 'value' property gets added by Selenium.
	var results struct {
		Value []struct {
			Name   string `json:"name"` // "suite.test"
			Errors []struct {
				Src string `json:"src"`
				Msg string `json:"msg"`
			} `json:"errors"`
		} `json:"value"`
	}
	if err := json.Unmarshal(out, &results); err != nil {
		t.Fatalf("Failed unmarshaling test results %q: %v", string(out), err)
	}

	// Some tests intentionally fail in order to exercise test.js.
	wantErrors := map[string][]string{
		"example.syncErrors": {
			"Got true (boolean); want false (boolean)",
			"Got true (boolean); want 1 (number)",
			"Got 1 (number); want 2 (number)",
			"Got null (object); want false (boolean)",
			"Got null (object); want undefined (undefined)",
			`Value is "foo" (string); want "bar" (string)`,
			`Got [4,"foo"] (object); want [4,"bar"] (object)`,
			`Got {"a":2} (object); want {"b":2} (object)`,
		},
		"example.syncFatal":             {"Fatal: Intentional (exception)"},
		"example.syncException":         {"Error: Intentional (exception)"},
		"example.asyncEarlyFatal":       {"Fatal: Intentional (exception)"},
		"example.asyncEarlyException":   {"Error: Intentional (exception)"},
		"example.asyncEarlyReject":      {"Unhandled rejection: Intentional"},
		"example.asyncTimeoutFatal":     {"Fatal: Intentional (exception)"},
		"example.asyncTimeoutException": {"Error: Intentional (exception)"},
		"example.asyncTimeoutReject":    {"Unhandled rejection: Intentional"},
		"example.doneEarlyFatal":        {"Fatal: Intentional (exception)"},
		"example.doneEarlyException":    {"Error: Intentional (exception)"},
		"example.doneTimeoutFatal":      {"Fatal: Intentional (exception)"},
		"example.doneTimeoutException":  {"Error: Intentional (exception)"},
		"example.doneTimeoutReject":     {"Unhandled rejection: Intentional"},
	}
	gotErrors := make(map[string][]string)

	for _, res := range results.Value {
		if _, ok := wantErrors[res.Name]; ok {
			msgs := make([]string, 0, len(res.Errors))
			for _, err := range res.Errors {
				// TODO: Check err.Src.
				msgs = append(msgs, err.Msg)
			}
			gotErrors[res.Name] = msgs
			continue
		}

		for _, err := range res.Errors {
			pre := res.Name
			if err.Src != "" {
				pre += ": " + err.Src
			}
			t.Errorf("%v: %v", pre, err.Msg)
		}
	}

	// Check that we got expected errors.
	re := regexp.MustCompile(unitTestRegexp)
	for test, want := range wantErrors {
		if !re.MatchString(test) {
			continue // won't see error if test was skipped
		}
		if got := gotErrors[test]; !reflect.DeepEqual(got, want) {
			t.Errorf("Got %q errors %q; want %q", test, got, want)
		}
	}
}

// unionFS implements http.FileSystem using layered http.Dirs.
type unionFS struct {
	dirs []http.Dir
}

func (fs unionFS) Open(name string) (http.File, error) {
	var err error
	for _, dir := range fs.dirs {
		var f http.File
		if f, err = dir.Open(name); err == nil {
			return f, nil
		}
	}
	return nil, err
}
