// Copyright 2021 Daniel Erat.
// All rights reserved.

// Package web contains Selenium-based tests of the web interface.
package web

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/derat/nup/server/config"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
	slog "github.com/tebeka/selenium/log"
)

var (
	// Globals shared across all tests.
	serverURL  string             // slash-terminated URL of App Engine server
	musicSrv   *httptest.Server   // HTTP server for music files
	webDrv     selenium.WebDriver // talks to browser using ChromeDriver
	tester     *test.Tester       // interacts with App Engine server
	browserLog *os.File           // contains log messages from browser

	// Pull some stuff into our namespace for convenience.
	file0s  = test.Song0s.Filename
	file1s  = test.Song1s.Filename
	file5s  = test.Song5s.Filename
	file10s = test.Song10s.Filename
)

func TestMain(m *testing.M) {
	binDir := flag.String("bin-dir", "", "Directory containing dump_music (empty to search $PATH)")
	debug := flag.Bool("debug", false, "Print Selenium debug logs to stderr")
	headless := flag.Bool("headless", true, "Run Chrome headlessly using Xvfb")
	flag.StringVar(&serverURL, "server", "http://localhost:8080/", "Slash-terminated URL of dev_appserver.py instance")
	flag.Parse()

	os.Exit(func() int {
		opts := []selenium.ServiceOption{}
		if *debug {
			selenium.SetDebug(true)
			opts = append(opts, selenium.Output(os.Stderr))
		}
		if *headless {
			opts = append(opts, selenium.StartFrameBuffer())
		}

		chromeDrvPort := findUnusedPort()
		svc, err := selenium.NewChromeDriverService("/usr/local/bin/chromedriver",
			chromeDrvPort, opts...)
		if err != nil {
			log.Fatal("Failed starting ChromeDriver service: ", err)
		}
		defer svc.Stop()

		caps := selenium.Capabilities{}
		caps.AddChrome(chrome.Capabilities{
			Args: []string{"--autoplay-policy=no-user-gesture-required"},
		})
		caps.SetLogLevel(slog.Browser, slog.All)
		webDrv, err = selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d/wd/hub", chromeDrvPort))
		if err != nil {
			log.Fatal("Failed connecting to Selenium: ", err)
		}
		defer webDrv.Quit()

		// Create a file containing messages logged by the web interface.
		if browserLog, err = ioutil.TempFile("", "nup_web_test-*.txt"); err != nil {
			log.Fatal("Failed creating browser log: ", err)
		}
		fmt.Fprintf(os.Stderr, "Writing browser logs to %v\n", browserLog.Name())
		defer browserLog.Close()
		writeLogHeader("Running web tests against " + serverURL)
		defer writeBrowserLogs()

		tester = test.NewTester(serverURL, *binDir)
		defer tester.Close()
		if err := tester.PingServer(); err != nil {
			log.Fatal("Failed pinging server (is dev_appserver.py running?): ", err)
		}

		// Serve music files in the background.
		test.CopySongs(tester.MusicDir, file0s, file1s, file5s, file10s)
		fs := http.FileServer(http.Dir(tester.MusicDir))
		musicSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Origin", serverURL)
			fs.ServeHTTP(w, r)
		}))
		defer musicSrv.Close()

		sendConfig(updatesSucceed)
		defer tester.SendConfig(nil)

		// WebDriver only allows setting cookies for the currently-loaded page,
		// so we need to load the site before setting the cookie that lets us
		// skip authentication.
		if err := webDrv.Get(serverURL); err != nil {
			log.Fatalf("Failed loading %v: %v", serverURL, err)
		}
		if err := webDrv.AddCookie(&selenium.Cookie{
			Name:   config.WebDriverCookie,
			Value:  "1", // arbitrary
			Expiry: math.MaxUint32,
		}); err != nil {
			log.Fatalf("Failed setting %q cookie: %v", config.WebDriverCookie, err)
		}
		if err := webDrv.Get(serverURL); err != nil {
			log.Fatalf("Failed reloading %v: %v", serverURL, err)
		}

		return m.Run()
	}())
}

// writeLogHeader writes s and a line of dashes to browserLog.
func writeLogHeader(s string) {
	fmt.Fprintf(browserLog, "%s\n%s\n", s, strings.Repeat("-", 80))
}

