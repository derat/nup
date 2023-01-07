// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package e2e contains end-to-end tests between the server and command-line tools.
package e2e

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/derat/nup/server/config"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/server/query"
	"github.com/derat/nup/test"

	"golang.org/x/sys/unix"
)

const (
	coverBucket = "cover-bucket"

	guestUsername    = "guest"
	guestPassword    = "guestpw"
	maxGuestRequests = 3
)

var (
	// Pull some stuff into our namespace for convenience.
	Song0s        = test.Song0s
	Song0sUpdated = test.Song0sUpdated
	Song1s        = test.Song1s
	Song5s        = test.Song5s
	Song10s       = test.Song10s
	LegacySong1   = test.LegacySong1
	LegacySong2   = test.LegacySong2

	appURL string // URL of App Engine app
	outDir string // base directory for temp files and logs

	guestExcludedTags = []string{"rock"}
	guestPresets      = []config.SearchPreset{{Name: "custom", MinRating: 4}}
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
	createIndexes := flag.Bool("create-indexes", false, "Update datastore indexes in index.yaml")
	flag.Parse()

	test.HandleSignals([]os.Signal{unix.SIGINT, unix.SIGTERM}, nil)

	var keepOutDir bool
	if outDir, keepOutDir, err = test.OutputDir("e2e_test"); err != nil {
		return -1, err
	}
	defer func() {
		if res == 0 && !keepOutDir {
			log.Print("Removing ", outDir)
			os.RemoveAll(outDir)
		}
	}()
	log.Print("Writing files to ", outDir)

	appLog, err := os.Create(filepath.Join(outDir, "app.log"))
	if err != nil {
		return -1, err
	}
	defer appLog.Close()

	// Serve the test/data/songs directory so the server's /song endpoint will work.
	// This just serves the checked-in data, so it won't necessarily match the songs
	// that a given test has imported into the server.
	songsDir, err := test.SongsDir()
	if err != nil {
		return -1, err
	}
	songsSrv := test.ServeFiles(songsDir)
	defer songsSrv.Close()

	cfg := &config.Config{
		Users: []config.User{
			{Username: test.Username, Password: test.Password, Admin: true},
			{
				Username:     guestUsername,
				Password:     guestPassword,
				Guest:        true,
				Presets:      guestPresets,
				ExcludedTags: guestExcludedTags,
			},
		},
		SongBaseURL:                 songsSrv.URL,
		CoverBaseURL:                songsSrv.URL, // bogus, but no tests request covers
		MaxGuestSongRequestsPerHour: maxGuestRequests,
	}
	storageDir := filepath.Join(outDir, "app_storage")
	srv, err := test.NewDevAppserver(cfg, storageDir, appLog, test.DevAppserverCreateIndexes(*createIndexes))
	if err != nil {
		return -1, fmt.Errorf("dev_appserver: %v", err)
	}
	defer os.RemoveAll(storageDir)
	defer srv.Close()
	appURL = srv.URL()
	log.Print("dev_appserver is listening at ", appURL)

	res = m.Run()
	return res, nil
}

func initTest(t *testing.T) (*test.Tester, func()) {
	tmpDir := filepath.Join(outDir, "tester."+t.Name())
	tester := test.NewTester(t, appURL, tmpDir, test.TesterConfig{})
	tester.PingServer()
	log.Print("Clearing ", appURL)
	tester.ClearData()
	tester.FlushCache(test.FlushAll)

	// Remove the test-specific temp dir since it often ends up holding music files.
	return tester, func() { os.RemoveAll(tmpDir) }
}

func compareQueryResults(expected, actual []db.Song, order test.OrderPolicy) error {
	expectedCleaned := make([]db.Song, len(expected))
	for i := range expected {
		s := expected[i]
		query.CleanSong(&s, 0)

		// Change some stuff back to match the expected values.
		s.SongID = ""
		if len(s.Tags) == 0 {
			s.Tags = nil
		}

		expectedCleaned[i] = s
	}

	actualCleaned := make([]db.Song, len(actual))
	for i := range actual {
		s := actual[i]

		if len(s.SongID) == 0 {
			return fmt.Errorf("song %v (%v) has no ID", i, s.Filename)
		}
		s.SongID = ""

		if len(s.Tags) == 0 {
			s.Tags = nil
		}

		actualCleaned[i] = s
	}

	return test.CompareSongs(expectedCleaned, actualCleaned, order)
}

func timeToSeconds(t time.Time) float64 {
	return float64(t.UnixNano()) / float64(time.Second/time.Nanosecond)
}

func TestUpdate(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Importing songs from music dir")
	test.Must(tt, test.CopySongs(t.MusicDir, Song0s.Filename, Song1s.Filename))
	t.UpdateSongs()
	if err := test.CompareSongs([]db.Song{Song0s, Song1s},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after import: ", err)
	}

	log.Print("Importing another song")
	test.Must(tt, test.CopySongs(t.MusicDir, Song5s.Filename))
	t.UpdateSongs()
	if err := test.CompareSongs([]db.Song{Song0s, Song1s, Song5s},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after second import: ", err)
	}

	log.Print("Updating a song")
	test.Must(tt, test.DeleteSongs(t.MusicDir, Song0s.Filename))
	test.Must(tt, test.CopySongs(t.MusicDir, Song0sUpdated.Filename))
	t.UpdateSongs()
	if err := test.CompareSongs([]db.Song{Song0sUpdated, Song1s, Song5s},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after update: ", err)
	}

	gs5 := Song5s
	gs5.TrackGain = -6.3
	gs5.AlbumGain = -7.1
	gs5.PeakAmp = 0.9

	// If we pass a glob, only the matched file should be updated.
	// Change the song's gain info (by getting it from a dump) so we can
	// verify that it worked as expected.
	log.Print("Importing dumped gain with glob")
	glob := strings.TrimSuffix(gs5.Filename, ".mp3") + ".*"
	dumpPath, err := test.WriteSongsToJSONFile(tt.TempDir(), gs5)
	if err != nil {
		tt.Fatal("Failed writing JSON file: ", err)
	}
	t.UpdateSongs(test.ForceGlobFlag(glob), test.DumpedGainsFlag(dumpPath))
	if err := test.CompareSongs([]db.Song{Song0sUpdated, Song1s, gs5},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after glob import: ", err)
	}
}

