// Copyright 2021 Daniel Erat.
// All rights reserved.

// Package main runs dev_appserver with example data.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/derat/nup/server/config"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/test"

	"golang.org/x/sys/unix"
)

func main() {
	code, err := run()
	if err != nil {
		log.Print("Failed serving example data: ", err)
	}
	os.Exit(code)
}

func run() (int, error) {
	binDir := flag.String("bin-dir", "",
		"Directory containing nup executables (empty to search $PATH)")
	debugApp := flag.Bool("debug-app", true, "Show dev_appserver output")
	email := flag.String("email", "test@example.com", "Email address for login")
	port := flag.Int("port", 8080, "HTTP port for app")
	flag.Parse()

	test.HandleSignals(unix.SIGINT, unix.SIGTERM)

	exampleDir, err := test.CallerDir()
	if err != nil {
		return -1, err
	}
	fileSrv := test.ServeFiles(exampleDir)
	defer fileSrv.Close()
	log.Print("File server is listening at ", fileSrv.URL)

	log.Print("Starting dev_appserver")
	appSrv, err := test.NewDevAppserver(*port, *debugApp)
	if err != nil {
		return -1, fmt.Errorf("dev_appserver: %v", err)
	}
	defer appSrv.Close()
	appURL := appSrv.URL()
	log.Print("dev_appserver is listening at ", appURL)

	tester := test.NewTester(nil, appURL, test.TesterConfig{
		MusicDir: filepath.Join(exampleDir, "music"),
		CoverDir: filepath.Join(exampleDir, "covers"),
		BinDir:   *binDir,
	})
	defer tester.Close()
	tester.SendConfig(&config.Config{
		GoogleUsers:    []string{*email},
		BasicAuthUsers: []config.BasicAuthInfo{{Username: test.Username, Password: test.Password}},
		SongBaseURL:    fileSrv.URL + "/music/",
		CoverBaseURL:   fileSrv.URL + "/covers/",
		Presets:        presets,
	})
	tester.ImportSongsFromJSONFile(songs)

	time.Sleep(time.Minute)
	return 0, nil
}

var presets = []config.SearchPreset{
	{
		Name:       "old",
		MinRating:  4,
		LastPlayed: 6,
		Shuffle:    true,
		Play:       true,
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
}

var songs = []db.Song{
	{
		SHA1:     "5439c23b4eae55f9dcd145fc3284cd8fa05696ff",
		SongID:   "1",
		Filename: "400x400.mp3",
		Artist:   "Artist",
		Title:    "400x400",
		Album:    "400x400",
		AlbumID:  "400-400",
		Track:    1,
		Disc:     1,
		Length:   1,
		Rating:   1,
	},
	{
		SHA1:     "74057828e637cdaa60338c220ad3f59e4262c3f2",
		SongID:   "2",
		Filename: "800x800.mp3",
		Artist:   "Artist",
		Title:    "800x800",
		Album:    "800x800",
		AlbumID:  "800-800",
		Track:    1,
		Disc:     1,
		Length:   1,
		Rating:   1,
	},
	{
		SHA1:     "0358287496e475b2e812e882b7885be665b604d1",
		SongID:   "3",
		Filename: "40x40.mp3",
		Artist:   "Artist",
		Title:    "40x40",
		Album:    "40x40",
		AlbumID:  "40-40",
		Track:    1,
		Disc:     1,
		Length:   1,
		Rating:   1,
	},
	{
		SHA1:     "22aa5c0ad793e7a86852cfb7e0aa6b41aa98e99c",
		SongID:   "4",
		Filename: "360x400.mp3",
		Artist:   "Artist",
		Title:    "360x400",
		Album:    "360x400",
		AlbumID:  "360-400",
		Track:    1,
		Disc:     1,
		Length:   1,
		Rating:   1,
	},
	{
		SHA1:     "11551e3ebd919e5ef2329d9d3716c3e453d98c7d",
		SongID:   "5",
		Filename: "400x360.mp3",
		Artist:   "Artist",
		Title:    "400x360",
		Album:    "400x360",
		AlbumID:  "400-360",
		Track:    1,
		Disc:     1,
		Length:   1,
		Rating:   1,
	},
}
