// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/derat/nup/server/config"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
)

const (
	chromeDriverPort = 8088
	musicServerAddr  = "localhost:8089"
	serverURL        = "http://localhost:8080/"
)

// Globals shared across all tests.
var wd selenium.WebDriver
var tester *test.Tester

func TestMain(m *testing.M) {
	os.Exit(func() int {
		//selenium.SetDebug(true)
		opts := []selenium.ServiceOption{
			//selenium.StartFrameBuffer(),
			//selenium.Output(os.Stderr),
		}
		svc, err := selenium.NewChromeDriverService("/usr/local/bin/chromedriver", chromeDriverPort, opts...)
		if err != nil {
			log.Fatal("Failed starting ChromeDriver service: ", err)
		}
		defer svc.Stop()

		caps := selenium.Capabilities{}
		caps.AddChrome(chrome.Capabilities{
			Args: []string{"--autoplay-policy=no-user-gesture-required"},
		})
		wd, err = selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d/wd/hub", chromeDriverPort))
		if err != nil {
			log.Fatal("Failed connecting to Selenium: ", err)
		}
		defer wd.Quit()

		tester = test.NewTester(serverURL, "") // TODO: Pass binary path?
		defer tester.Close()

		// Serve music files in the background.
		test.CopySongs(tester.MusicDir, song1s.Filename, song5s.Filename, song10s.Filename)
		fs := http.FileServer(http.Dir(tester.MusicDir))
		ms := &http.Server{
			Addr: musicServerAddr,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Origin", serverURL)
				fs.ServeHTTP(w, r)
			}),
		}
		go func() {
			if err := ms.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal("Failed starting music server")
			}
		}()
		defer ms.Shutdown(context.Background())

		tester.SendConfig(&config.Config{
			SongBaseURL:  fmt.Sprintf("http://%s/", musicServerAddr),
			CoverBaseURL: "",
		})
		defer tester.SendConfig(nil)

		// WebDriver only allows setting cookies for the currently-loaded page,
		// so we need to load the site before setting the cookie that lets us
		// skip authentication.
		if err := wd.Get(serverURL); err != nil {
			log.Fatalf("Failed loading %v: %v", serverURL, err)
		}
		if err := wd.AddCookie(&selenium.Cookie{
			Name:   config.WebDriverCookie,
			Value:  "1", // arbitrary
			Expiry: math.MaxUint32,
		}); err != nil {
			log.Fatalf("Failed setting %q cookie: %v", config.WebDriverCookie, err)
		}
		if err := wd.Get(serverURL); err != nil {
			log.Fatalf("Failed reloading %v: %v", serverURL, err)
		}

		return m.Run()
	}())
}

func initWebTest(t *testing.T) *page {
	tester.ClearData()
	return newPage(t, wd)
}

type songUserData struct {
	rating float64
	tags   []string
	plays  [][2]time.Time // lower/upper bounds for timestamps
}

func newSongUserData(rating float64, tags []string, plays ...[2]time.Time) songUserData {
	d := songUserData{rating, tags, plays}
	sort.Slice(d.plays, func(i, j int) bool { return d.plays[i][0].Before(d.plays[j][0]) })
	return d
}

func checkServerUserData(t *testing.T, want map[string]songUserData) {
	getData := func() map[string]songUserData {
		m := make(map[string]songUserData)
		for _, s := range tester.DumpSongs(test.KeepIDs) {
			var plays [][2]time.Time
			for _, p := range s.Plays {
				plays = append(plays, [2]time.Time{p.StartTime, p.StartTime})
			}
			data := newSongUserData(s.Rating, s.Tags, plays...)
			m[s.SHA1] = data
		}
		return m
	}
	if err := wait(func() error {
		got := getData()
		for sha1, wd := range want {
			gd, ok := got[sha1]
			if !ok {
				return fmt.Errorf("%v missing from server", sha1)
			}
			if gd.rating != wd.rating {
				return fmt.Errorf("%v rating is %.2f; want %.2f", sha1, gd.rating, wd.rating)
			}
			sort.Strings(gd.tags)
			sort.Strings(wd.tags)
			if !reflect.DeepEqual(gd.tags, wd.tags) {
				return fmt.Errorf("%v tags are %v; want %v", sha1, gd.tags, wd.tags)
			}

			if len(gd.plays) != len(wd.plays) {
				return fmt.Errorf("%v has %v play(s); want %v", sha1, len(gd.plays), len(wd.plays))
			}
			for i := range gd.plays {
				if gp, wp := gd.plays[i], wd.plays[i]; gp[0].Before(wp[0]) || gp[0].After(wp[1]) {
					return fmt.Errorf("%v play %d has time %v outside of [%v, %v]",
						sha1, i, gp[0], wp[0], wp[1])
				}
			}
		}
		return nil
	}); err != nil {
		// TODO: Consider dumping all data.
		msg := fmt.Sprintf("Bad server user data for %v: %v\n", caller(), err)
		t.Fatal(msg)
	}
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
	tester.PostSongs(joinSongs(album1, album2, album3), true, 0)

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
	tester.PostSongs(joinSongs(song1, song2, song3), true, 0)

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
	tester.PostSongs(allSongs, true, 0)

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
	tester.PostSongs(joinSongs(album1, album2), true, 0)

	page.click(firstTrackCheckbox)
	page.click(searchButton)
	page.checkSearchResults(joinSongs(album1[0], album2[0]))
}