func TestUserData(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Importing a song")
	test.Must(tt, test.CopySongs(t.MusicDir, Song0s.Filename))
	t.UpdateSongs()
	id := t.SongID(Song0s.SHA1)

	log.Print("Rating and tagging")
	s := Song0s
	s.Rating = 4
	s.Tags = []string{"electronic", "instrumental"}
	t.RateAndTag(id, s.Rating, s.Tags)
	if err := test.CompareSongs([]db.Song{s}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after rating and tagging: ", err)
	}

	log.Print("Reporting play")
	s.Plays = []db.Play{
		db.NewPlay(test.Date(2014, 9, 15, 2, 5, 18), "127.0.0.1"),
		db.NewPlay(test.Date(2014, 9, 15, 2, 8, 43), "127.0.0.1"),
		db.NewPlay(test.Date(2014, 9, 15, 2, 13, 4), "127.0.0.1"),
	}
	for i, p := range s.Plays {
		if i < len(s.Plays)-1 {
			t.ReportPlayed(id, p.StartTime) // RFC 3339
		} else {
			t.ReportPlayedUnix(id, p.StartTime) // seconds since epoch
		}
	}
	if err := test.CompareSongs([]db.Song{s}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after reporting play: ", err)
	}

	log.Print("Updating song and checking that user data is preserved")
	test.Must(tt, test.DeleteSongs(t.MusicDir, s.Filename))
	us := Song0sUpdated
	us.Rating = s.Rating
	us.Tags = s.Tags
	us.Plays = s.Plays
	test.Must(tt, test.CopySongs(t.MusicDir, us.Filename))
	t.UpdateSongs()
	if err := test.CompareSongs([]db.Song{us}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after updating song: ", err)
	}

	log.Print("Checking that duplicate plays are ignored")
	t.ReportPlayed(id, s.Plays[len(us.Plays)-1].StartTime)
	if err := test.CompareSongs([]db.Song{us}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after duplicate play: ", err)
	}

	log.Print("Checking that duplicate tags are ignored")
	us.Tags = []string{"electronic", "rock"}
	t.RateAndTag(id, -1, []string{"electronic", "electronic", "rock", "electronic"})
	if err := test.CompareSongs([]db.Song{us}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after duplicate tags: ", err)
	}

	log.Print("Clearing tags")
	us.Tags = nil
	t.RateAndTag(id, -1, []string{})
	if err := test.CompareSongs([]db.Song{us}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after clearing tags: ", err)
	}

	plays := us.Plays
	sort.Sort(db.PlayArray(plays))

	log.Print("Checking first-played queries")
	firstPlay := plays[0].StartTime
	query := "minFirstPlayed=" + firstPlay.Add(-10*time.Second).Format(time.RFC3339)
	if err := compareQueryResults([]db.Song{us}, t.QuerySongs(query), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", query, err)
	}
	query = "minFirstPlayed=" + firstPlay.Add(10*time.Second).Format(time.RFC3339)
	if err := compareQueryResults([]db.Song{}, t.QuerySongs(query), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", query, err)
	}

	log.Print("Checking last-played queries")
	lastPlay := plays[len(plays)-1].StartTime
	query = "maxLastPlayed=" + lastPlay.Add(-10*time.Second).Format(time.RFC3339)
	if err := compareQueryResults([]db.Song{}, t.QuerySongs(query), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", query, err)
	}
	query = "maxLastPlayed=" + lastPlay.Add(10*time.Second).Format(time.RFC3339)
	if err := compareQueryResults([]db.Song{us}, t.QuerySongs(query), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", query, err)
	}

	log.Print("Checking that play stats were updated")
	for i := 0; i < 3; i++ {
		query = "maxPlays=" + strconv.Itoa(i)
		if err := compareQueryResults([]db.Song{},
			t.QuerySongs(query), test.IgnoreOrder); err != nil {
			tt.Errorf("Bad results for %q: %v", query, err)
		}
	}
	query = "maxPlays=3"
	if err := compareQueryResults([]db.Song{us}, t.QuerySongs(query), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", query, err)
	}
}

func TestUpdateUseFilenames(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	mv := func(oldFn, newFn string) {
		if err := os.Rename(filepath.Join(t.MusicDir, oldFn), filepath.Join(t.MusicDir, newFn)); err != nil {
			tt.Error("Failed renaming song: ", err)
		}
	}

	const (
		oldFn  = "old.mp3"
		newFn  = "new.mp3"
		rating = 4
	)

	log.Print("Importing song from music dir")
	test.Must(tt, test.CopySongs(t.MusicDir, Song0s.Filename))
	mv(Song0s.Filename, oldFn)
	song := Song0s
	song.Filename = oldFn
	t.UpdateSongs()
	if err := test.CompareSongs([]db.Song{song}, t.DumpSongs(test.StripIDs),
		test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after import: ", err)
	}

	log.Print("Rating song")
	song.Rating = rating
	t.RateAndTag(t.SongID(song.SHA1), song.Rating, nil)

	// If we just rename the file, its hash will remain the same, so the existing
	// datastore entity should be reused.
	log.Print("Renaming song and checking that rating is preserved")
	mv(oldFn, newFn)
	song.Filename = newFn
	t.UpdateSongs()
	if err := test.CompareSongs([]db.Song{song}, t.DumpSongs(test.StripIDs),
		test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after renaming song: ", err)
	}

	// If we replace the file (changing its hash) but pass -use-filenames,
	// the datastore entity should be looked up by filename rather than by hash,
	// so we should still update the existing entity.
	test.Must(tt, test.CopySongs(t.MusicDir, Song5s.Filename))
	mv(Song5s.Filename, newFn)
	song = Song5s
	song.Filename = newFn
	song.Rating = rating
	t.UpdateSongs(test.UseFilenamesFlag)
	if err := test.CompareSongs([]db.Song{song}, t.DumpSongs(test.StripIDs),
		test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after replacing song: ", err)
	}
}