// writeBrowserLogs gets new log messages from the browser and writes them to browserLog.
func writeBrowserLogs() {
	msgs, err := webDrv.Log(slog.Browser)
	if err != nil {
		fmt.Fprintf(browserLog, "Failed getting browser logs: %v\n", err)
		return
	}

	for _, msg := range msgs {
		ts := msg.Timestamp.Format("15:04:05.000")

		// Log messages usually look like this:
		//  music-searcher.js 478:18 "Got response with 1 song(s)"
		// Try to make them more readable by dropping the server URL from the
		// beginning of the filename and lining up the actual messages.
		text := strings.TrimPrefix(msg.Message, serverURL)
		if parts := strings.SplitN(text, " ", 3); len(parts) == 3 &&
			strings.HasSuffix(parts[0], ".js") && strings.Contains(parts[1], ":") {
			text = fmt.Sprintf("%-30s %s", parts[0]+" "+parts[1], strings.Trim(parts[2], `"`))
		}

		fmt.Fprintf(browserLog, "%s %-7s %s\n", ts, msg.Level, text)
	}
}

// initWebTest should be called at the beginning of each test.
// The returned object is used to interact with the web interface via Selenium.
func initWebTest(t *testing.T) *page {
	// Copy any browser logs from the previous test and write a header.
	writeBrowserLogs()
	io.WriteString(browserLog, "\n")
	writeLogHeader(t.Name())

	tester.ClearData()
	return newPage(t, webDrv)
}

// updatePolicy is passed to sendConfig to control the server's handling of updates.
type updatePolicy bool

const (
	// Values correspond to server's ForceUpdateFailures field.
	updatesSucceed updatePolicy = false
	updatesFail    updatePolicy = true
)

// sendConfig updates the server's configuration.
func sendConfig(p updatePolicy) {
	tester.SendConfig(&config.Config{
		SongBaseURL:         musicSrv.URL + "/",
		CoverBaseURL:        "",
		ForceUpdateFailures: bool(p),
		Presets: []config.SearchPreset{
			{
				Name:       "instrumental old",
				Tags:       "instrumental",
				MinRating:  4,
				LastPlayed: 6,
				Shuffle:    true,
				Play:       true,
			},
			{
				Name:      "mellow",
				Tags:      "mellow",
				MinRating: 4,
				Shuffle:   true,
				Play:      true,
			},
			{
				Name:        "new albums",
				FirstPlayed: 3,
				FirstTrack:  true,
			},
			{
				Name:    "unrated",
				Unrated: true,
				Play:    true,
			},
		},
	})
}

// importSongs posts the supplied db.Song or []db.Song args to the server.
func importSongs(songs ...interface{}) {
	tester.PostSongs(joinSongs(songs...), true, 0)
}

