// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"log"
	"math"
	"os"
	"testing"

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

	album1 := []db.Song{
		newSong("ar1", "ti1", "al1", 1),
		newSong("ar1", "ti2", "al1", 2),
		newSong("ar1", "ti3", "al1", 3),
	}
	album2 := []db.Song{
		newSong("ar2", "ti1", "al2", 1),
		newSong("ar2", "ti2", "al2", 2),
	}
	album3 := []db.Song{
		newSong("artist with space", "ti1", "al3", 1),
	}

	var allSongs []db.Song
	allSongs = append(allSongs, album1...)
	allSongs = append(allSongs, album2...)
	allSongs = append(allSongs, album3...)
	tester.PostSongs(allSongs, true /* replaceUserData */, 0)

	for _, tc := range []struct {
		kw   string
		want []db.Song
	}{
		{"album:al1", album1},
		{"album:al2", album2},
		{"artist:ar1", album1},
		{"artist:\"artist with space\"", album3},
		{"ti2", []db.Song{album1[1], album2[1]}},
		{"AR2 ti1", []db.Song{album2[0]}},
		{"ar1 bogus", []db.Song{}},
	} {
		page.setText(keywordsInput, tc.kw)
		page.click(searchButton)
		page.waitForSearchResults(tc.want)
	}
}