func TestUpdateCompare(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	test.Must(tt, test.CopySongs(t.MusicDir, Song0s.Filename, Song1s.Filename, Song5s.Filename))

	// Dump 0s (with changed user data) and 1s (with changed metadata) to a file.
	s0 := Song0s
	s0.Rating = 3
	s0.Tags = []string{"instrumental"}
	s1 := Song1s
	s1.Artist = s1.Artist + " (old)"
	dump, err := test.WriteSongsToJSONFile(tt.TempDir(), s0, s1)
	if err != nil {
		tt.Fatal("Failed writing songs: ", err)
	}

	// The update should send 1s (since the actual metadata differs from the dump)
	// and 5s (since it wasn't present in the dump).
	t.UpdateSongs(test.CompareDumpFileFlag(dump))
	if err := test.CompareSongs([]db.Song{Song1s, Song5s}, t.DumpSongs(test.StripIDs),
		test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after update: ", err)
	}
}

func TestQueries(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting some songs")
	t.PostSongs([]db.Song{LegacySong1, LegacySong2}, true, 0)
	t.PostSongs([]db.Song{Song0s, Song1s, Song5s}, false, 0)

	// Also post a song with Unicode characters that should be normalized.
	s10s := test.Song10s
	s10s.Artist = "µ-Ziq" // U+00B5 (MICRO SIGN)
	s10s.Title = "Mañana"
	s10s.Album = "Two²"
	t.PostSongs([]db.Song{s10s}, false, 0)

	const noIndex = 1 // flag indicating that fallback mode is allowed for query

	for _, tc := range []struct {
		params string
		flags  uint32
		want   []db.Song
	}{
		{"artist=AROVANE", 0, []db.Song{LegacySong1}},
		{"title=thaem+nue", 0, []db.Song{LegacySong1}},
		{"album=ATOL+scrap", 0, []db.Song{LegacySong1}},
		{"albumId=1e477f68-c407-4eae-ad01-518528cedc2c", 0, []db.Song{Song0s, Song1s}},
		{"album=Another+Album&albumId=a1d2405b-afe0-4e28-a935-b5b256f68131", 0, []db.Song{Song5s}},
		{"keywords=arovane+thaem+atol", 0, []db.Song{LegacySong1}},
		{"keywords=arovane+foo", 0, []db.Song{}},
		{"keywords=second+artist", 0, []db.Song{Song1s}}, // track artist
		{"keywords=remixer", 0, []db.Song{Song1s}},       // album artist
		{"minRating=5", 0, []db.Song{}},
		{"minRating=4", 0, []db.Song{LegacySong1}},
		{"minRating=3", 0, []db.Song{LegacySong2, LegacySong1}},
		{"minRating=1", 0, []db.Song{LegacySong2, LegacySong1}},
		{"unrated=1", 0, []db.Song{Song5s, Song0s, Song1s, s10s}},
		{"tags=instrumental", 0, []db.Song{LegacySong2, LegacySong1}},
		{"tags=electronic+instrumental", 0, []db.Song{LegacySong1}},
		{"tags=-electronic+instrumental", 0, []db.Song{LegacySong2}},
		{"tags=instrumental&minRating=4", 0, []db.Song{LegacySong1}},
		{"tags=instrumental&minRating=4&maxPlays=1", noIndex, []db.Song{}},
		{"tags=instrumental&minRating=4&maxPlays=2", noIndex, []db.Song{LegacySong1}},
		{"firstTrack=1", 0, []db.Song{LegacySong1, Song0s}},
		{"artist=" + url.QueryEscape("µ-Ziq"), 0, []db.Song{s10s}}, // U+00B5 (MICRO SIGN)
		{"artist=" + url.QueryEscape("μ-Ziq"), 0, []db.Song{s10s}}, // U+03BC (GREEK SMALL LETTER MU)
		{"title=manana", 0, []db.Song{s10s}},
		{"title=" + url.QueryEscape("mánanä"), 0, []db.Song{s10s}},
		{"album=two2", 0, []db.Song{s10s}},
		{"filename=" + url.QueryEscape(Song5s.Filename), 0, []db.Song{Song5s}},
		{"minDate=2000-01-01T00:00:00Z", 0, []db.Song{Song5s, Song1s}},
		{"maxDate=2000-01-01T00:00:00Z", 0, []db.Song{Song0s}},
		{"minDate=2000-01-01T00:00:00Z&maxDate=2010-01-01T00:00:00Z", 0, []db.Song{Song1s}},
		// Ensure that Datastore indexes exist to satisfy various queries (or if not, that the
		// server's fallback mode is still able to handle them).
		{"tags=-bogus&minRating=5&shuffle=1&orderByLastPlayed=1", 0, []db.Song{}},
		{"tags=instrumental&minRating=4&shuffle=1&orderByLastPlayed=1", 0, []db.Song{LegacySong1}},
		{"tags=instrumental+-bogus&minRating=4&shuffle=1&orderByLastPlayed=1", 0, []db.Song{LegacySong1}},
		{"minRating=4&shuffle=1&orderByLastPlayed=1", 0, []db.Song{LegacySong1}}, // old songs
		{"minRating=4&maxPlays=1&shuffle=1", 0, []db.Song{}},
		{"minRating=2&orderByLastPlayed=1", noIndex, []db.Song{LegacySong1, LegacySong2}},
		{"tags=instrumental&minRating=4&shuffle=1&maxLastPlayed=2022-04-06T14:41:14Z", 0, []db.Song{LegacySong1}},
		{"tags=instrumental&minRating=4&shuffle=1&maxPlays=1", noIndex, []db.Song{}},
		{"tags=instrumental&maxLastPlayed=2022-04-06T14:41:14Z", noIndex, []db.Song{LegacySong2, LegacySong1}},
		{"firstTrack=1&minFirstPlayed=2010-06-09T04:19:30Z", 0, []db.Song{LegacySong1}}, // new albums
		{"firstTrack=1&minFirstPlayed=2010-06-09T04:19:30Z&maxPlays=1", noIndex, []db.Song{}},
		{"keywords=arovane&minRating=4", 0, []db.Song{LegacySong1}},
		{"keywords=arovane&minRating=4&maxPlays=1", noIndex, []db.Song{}},
		{"keywords=arovane&firstTrack=1", 0, []db.Song{LegacySong1}},
		{"keywords=arovane&tags=instrumental&minRating=4&shuffle=1", 0, []db.Song{LegacySong1}},
		{"artist=arovane&firstTrack=1", 0, []db.Song{LegacySong1}},
		{"artist=arovane&minRating=4", 0, []db.Song{LegacySong1}},
		{"artist=arovane&minRating=4&maxPlays=1", noIndex, []db.Song{}},
		{"orderByLastPlayed=1&minFirstPlayed=2010-06-09T04:19:30Z&maxLastPlayed=2022-04-06T14:41:14Z",
			noIndex, []db.Song{LegacySong1, LegacySong2}},
		{"orderByLastPlayed=1&maxPlays=1&minFirstPlayed=2010-06-09T04:19:30Z&maxLastPlayed=2022-04-06T14:41:14Z",
			noIndex, []db.Song{LegacySong2}},
		{"orderByLastPlayed=1&maxPlays=1&minFirstPlayed=1276057160&maxLastPlayed=1649256074",
			noIndex, []db.Song{LegacySong2}}, // pass Unix timestamps
	} {
		suffixes := []string{""}
		if tc.flags&noIndex == 0 {
			// If we should have an index, also verify that the query works both without
			// falling back and (for good measure) when only using the fallback path.
			suffixes = append(suffixes, "&fallback=never", "&fallback=force")
		}
		for _, suf := range suffixes {
			query := tc.params + suf
			log.Printf("Doing query %q", query)
			if err := compareQueryResults(tc.want, t.QuerySongs(query), test.CompareOrder); err != nil {
				tt.Errorf("%v: %v", query, err)
			}
		}
	}
}