func TestMaxPlaysQuery(t *testing.T) {
	page := initWebTest(t)
	song1 := newSong("ar1", "ti1", "al1", withPlays(1, 2))
	song2 := newSong("ar2", "ti2", "al2", withPlays(1, 2, 3))
	song3 := newSong("ar3", "ti3", "al3")
	tester.PostSongs(joinSongs(song1, song2, song3), true, 0)

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

	const day = 86400
	now := time.Now().Unix()
	song1 := newSong("ar1", "ti1", "al1", withPlays(now-5*day))
	song2 := newSong("ar2", "ti2", "al2", withPlays(now-90*day))
	tester.PostSongs(joinSongs(song1, song2), true, 0)

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
	tester.PostSongs(songs, true, 0)

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
	page.clickSearchResultsSongCheckbox(0, "")
	page.checkSearchResults(songs, hasChecked(false, true, true))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked|checkboxTransparent)

	// Click the top checkbox to deselect all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, hasChecked(false, false, false))
	page.checkCheckbox(searchResultsCheckbox, 0)

	// Click the first and second songs individually to select them.
	page.clickSearchResultsSongCheckbox(0, "")
	page.clickSearchResultsSongCheckbox(1, "")
	page.checkSearchResults(songs, hasChecked(true, true, false))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked|checkboxTransparent)

	// Click the third song to select it as well.
	page.clickSearchResultsSongCheckbox(2, "")
	page.checkSearchResults(songs, hasChecked(true, true, true))
	page.checkCheckbox(searchResultsCheckbox, checkboxChecked)

	// Shift-click from the first to third song to select all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, hasChecked(false, false, false))
	page.clickSearchResultsSongCheckbox(0, selenium.ShiftKey)
	page.clickSearchResultsSongCheckbox(2, selenium.ShiftKey)
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
	tester.PostSongs(joinSongs(song1, song2, song3, song4, song5, song6), true, 0)

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
	song1 := newSong("artist", "track1", "album", withTrack(1), withFilename(song5s.Filename))
	song2 := newSong("artist", "track2", "album", withTrack(2), withFilename(song1s.Filename))
	tester.PostSongs(joinSongs(song1, song2), true, 0)

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
	tester.PostSongs(songs, true, 0)

	page.setText(keywordsInput, song1.Album)
	page.click(luckyButton)
	page.checkSong(song1, isPaused(false))
	page.checkPlaylist(songs, hasActive(0))

	page.rightClickPlaylistSong(3)
	page.checkPlaylist(songs, hasMenu(3))
	page.click(menuPlay)
	page.checkSong(song4, isPaused(false))
	page.checkPlaylist(songs, hasActive(3))

	page.rightClickPlaylistSong(2)
	page.checkPlaylist(songs, hasMenu(2))
	page.click(menuPlay)
	page.checkSong(song3, isPaused(false))
	page.checkPlaylist(songs, hasActive(2))

	page.rightClickPlaylistSong(0)
	page.checkPlaylist(songs, hasMenu(0))
	page.click(menuRemove)
	page.checkSong(song3, isPaused(false))
	page.checkPlaylist(joinSongs(song2, song3, song4, song5), hasActive(1))

	page.rightClickPlaylistSong(1)
	page.checkPlaylist(joinSongs(song2, song3, song4, song5), hasMenu(1))
	page.click(menuTruncate)
	page.checkSong(song2, isPaused(true))
	page.checkPlaylist(joinSongs(song2), hasActive(0))
}

