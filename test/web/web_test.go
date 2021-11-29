// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"log"
	"math"
	"os"
	"testing"
	"time"

	"github.com/derat/nup/server/auth"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"

	"github.com/tebeka/selenium"
)

const (
	chromeDriverPort = 8088
	serverURL        = "http://localhost:8080/"
)

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

		caps := selenium.Capabilities{"browserName": "chrome"}
		wd, err = selenium.NewRemote(caps, fmt.Sprintf("http://localhost:%d/wd/hub", chromeDriverPort))
		if err != nil {
			log.Fatal("Failed connecting to Selenium: ", err)
		}
		defer wd.Quit()

		tester = test.NewTester(serverURL, "")
		defer tester.Close()

		// WebDriver only allows setting cookies for the currently-loaded page,
		// so we need to load the site before setting the cookie that lets us
		// skip authentication.
		if err := wd.Get(serverURL); err != nil {
			log.Fatalf("Failed loading %v: %v", serverURL, err)
		}
		if err := wd.AddCookie(&selenium.Cookie{
			Name:   auth.WebDriverCookie,
			Value:  "1", // arbitrary
			Expiry: math.MaxUint32,
		}); err != nil {
			log.Fatalf("Failed setting %q cookie: %v", auth.WebDriverCookie, err)
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
		page.setText(keywordsInput, tc.kw)
		page.click(searchButton)
		page.checkSearchResults(tc.want, nil, tc.kw)
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
		page.setText(tagsInput, tc.tags)
		page.click(searchButton)
		page.checkSearchResults(tc.want, nil, tc.tags)
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

	page.setText(keywordsInput, "t") // need to set at least one search term
	page.click(searchButton)
	page.checkSearchResults(allSongs, nil, "one star")

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
		page.clickOption(minRatingSelect, tc.option)
		page.click(searchButton)
		page.checkSearchResults(tc.want, nil, tc.option)
	}

	page.click(resetButton)
	page.click(unratedCheckbox)
	page.click(searchButton)
	page.checkSearchResults(joinSongs(song6), nil, "unrated")
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
	page.checkSearchResults(joinSongs(album1[0], album2[0]), nil, "first tracks")
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
		page.setText(maxPlaysInput, tc.plays)
		page.click(searchButton)
		page.checkSearchResults(tc.want, nil, tc.plays)
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
		page.clickOption(firstPlayedSelect, tc.first)
		page.clickOption(lastPlayedSelect, tc.last)
		page.click(searchButton)
		page.checkSearchResults(tc.want, nil, fmt.Sprintf("%s / %s", tc.first, tc.last))
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
	page.checkSearchResults(songs, []bool{true, true, true}, "")
	page.checkCheckbox(searchResultsCheckbox, checked)

	// Click the top checkbox to deselect all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, []bool{false, false, false}, "")
	page.checkCheckbox(searchResultsCheckbox, 0)

	// Click it again to select all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, []bool{true, true, true}, "")
	page.checkCheckbox(searchResultsCheckbox, checked)

	// Click the first song to deselect it.
	page.clickSearchResultsSongCheckbox(0, "")
	page.checkSearchResults(songs, []bool{false, true, true}, "")
	page.checkCheckbox(searchResultsCheckbox, checked|transparent)

	// Click the top checkbox to deselect all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, []bool{false, false, false}, "")
	page.checkCheckbox(searchResultsCheckbox, 0)

	// Click the first and second songs individually to select them.
	page.clickSearchResultsSongCheckbox(0, "")
	page.clickSearchResultsSongCheckbox(1, "")
	page.checkSearchResults(songs, []bool{true, true, false}, "")
	page.checkCheckbox(searchResultsCheckbox, checked|transparent)

	// Click the third song to select it as well.
	page.clickSearchResultsSongCheckbox(2, "")
	page.checkSearchResults(songs, []bool{true, true, true}, "")
	page.checkCheckbox(searchResultsCheckbox, checked)

	// Shift-click from the first to third song to select all songs.
	page.click(searchResultsCheckbox)
	page.checkSearchResults(songs, []bool{false, false, false}, "")
	page.clickSearchResultsSongCheckbox(0, selenium.ShiftKey)
	page.clickSearchResultsSongCheckbox(2, selenium.ShiftKey)
	page.checkSearchResults(songs, []bool{true, true, true}, "")
	page.checkCheckbox(searchResultsCheckbox, checked)
}