func TestCaching(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting and querying a song")
	const cacheParam = "cacheOnly=1"
	s1 := LegacySong1
	t.PostSongs([]db.Song{s1}, true, 0)
	if err := compareQueryResults([]db.Song{s1}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results when querying from cache: ", err)
	}

	// After rating the song, the query results should still be served from the cache.
	log.Print("Rating and re-querying")
	id1 := t.SongID(s1.SHA1)
	s1.Rating = 5
	t.RateAndTag(id1, s1.Rating, nil)
	if err := compareQueryResults([]db.Song{s1}, t.QuerySongs(cacheParam), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after rating: ", err)
	}

	// After updating metadata, the updated song should be returned (indicating
	// that the cached results were dropped).
	log.Print("Updating and re-querying")
	s1.Artist = "The Artist Formerly Known As " + s1.Artist
	t.PostSongs([]db.Song{s1}, false, 0)
	if err := compareQueryResults([]db.Song{s1}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after updating: ", err)
	}

	log.Print("Checking that time-based queries aren't cached")
	timeParam := "maxLastPlayed=" + s1.Plays[1].StartTime.Add(time.Second).Format(time.RFC3339)
	if err := compareQueryResults([]db.Song{s1}, t.QuerySongs(timeParam), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q without cache: %v", timeParam, err)
	}
	if err := compareQueryResults([]db.Song{}, t.QuerySongs(timeParam, cacheParam), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q from cache: %v", timeParam, err)
	}

	log.Print("Checking that play-count-based queries aren't cached")
	playParam := "maxPlays=10"
	if err := compareQueryResults([]db.Song{s1}, t.QuerySongs(playParam), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", playParam, err)
	}
	if err := compareQueryResults([]db.Song{}, t.QuerySongs(playParam, cacheParam), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q from cache: %v", playParam, err)
	}

	log.Print("Checking that datastore cache is used after memcache miss")
	if err := compareQueryResults([]db.Song{s1}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results before flushing memcache: ", err)
	}
	t.FlushCache(test.FlushMemcache)
	if err := compareQueryResults([]db.Song{s1}, t.QuerySongs(cacheParam), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after flushing memcache: ", err)
	}

	log.Print("Checking that posting a song drops cached queries")
	s2 := LegacySong2
	t.PostSongs([]db.Song{s2}, true, 0)
	if err := compareQueryResults([]db.Song{s1, s2}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after posting song: ", err)
	}

	log.Print("Checking that deleting a song drops cached queries")
	if err := compareQueryResults([]db.Song{s2},
		t.QuerySongs("album="+url.QueryEscape(s2.Album)), test.IgnoreOrder); err != nil {
		tt.Error("Bad results before deleting song: ", err)
	}
	id2 := t.SongID(s2.SHA1)
	t.DeleteSong(id2)
	if err := compareQueryResults([]db.Song{},
		t.QuerySongs("album="+url.QueryEscape(s2.Album)), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after deleting song: ", err)
	}
}