func TestDisplayTimeWhilePlaying(t *testing.T) {
	page := initWebTest(t)
	song := newSong("ar", "t", "al", withFilename(song5s.Filename))
	tester.PostSongs(joinSongs(song), true, 0)

	page.setText(keywordsInput, song.Artist)
	page.click(luckyButton)
	page.checkSong(song, isPaused(false), hasTime("[ 0:00 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTime("[ 0:01 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTime("[ 0:02 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTime("[ 0:03 / 0:05 ]"))
	page.checkSong(song, isPaused(false), hasTime("[ 0:04 / 0:05 ]"))
	page.checkSong(song, isEnded(true), isPaused(true), hasTime("[ 0:05 / 0:05 ]"))
}

func TestReportPlayed(t *testing.T) {
	page := initWebTest(t)
	song1 := newSong("a", "t1", "al", withTrack(1), withFilename(song5s.Filename))
	song2 := newSong("a", "t2", "al", withTrack(2), withFilename(song1s.Filename))
	tester.PostSongs(joinSongs(song1, song2), true, 0)

	// Skip the first song early on, but listen to all of the second song.
	page.setText(keywordsInput, song1.Artist)
	page.click(luckyButton)
	page.checkSong(song1, isPaused(false))
	song2Lower := time.Now()
	page.click(nextButton)
	page.checkSong(song2, isEnded(true))
	song2Upper := time.Now()

	// Only the second song should've been reported.
	checkServerUserData(t, map[string]songUserData{
		song1.SHA1: newSongUserData(-1.0, nil),
		song2.SHA1: newSongUserData(-1.0, nil, [2]time.Time{song2Lower, song2Upper}),
	})

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
	checkServerUserData(t, map[string]songUserData{
		song1.SHA1: newSongUserData(-1.0, nil, [2]time.Time{song1Lower, song1Upper}),
		song2.SHA1: newSongUserData(-1.0, nil, [2]time.Time{song2Lower, song2Upper}),
	})
}

func TestReportReplay(t *testing.T) {
	page := initWebTest(t)
	song := newSong("a", "t1", "al", withFilename(song1s.Filename))
	tester.PostSongs(joinSongs(song), true, 0)

	// Play the song to completion.
	page.setText(keywordsInput, song.Artist)
	firstLower := time.Now()
	page.click(luckyButton)
	page.checkSong(song, isEnded(true))

	// Replay the song.
	secondLower := time.Now()
	page.click(playPauseButton)

	// Both playbacks should be reported.
	checkServerUserData(t, map[string]songUserData{
		song.SHA1: newSongUserData(-1.0, nil,
			[2]time.Time{firstLower, secondLower},
			[2]time.Time{secondLower, secondLower.Add(2 * time.Second)},
		),
	})
}

func TestRateAndTag(t *testing.T) {
	page := initWebTest(t)
	song := newSong("ar", "t1", "al", withRating(0.5), withTags("rock", "guitar"))
	tester.PostSongs(joinSongs(song), true, 0)

	page.setText(keywordsInput, song.Artist)
	page.click(luckyButton)
	page.click(playPauseButton)
	page.checkSong(song, isPaused(true), hasRating(threeStars),
		hasImgTitle("Rating: ★★★☆☆\nTags: guitar rock"))

	page.click(coverImage)
	page.click(ratingFourStars)
	page.click(updateCloseImage)
	page.checkSong(song, hasRating(fourStars),
		hasImgTitle("Rating: ★★★★☆\nTags: guitar rock"))
	checkServerUserData(t, map[string]songUserData{
		song.SHA1: newSongUserData(0.75, []string{"guitar", "rock"}),
	})

	page.click(coverImage)
	page.sendKeys(editTagsTextarea, " +metal", false)
	page.click(updateCloseImage)
	page.checkSong(song, hasRating(fourStars),
		hasImgTitle("Rating: ★★★★☆\nTags: guitar metal rock"))
	checkServerUserData(t, map[string]songUserData{
		song.SHA1: newSongUserData(0.75, []string{"guitar", "metal", "rock"}),
	})
}