func TestKeywordQuery(t *testing.T) {
	page := initWebTest(t)
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
	page := initWebTest(t)
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
	page := initWebTest(t)
	song1 := newSong("a", "t", "al1", withRating(0.0))
	song2 := newSong("a", "t", "al2", withRating(0.25))
	song3 := newSong("a", "t", "al3", withRating(0.5))
	song4 := newSong("a", "t", "al4", withRating(0.75))
	song5 := newSong("a", "t", "al5", withRating(1.0))
	song6 := newSong("a", "t", "al6", withRating(-1.0))
	allSongs := joinSongs(song1, song2, song3, song4, song5, song6)
	importSongs(allSongs)

	page.setStage(oneStar)
	page.setText(keywordsInput, "t") // need to set at least one search term
	page.click(searchButton)
	page.checkSearchResults(allSongs)

	page.click(resetButton)
	for _, tc := range []struct {
		option string
		want   []db.Song
	}{
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
	page := initWebTest(t)
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

func TestMaxPlaysQuery(t *testing.T) {
	page := initWebTest(t)
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
	page := initWebTest(t)
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
	page := initWebTest(t)
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
	page := initWebTest(t)
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
	page := initWebTest(t)
	song1 := newSong("artist", "track1", "album", withTrack(1), withFilename(file5s))
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
	page.checkSong(song2, isPaused(false))
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
	page.checkSong(song2, isPaused(false))
	page.checkPlaylist(joinSongs(song1, song2), hasActive(1))
}

func TestContextMenu(t *testing.T) {
	page := initWebTest(t)
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
	page.checkPlaylist(songs, hasActive(2))

	page.rightClickSongRow(playlistTable, 0)
	page.checkPlaylist(songs, hasMenu(0))
	page.click(menuRemove)
	page.checkSong(song3, isPaused(false))
	page.checkPlaylist(joinSongs(song2, song3, song4, song5), hasActive(1))

	page.rightClickSongRow(playlistTable, 1)
	page.checkPlaylist(joinSongs(song2, song3, song4, song5), hasMenu(1))
	page.click(menuTruncate)
	page.checkSong(song2, isPaused(true))
	page.checkPlaylist(joinSongs(song2), hasActive(0))
}

func TestDisplayTimeWhilePlaying(t *testing.T) {
	page := initWebTest(t)
	song := newSong("ar", "t", "al", withFilename(file5s))
	importSongs(song)

	page.setText(keywordsInput, song.Artist)
	page.click(luckyButton)
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:00 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:01 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:02 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:03 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTimeStr("[ 0:04 / 0:05 ]"))
	page.checkSong(song, isEnded(true), isPaused(true), hasTimeStr("[ 0:05 / 0:05 ]"))
}

func TestReportPlayed(t *testing.T) {
	page := initWebTest(t)
	song1 := newSong("a", "t1", "al", withTrack(1), withFilename(file5s))
	song2 := newSong("a", "t2", "al", withTrack(2), withFilename(file1s))
	importSongs(song1, song2)

	// Skip the first song early on, but listen to all of the second song.
	page.setText(keywordsInput, song1.Artist)
	page.click(luckyButton)
	page.checkSong(song1, isPaused(false))
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
	page := initWebTest(t)
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
	page := initWebTest(t)
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
	page := initWebTest(t)
	song := newSong("ar", "t1", "al", withFilename(file1s),
		withRating(0.5), withTags("rock", "guitar"))
	importSongs(song)

	// Configure the server to reject updates and play the song.
	sendConfig(updatesFail)
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
	sendConfig(updatesSucceed)
	checkServerSong(t, song, hasSrvRating(0.75), hasSrvTags("jazz", "mellow"),
		hasSrvPlay(firstLower, firstUpper))

	// Queue some more failed updates.
	sendConfig(updatesFail)
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
	sendConfig(updatesSucceed)
	checkServerSong(t, song, hasSrvRating(0.25), hasSrvTags("lively", "soul"),
		hasSrvPlay(firstLower, firstUpper), hasSrvPlay(secondLower, secondUpper))

	// In the case of multiple queued updates, the last one should take precedence.
	sendConfig(updatesFail)
	page.setText(keywordsInput, song.Artist)
	page.click(luckyButton)
	page.checkSong(song)
	for _, r := range [][]loc{ratingThreeStars, ratingFourStars, ratingFiveStars} {
		page.click(coverImage)
		page.click(r)
		page.click(updateCloseImage)
		time.Sleep(100 * time.Millisecond)
	}
	sendConfig(updatesSucceed)
	checkServerSong(t, song, hasSrvRating(1.0))
}

func TestEditTagsAutocomplete(t *testing.T) {
	page := initWebTest(t)
	song1 := newSong("ar", "t1", "al", withTags("a0", "a1", "b"))
	song2 := newSong("ar", "t2", "al", withTags("c0", "c1", "d"))
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
}

func TestOptions(t *testing.T) {
	page := initWebTest(t)
	show := func() { page.emitKeyDown("o", 79, true /* alt */) }

	show()
	page.checkAttr(gainTypeSelect, "value", gainAlbumValue)
	page.clickOption(gainTypeSelect, gainTrack)
	page.checkAttr(gainTypeSelect, "value", gainTrackValue)

	// I *think* that this clicks the middle of the range. This might be a
	// no-op since it should be 0, which is the default. :-/
	page.click(preAmpRange)
	origPreAmp := page.getAttrOrFail(page.getOrFail(preAmpRange), "value", false)

	page.click(optionsOKButton)
	page.checkGone(optionsOKButton)

	// Escape should dismiss the dialog.
	show()
	page.sendKeys(body, selenium.EscapeKey, false)
	page.checkGone(optionsOKButton)

	page.reload()
	show()
	page.checkAttr(gainTypeSelect, "value", gainTrackValue)
	page.checkAttr(preAmpRange, "value", origPreAmp)
	page.click(optionsOKButton)
	page.checkGone(optionsOKButton)
}

func TestPresets(t *testing.T) {
	page := initWebTest(t)
	now := time.Now()
	old := now.Add(-2 * 365 * 24 * time.Hour)
	song1 := newSong("a", "t1", "unrated")
	song2 := newSong("a", "t1", "new", withRating(0.25), withTrack(1), withDisc(1), withPlays(now))
	song3 := newSong("a", "t2", "new", withRating(1.0), withTrack(2), withDisc(1), withPlays(now))
	song4 := newSong("a", "t1", "old", withRating(0.75), withPlays(old))
	song5 := newSong("a", "t2", "old", withRating(0.75), withTags("instrumental"), withPlays(old))
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

	if active, err := page.wd.ActiveElement(); err != nil {
		t.Error("Failed getting active element: ", err)
	} else if reflect.DeepEqual(active, page.getOrFail(presetSelect)) {
		t.Error("Preset select still focused after click")
	}
}

func TestPresentation(t *testing.T) {
	page := initWebTest(t)
	show := func() { page.emitKeyDown("v", 86, true /* alt */) }
	next := func() { page.emitKeyDown("n", 78, true /* alt */) }

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

	// Skip to the last song.
	next()
	page.checkPresentation(&song2, nil)
	page.sendKeys(body, selenium.EscapeKey, false)
	page.checkPresentation(nil, nil)
}