func TestAndroid(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting songs")
	now := t.GetNowFromServer()
	t.PostSongs([]db.Song{LegacySong1, LegacySong2}, true, 0)
	if err := compareQueryResults([]db.Song{LegacySong1, LegacySong2},
		t.GetSongsForAndroid(time.Time{}, test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad results with empty time: ", err)
	}
	if err := compareQueryResults([]db.Song{LegacySong1, LegacySong2},
		t.GetSongsForAndroid(now, test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad results with old time: ", err)
	}
	if err := compareQueryResults([]db.Song{},
		t.GetSongsForAndroid(t.GetNowFromServer(), test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad results with now: ", err)
	}

	log.Print("Rating a song")
	id := t.SongID(LegacySong1.SHA1)
	updatedLegacySong1 := LegacySong1
	updatedLegacySong1.Rating = 5
	now = t.GetNowFromServer()
	t.RateAndTag(id, updatedLegacySong1.Rating, nil)
	if err := compareQueryResults([]db.Song{updatedLegacySong1},
		t.GetSongsForAndroid(now, test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after rating and tagging: ", err)
	}

	// Reporting a play shouldn't update the song's last-modified time.
	log.Print("Reporting play")
	p := db.NewPlay(test.Date(2014, 9, 15, 2, 5, 18), "127.0.0.1")
	updatedLegacySong1.Plays = append(updatedLegacySong1.Plays, p)
	now = t.GetNowFromServer()
	t.ReportPlayed(id, p.StartTime)
	if err := compareQueryResults([]db.Song{},
		t.GetSongsForAndroid(now, test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after reporting play: ", err)
	}
}

func TestTags(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Getting hopefully-empty tag list")
	if tags := t.GetTags(false); len(tags) > 0 {
		tt.Errorf("got unexpected tags %q", tags)
	}

	log.Print("Posting song and getting tags")
	t.PostSongs([]db.Song{LegacySong1}, true, 0)
	if tags := t.GetTags(false); tags != "electronic,instrumental" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("Posting another song and getting tags")
	t.PostSongs([]db.Song{LegacySong2}, true, 0)
	if tags := t.GetTags(false); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("Checking that tags are cached")
	if tags := t.GetTags(true); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("Checking that datastore cache is used after memcache miss")
	t.FlushCache(test.FlushMemcache)
	if tags := t.GetTags(true); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("Adding tags and checking that they're returned")
	id := t.SongID(LegacySong1.SHA1)
	t.RateAndTag(id, -1, []string{"electronic", "instrumental", "drums", "idm"})
	if tags := t.GetTags(false); tags != "drums,electronic,idm,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}
}

func TestCovers(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	createCover := func(fn string) {
		f, err := os.Create(filepath.Join(t.CoverDir, fn))
		if err != nil {
			tt.Fatal("Failed creating cover: ", err)
		}
		if err := f.Close(); err != nil {
			tt.Fatal("Failed closing cover: ", err)
		}
	}

	log.Print("Writing cover and importing songs")
	test.Must(tt, test.CopySongs(t.MusicDir, Song0s.Filename, Song5s.Filename))
	s5 := Song5s
	s5.CoverFilename = fmt.Sprintf("%s.jpg", s5.AlbumID)
	createCover(s5.CoverFilename)
	t.UpdateSongs()
	if err := compareQueryResults([]db.Song{Song0s, s5}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after importing songs: ", err)
	}

	log.Print("Writing another cover and updating")
	test.Must(tt, test.DeleteSongs(t.MusicDir, Song0s.Filename))
	test.Must(tt, test.CopySongs(t.MusicDir, Song0sUpdated.Filename))
	s0 := Song0sUpdated
	s0.CoverFilename = fmt.Sprintf("%s.jpg", s0.AlbumID)
	createCover(s0.CoverFilename)
	t.UpdateSongs()
	if err := compareQueryResults([]db.Song{s0, s5}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after updating songs: ", err)
	}

	log.Print("Writing cover named after recording ID")
	test.Must(tt, test.CopySongs(t.MusicDir, Song1s.Filename))
	s1 := Song1s
	s1.CoverFilename = fmt.Sprintf("%s.jpg", s1.RecordingID)
	createCover(s1.CoverFilename)
	test.Must(tt, test.DeleteSongs(t.CoverDir, s0.CoverFilename))
	t.UpdateSongs()
	if err := compareQueryResults([]db.Song{s0, s1, s5}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after using recording ID: ", err)
	}

	log.Print("Checking that covers are dumped")
	if err := test.CompareSongs([]db.Song{s0, s1, s5},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs when dumping covers: ", err)
	}
	if err := test.CompareSongs([]db.Song{s0, s1, s5},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs when dumping covers: ", err)
	}
}

func TestJSONImport(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Importing songs from JSON")
	t.ImportSongsFromJSONFile([]db.Song{LegacySong1, LegacySong2})
	if err := test.CompareSongs([]db.Song{LegacySong1, LegacySong2},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after importing from JSON: ", err)
	}

	log.Print("Updating song from JSON")
	us := LegacySong1
	us.Artist += " bogus"
	us.Title += " bogus"
	us.Album += " bogus"
	us.Track += 1
	us.Disc += 1
	us.Length *= 2
	us.Rating = 2
	us.Plays = us.Plays[0:1]
	us.Tags = []string{"bogus"}
	t.ImportSongsFromJSONFile([]db.Song{us, LegacySong2})
	if err := test.CompareSongs([]db.Song{us, LegacySong2},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after updating from JSON: ", err)
	}

	log.Print("Reporting play")
	id := t.SongID(us.SHA1)
	st := test.Date(2014, 9, 15, 2, 5, 18)
	t.ReportPlayed(id, st)
	us.Plays = append(us.Plays, db.NewPlay(st, "127.0.0.1"))
	if err := test.CompareSongs([]db.Song{us, LegacySong2},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after reporting play: ", err)
	}

	log.Print("Updating song from JSON but preserving user data")
	t.ImportSongsFromJSONFile([]db.Song{LegacySong1, LegacySong2}, test.KeepUserDataFlag)
	us2 := LegacySong1
	us2.Rating = us.Rating
	us2.Tags = us.Tags
	us2.Plays = us.Plays
	if err := test.CompareSongs([]db.Song{us2, LegacySong2},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after updating from JSON with preserved user data: ", err)
	}
}

func TestUpdateList(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	test.Must(tt, test.CopySongs(t.MusicDir, Song0s.Filename, Song1s.Filename, Song5s.Filename))
	tempDir := tt.TempDir()
	listPath, err := test.WriteSongPathsFile(tempDir, Song0s.Filename, Song5s.Filename)
	if err != nil {
		tt.Fatal("Failed writing song paths: ", err)
	}

	gs0 := Song0s
	gs0.TrackGain = -8.4
	gs0.AlbumGain = -7.6
	gs0.PeakAmp = 1.2

	gs5 := Song5s
	gs5.TrackGain = -6.3
	gs5.AlbumGain = -7.1
	gs5.PeakAmp = 0.9

	log.Print("Updating songs from list")
	t.UpdateSongsFromList(listPath)
	if err := test.CompareSongs([]db.Song{Song0s, Song5s},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after updating from list: ", err)
	}

	// When a dump file is passed, its gain info should be sent to the server.
	log.Print("Updating songs from list with dumped gains")
	dumpPath, err := test.WriteSongsToJSONFile(tempDir, gs0, gs5)
	if err != nil {
		tt.Fatal("Failed writing JSON file: ", err)
	}
	t.UpdateSongsFromList(listPath, test.DumpedGainsFlag(dumpPath))
	if err := test.CompareSongs([]db.Song{gs0, gs5},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after updating from list with dumped gains: ", err)
	}
}

func TestSorting(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	songs := make([]db.Song, 0)
	for _, s := range []struct {
		Artist  string
		Album   string
		AlbumID string
		Disc    int
		Track   int
	}{
		// Sorting should be [Album, AlbumID, Disc, Track].
		{"b", "album1", "23", 1, 1},
		{"b", "album1", "23", 1, 2},
		{"b", "album1", "23", 2, 1},
		{"b", "album1", "23", 2, 2},
		{"b", "album1", "23", 2, 3},
		{"a", "album1", "56", 1, 1},
		{"a", "album1", "56", 1, 2},
		{"a", "album1", "56", 1, 3},
		{"b", "album2", "10", 1, 1},
	} {
		id := fmt.Sprintf("%s-%s-%d-%d", s.Artist, s.Album, s.Disc, s.Track)
		songs = append(songs, db.Song{
			SHA1:     id,
			Filename: id + ".mp3",
			Artist:   s.Artist,
			Title:    fmt.Sprintf("Track %d", s.Track),
			Album:    s.Album,
			AlbumID:  s.AlbumID,
			Track:    s.Track,
			Disc:     s.Disc,
		})
	}

	log.Print("Importing songs and checking sort order")
	t.ImportSongsFromJSONFile(songs)
	if err := compareQueryResults(songs, t.QuerySongs(), test.CompareOrder); err != nil {
		tt.Error("Bad results: ", err)
	}
}

func TestDeleteSong(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting songs and deleting first song")
	postTime := t.GetNowFromServer()
	t.PostSongs([]db.Song{LegacySong1, LegacySong2}, true, 0)
	id1 := t.SongID(LegacySong1.SHA1)
	t.DeleteSong(id1)

	log.Print("Checking non-deleted song")
	if err := compareQueryResults([]db.Song{LegacySong2}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results for non-deleted song: ", err)
	}
	if err := compareQueryResults([]db.Song{LegacySong2},
		t.GetSongsForAndroid(time.Time{}, test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad Android results for non-deleted song with empty time: ", err)
	}
	if err := compareQueryResults([]db.Song{LegacySong2},
		t.GetSongsForAndroid(postTime, test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad Android results for non-deleted song with later time: ", err)
	}
	if err := test.CompareSongs([]db.Song{LegacySong2},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after deletion: ", err)
	}

	log.Print("Checking that deleted song is in Android query")
	deletedSongs := t.GetSongsForAndroid(postTime, test.GetDeletedSongs)
	if err := compareQueryResults([]db.Song{LegacySong1}, deletedSongs, test.IgnoreOrder); err != nil {
		tt.Error("Bad Android results for deleted song: ", err)
	}
	if deletedSongs[0].SongID != id1 {
		tt.Errorf("Deleted song's ID (%v) didn't match original id (%v)",
			deletedSongs[0].SongID, id1)
	}

	log.Print("Deleting second song")
	laterTime := t.GetNowFromServer()
	id2 := t.SongID(LegacySong2.SHA1)
	t.DeleteSong(id2)

	log.Print("Checking no non-deleted songs")
	if err := compareQueryResults([]db.Song{}, t.QuerySongs(), test.IgnoreOrder); err != nil {
		tt.Error("Bad results for empty non-deleted songs: ", err)
	}
	if err := compareQueryResults([]db.Song{},
		t.GetSongsForAndroid(time.Time{}, test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad Android results for empty non-deleted songs: ", err)
	}
	if err := test.CompareSongs([]db.Song{}, t.DumpSongs(test.StripIDs),
		test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after deleting second song: ", err)
	}

	log.Print("Checking that both deleted songs are in Android query")
	deletedSongs = t.GetSongsForAndroid(laterTime, test.GetDeletedSongs)
	if err := compareQueryResults([]db.Song{LegacySong2}, deletedSongs, test.IgnoreOrder); err != nil {
		tt.Error("Bad deleted songs for Android: ", err)
	}
	if deletedSongs[0].SongID != id2 {
		tt.Errorf("Deleted song's ID (%v) didn't match original id (%v)",
			deletedSongs[0].SongID, id2)
	}
}

func TestMergeSongs(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting songs")
	s1 := Song0s
	s1.Rating = 4
	s1.Tags = []string{"guitar", "instrumental"}
	s1.Plays = []db.Play{
		db.NewPlay(test.Date(2014, 9, 15, 2, 5, 18), "127.0.0.1"),
	}
	s2 := Song1s
	s2.Rating = 2
	s2.Tags = []string{"drums", "guitar", "rock"}
	s2.Plays = []db.Play{
		db.NewPlay(test.Date(2014, 9, 15, 2, 8, 43), "127.0.0.1"),
		db.NewPlay(test.Date(2014, 9, 15, 2, 13, 4), "127.0.0.1"),
	}
	t.PostSongs([]db.Song{s1, s2}, true, 0)

	log.Print("Merging songs")
	t.MergeSongs(t.SongID(s1.SHA1), t.SongID(s2.SHA1))

	log.Print("Checking that songs were merged")
	s2.Rating = s1.Rating
	s2.Tags = []string{"drums", "guitar", "instrumental", "rock"}
	s2.Plays = append(s2.Plays, s1.Plays...)
	sort.Sort(db.PlayArray(s2.Plays))
	if err := test.CompareSongs([]db.Song{s1, s2}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after merging: ", err)
	}

	log.Print("Merging songs again")
	t.MergeSongs(t.SongID(s1.SHA1), t.SongID(s2.SHA1), test.DeleteAfterMergeFlag)

	log.Print("Checking that first song was deleted")
	if err := test.CompareSongs([]db.Song{s2}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after merging again: ", err)
	}
}

func TestReindexSongs(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting song")
	s := Song0s
	s.Rating = 4
	s.Tags = []string{"guitar", "instrumental"}
	s.Plays = []db.Play{
		db.NewPlay(test.Date(2014, 9, 15, 2, 5, 18), "127.0.0.1"),
	}
	t.PostSongs([]db.Song{s}, true, 0)

	log.Print("Reindexing")
	t.ReindexSongs()

	// This doesn't actually check that we reindex, but it at least verifies that the server isn't
	// dropping user data.
	log.Print("Querying after reindex")
	if err := compareQueryResults([]db.Song{s}, t.QuerySongs("minRating=1"), test.IgnoreOrder); err != nil {
		tt.Error("Bad results for query: ", err)
	}
}

func TestStats(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting songs")
	s1 := Song1s
	s1.Rating = 4
	s1.Tags = []string{"guitar", "instrumental"}
	s1.Plays = []db.Play{db.NewPlay(test.Date(2014, 9, 15, 2, 5, 18), "127.0.0.1")}
	s2 := Song5s
	s2.Rating = 5
	s2.Tags = []string{"guitar", "vocals"}
	s2.Plays = []db.Play{
		db.NewPlay(test.Date(2013, 9, 15, 2, 5, 18), "127.0.0.1"),
		db.NewPlay(test.Date(2014, 9, 15, 2, 5, 18), "127.0.0.1"),
	}
	s3 := Song10s
	t.PostSongs([]db.Song{s1, s2, s3}, true, 0)

	log.Print("Updating stats")
	t.UpdateStats()

	log.Print("Checking stats")
	got := t.GetStats()
	if got.UpdateTime.IsZero() {
		tt.Error("Stats update time is zero")
	}
	want := db.Stats{
		Songs:       3,
		Albums:      3,
		TotalSec:    s1.Length + s2.Length + s3.Length,
		Ratings:     map[int]int{0: 1, 4: 1, 5: 1},
		SongDecades: map[int]int{0: 1, 2000: 1, 2010: 1},
		Tags:        map[string]int{"guitar": 2, "instrumental": 1, "vocals": 1},
		Years: map[int]db.PlayStats{
			2013: {Plays: 1, TotalSec: s2.Length, FirstPlays: 1},
			2014: {Plays: 2, TotalSec: s1.Length + s2.Length, FirstPlays: 1, LastPlays: 2},
		},
		UpdateTime: got.UpdateTime, // checked for non-zero earlier
	}
	if !reflect.DeepEqual(got, want) {
		tt.Errorf("Got %+v, want %+v", got, want)
	}
}

func TestUpdateError(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	// Write a file with a header with a bogus ID3v2 version number (which should cause an error in
	// taglib-go) and no trailing ID3v1 tag (so we can't fall back to it).
	f, err := os.Create(filepath.Join(t.MusicDir, "song.mp3"))
	if err != nil {
		tt.Fatal("Failed creating file: ", err)
	}
	if _, err := io.WriteString(f, "ID301"+strings.Repeat("\x00", 1024)); err != nil {
		tt.Fatal("Failed writing file: ", err)
	}
	if err := f.Close(); err != nil {
		tt.Fatal("Failed closing file: ", err)
	}
	want := filepath.Base(f.Name()) + ": taglib: format not supported"

	log.Print("Importing malformed song")
	if _, stderr, err := t.UpdateSongsRaw(); err == nil {
		tt.Error("Update unexpectedly succeeded with bad file\nstderr:\n" + stderr)
	} else if !strings.Contains(stderr, want) {
		tt.Errorf("Output doesn't include %q\nstderr:\n%s", want, stderr)
	}

	// We shouldn't have written the last update time, so a second attempt should also fail.
	log.Print("Importing malformed song again")
	if _, stderr, err := t.UpdateSongsRaw(); err == nil {
		tt.Error("Repeated update attempt unexpectedly succeeded\nstderr:\n" + stderr)
	}
}

func TestGuestUser(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting song and updating stats")
	t.PostSongs([]db.Song{Song0s}, false, 0)
	t.UpdateStats()
	songID := t.SongID(Song0s.SHA1)

	send := func(method, path, user, pass string) (int, []byte) {
		req := t.NewRequest(method, path, nil)
		req.SetBasicAuth(user, pass)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			tt.Fatalf("%v request for %v from %q failed: %v", method, path, user, err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			tt.Fatalf("Failed reading body from %v request for %v from %q failed: %v", method, path, user, err)
		}
		return resp.StatusCode, body
	}

	// Normal (or admin) users should be able to call /rate_and_tag, but guest users shouldn't.
	log.Print("Checking /rate_and_tag access")
	ratePath := "rate_and_tag?songId=" + songID + "&rating=5&tags=drums+guitar+rock"
	if code, _ := send("POST", ratePath, test.Username, test.Password); code != http.StatusOK {
		tt.Fatalf("Normal request for /%v returned %v; want %v", ratePath, code, http.StatusOK)
	}
	if code, _ := send("POST", ratePath, guestUsername, guestPassword); code != http.StatusForbidden {
		tt.Fatalf("Guest request for /%v returned %v; want %v", ratePath, code, http.StatusForbidden)
	}

	// Ditto for /played.
	log.Print("Checking /played access")
	playedPath := "played?songId=" + songID + "&startTime=2006-01-02T15:04:05Z"
	if code, _ := send("POST", playedPath, test.Username, test.Password); code != http.StatusOK {
		tt.Fatalf("Normal request for /%v returned %v; want %v", playedPath, code, http.StatusOK)
	}
	if code, _ := send("POST", playedPath, guestUsername, guestPassword); code != http.StatusForbidden {
		tt.Fatalf("Guest request for /%v returned %v; want %v", playedPath, code, http.StatusForbidden)
	}

	// The /user endpoint should return information about the requesting user.
	log.Print("Checking /user")
	if code, got := send("GET", "user", test.Username, test.Password); code != http.StatusOK {
		tt.Fatalf("Normal request for /user returned %v; want %v", code, http.StatusOK)
	} else if want, err := json.Marshal(config.User{Username: test.Username, Admin: true}); err != nil {
		tt.Fatal("Failed marshaling:", err)
	} else if !bytes.Equal(got, want) {
		tt.Fatalf("Normal request for /user gave %q; want %q", got, want)
	}
	if code, got := send("GET", "user", guestUsername, guestPassword); code != http.StatusOK {
		tt.Fatalf("Guest request for /user returned %v; want %v", code, http.StatusOK)
	} else if want, err := json.Marshal(
		config.User{
			Username:     guestUsername,
			Guest:        true,
			Presets:      guestPresets,
			ExcludedTags: guestExcludedTags,
		}); err != nil {
		tt.Fatal("Failed marshaling:", err)
	} else if !bytes.Equal(got, want) {
		tt.Fatalf("Guest request for /user gave %q; want %q", got, want)
	}

	// Guest users should be able to fetch /stats, but not update them via /stats?update=1.
	log.Print("Checking /stats access")
	if code, _ := send("GET", "stats", guestUsername, guestPassword); code != http.StatusOK {
		tt.Fatalf("Guest request for /stats returned %v; want %v", code, http.StatusOK)
	}
	if code, _ := send("GET", "stats?update=1", guestUsername, guestPassword); code != http.StatusForbidden {
		tt.Fatalf("Guest request for /stats?update=1 returned %v; want %v", code, http.StatusForbidden)
	}

	log.Print("Checking that tags are excluded from /tags")
	var tags []string
	if code, body := send("GET", "tags", guestUsername, guestPassword); code != http.StatusOK {
		tt.Fatalf("Guest request for /tags returned %v; want %v", code, http.StatusOK)
	} else if err := json.Unmarshal(body, &tags); err != nil {
		tt.Fatal("Failed unmarshaling:", err)
	} else if want := []string{"drums", "guitar"}; !reflect.DeepEqual(tags, want) {
		tt.Fatalf("Guest request for /tags returned %q; want %q", tags, want)
	}

	// Normal (or admin) users should be able to go above the guest rate limit for /song.
	log.Print("Checking /song rate-limiting")
	songPath := "song?filename=" + url.QueryEscape(Song0s.Filename)
	for i := 0; i <= maxGuestRequests; i++ {
		if code, _ := send("GET", songPath, test.Username, test.Password); code != http.StatusOK {
			tt.Fatalf("Normal request %v for /%v returned %v; want %v", i, songPath, code, http.StatusOK)
		}
	}
	// The guest user should get an error when they exceed the limit.
	for i := 0; i <= maxGuestRequests; i++ {
		want := http.StatusOK
		if i == maxGuestRequests {
			want = http.StatusTooManyRequests
		}
		if code, _ := send("GET", songPath, guestUsername, guestPassword); code != want {
			tt.Fatalf("Guest request %v for /%v returned %v; want %v", i, songPath, code, want)
		}
	}
}
