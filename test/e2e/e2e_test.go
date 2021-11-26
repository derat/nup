// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package e2e contains end-to-end tests between the server and command-line tools.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/derat/nup/server/db"
	"github.com/derat/nup/server/query"
	"github.com/derat/nup/test"
)

const (
	server      = "http://localhost:8080/"
	songBucket  = "song-bucket"
	coverBucket = "cover-bucket"
)

var binDir string = filepath.Join(os.Getenv("GOPATH"), "bin")

var (
	// Pull some stuff into our namespace for convenience.
	Song0s        = test.Song0s
	Song0sUpdated = test.Song0sUpdated
	Song1s        = test.Song1s
	Song5s        = test.Song5s
	LegacySong1   = test.LegacySong1
	LegacySong2   = test.LegacySong2
)

func setUpTest() *tester {
	t := newTester(server, binDir)
	if err := t.pingServer(); err != nil {
		log.Printf("Unable to connect to server: %v\n", err)
		log.Printf("Run dev_appserver.py, maybe?")
		os.Exit(1)
	}
	log.Printf("clearing all data on %v", server)
	t.doPost("clear", nil)
	t.doPost("flush_cache", nil)

	// This corresponds to the config struct from server/config.go.
	b, err := json.Marshal(struct {
		SongBucket  string `json:"songBucket"`
		CoverBucket string `json:"coverBucket"`
	}{
		SongBucket:  songBucket,
		CoverBucket: coverBucket,
	})
	if err != nil {
		panic(err)
	}
	t.doPost("config", bytes.NewBuffer(b))

	return t
}

func cleanUpTest(t *tester) {
	t.doPost("config", nil)
	t.close()
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
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("importing songs from music dir")
	test.CopySongs(t.musicDir, Song0s.Filename, Song1s.Filename)
	t.updateSongs()
	if err := test.CompareSongs([]db.Song{Song0s, Song1s},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("importing another song")
	test.CopySongs(t.musicDir, Song5s.Filename)
	t.updateSongs()
	if err := test.CompareSongs([]db.Song{Song0s, Song1s, Song5s},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("updating a song")
	test.DeleteSongs(t.musicDir, Song0s.Filename)
	test.CopySongs(t.musicDir, Song0sUpdated.Filename)
	t.updateSongs()
	if err := test.CompareSongs([]db.Song{Song0sUpdated, Song1s, Song5s},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	gs5 := Song5s
	gs5.TrackGain = -6.3
	gs5.AlbumGain = -7.1
	gs5.PeakAmp = 0.9

	// If we pass a glob, only the matched file should be updated.
	// Change the song's gain info (by getting it from a dump) so we can
	// verify that it worked as expected.
	log.Print("importing dumped gain with glob")
	glob := strings.TrimSuffix(gs5.Filename, ".mp3") + ".*"
	dumpPath := test.WriteSongsToJSONFile(t.tempDir, []db.Song{gs5})
	t.updateSongs(forceGlobFlag(glob), dumpedGainsFlag(dumpPath))
	if err := test.CompareSongs([]db.Song{Song0sUpdated, Song1s, gs5},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestUserData(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("importing a song")
	test.CopySongs(t.musicDir, Song0s.Filename)
	t.updateSongs()
	id := t.SongID(Song0s.SHA1)

	log.Print("rating and tagging")
	s := Song0s
	s.Rating = 0.75
	s.Tags = []string{"electronic", "instrumental"}
	t.doPost("rate_and_tag?songId="+id+"&rating=0.75&tags=electronic+instrumental", nil)
	if err := test.CompareSongs([]db.Song{s}, t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	log.Print("reporting playback")
	s.Plays = []db.Play{
		db.NewPlay(time.Unix(1410746718, 0), "127.0.0.1"),
		db.NewPlay(time.Unix(1410746923, 0), "127.0.0.1"),
		db.NewPlay(time.Unix(1410747184, 0), "127.0.0.1"),
	}
	for _, p := range s.Plays {
		t.doPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, p.StartTime.Unix()), nil)
	}
	if err := test.CompareSongs([]db.Song{s}, t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	log.Print("updating song and checking that user data is preserved")
	test.DeleteSongs(t.musicDir, s.Filename)
	us := Song0sUpdated
	us.Rating = s.Rating
	us.Tags = s.Tags
	us.Plays = s.Plays
	test.CopySongs(t.musicDir, us.Filename)
	t.updateSongs()
	if err := test.CompareSongs([]db.Song{us}, t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that duplicate plays are ignored")
	t.doPost(fmt.Sprintf("played?songId=%v&startTime=%v",
		id, s.Plays[len(us.Plays)-1].StartTime.Unix()), nil)
	if err := test.CompareSongs([]db.Song{us}, t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	log.Print("checking that duplicate tags are ignored")
	us.Tags = []string{"electronic", "rock"}
	t.doPost("rate_and_tag?songId="+id+"&tags=electronic+electronic+rock+electronic", nil)
	if err := test.CompareSongs([]db.Song{us}, t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	log.Print("clearing tags")
	us.Tags = nil
	t.doPost("rate_and_tag?songId="+id+"&tags=", nil)
	if err := test.CompareSongs([]db.Song{us}, t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	plays := us.Plays
	sort.Sort(db.PlayArray(plays))

	log.Print("checking first-played queries")
	firstPlaySec := timeToSeconds(plays[0].StartTime)
	if err := compareQueryResults([]db.Song{us},
		t.querySongs(fmt.Sprintf("minFirstPlayed=%.1f", firstPlaySec-10)),
		test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{},
		t.querySongs(fmt.Sprintf("minFirstPlayed=%.1f", firstPlaySec+10)),
		test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking last-played queries")
	lastPlaySec := timeToSeconds(plays[len(plays)-1].StartTime)
	if err := compareQueryResults([]db.Song{},
		t.querySongs(fmt.Sprintf("maxLastPlayed=%.1f", lastPlaySec-10)),
		test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{us},
		t.querySongs(fmt.Sprintf("maxLastPlayed=%.1f", lastPlaySec+10)),
		test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that play stats were updated")
	for i := 0; i < 3; i++ {
		if err := compareQueryResults([]db.Song{},
			t.querySongs("maxPlays="+strconv.Itoa(i)), test.IgnoreOrder); err != nil {
			tt.Error(err)
		}
	}
	if err := compareQueryResults([]db.Song{us},
		t.querySongs("maxPlays=3"), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestQueries(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("posting some songs")
	t.postSongs([]db.Song{LegacySong1, LegacySong2}, true, 0)
	t.postSongs([]db.Song{Song0s, Song1s, Song5s}, false, 0)

	log.Print("doing a bunch of queries")
	for _, q := range []struct {
		params []string
		exp    []db.Song
	}{
		{[]string{"artist=AROVANE"}, []db.Song{LegacySong1}},
		{[]string{"title=thaem+nue"}, []db.Song{LegacySong1}},
		{[]string{"album=ATOL+scrap"}, []db.Song{LegacySong1}},
		{[]string{"albumId=1e477f68-c407-4eae-ad01-518528cedc2c"}, []db.Song{Song0s, Song1s}},
		{[]string{"album=Another+Album", "albumId=a1d2405b-afe0-4e28-a935-b5b256f68131"}, []db.Song{Song5s}},
		{[]string{"keywords=arovane+thaem+atol"}, []db.Song{LegacySong1}},
		{[]string{"keywords=arovane+foo"}, []db.Song{}},
		{[]string{"minRating=1.0"}, []db.Song{}},
		{[]string{"minRating=0.75"}, []db.Song{LegacySong1}},
		{[]string{"minRating=0.5"}, []db.Song{LegacySong2, LegacySong1}},
		{[]string{"minRating=0.0"}, []db.Song{LegacySong2, LegacySong1}},
		{[]string{"unrated=1"}, []db.Song{Song5s, Song0s, Song1s}},
		{[]string{"tags=instrumental"}, []db.Song{LegacySong2, LegacySong1}},
		{[]string{"tags=electronic+instrumental"}, []db.Song{LegacySong1}},
		{[]string{"tags=-electronic+instrumental"}, []db.Song{LegacySong2}},
		{[]string{"tags=instrumental", "minRating=0.75"}, []db.Song{LegacySong1}},
		{[]string{"firstTrack=1"}, []db.Song{LegacySong1, Song0s}},
	} {
		if err := compareQueryResults(q.exp, t.querySongs(q.params...), test.CompareOrder); err != nil {
			tt.Errorf("%v: %v", q.params, err)
		}
	}
}

func TestCaching(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("posting and querying a song")
	const cacheParam = "cacheOnly=1"
	s1 := LegacySong1
	t.postSongs([]db.Song{s1}, true, 0)
	if err := compareQueryResults([]db.Song{s1}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	// After rating the song, the query results should still be served from the cache.
	log.Print("rating and re-querying")
	id1 := t.SongID(s1.SHA1)
	s1.Rating = 1.0
	t.doPost("rate_and_tag?songId="+id1+"&rating=1.0", nil)
	if err := compareQueryResults([]db.Song{s1}, t.querySongs(cacheParam), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	// After updating metadata, the updated song should be returned (indicating
	// that the cached results were dropped).
	log.Print("updating and re-querying")
	s1.Artist = "The Artist Formerly Known As " + s1.Artist
	t.postSongs([]db.Song{s1}, false, 0)
	if err := compareQueryResults([]db.Song{s1}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that time-based queries aren't cached")
	timeParam := fmt.Sprintf("maxLastPlayed=%d", s1.Plays[1].StartTime.Unix()+1)
	if err := compareQueryResults([]db.Song{s1}, t.querySongs(timeParam), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{}, t.querySongs(timeParam, cacheParam), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that play-count-based queries aren't cached")
	playParam := "maxPlays=10"
	if err := compareQueryResults([]db.Song{s1}, t.querySongs(playParam), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{}, t.querySongs(playParam, cacheParam), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that datastore cache is used after memcache miss")
	if err := compareQueryResults([]db.Song{s1}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	t.doPost("flush_cache?onlyMemcache=1", nil)
	if err := compareQueryResults([]db.Song{s1}, t.querySongs(cacheParam), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that posting a song drops cached queries")
	s2 := LegacySong2
	t.postSongs([]db.Song{s2}, true, 0)
	if err := compareQueryResults([]db.Song{s1, s2}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that deleting a song drops cached queries")
	if err := compareQueryResults([]db.Song{s2},
		t.querySongs("album="+url.QueryEscape(s2.Album)), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	id2 := t.SongID(s2.SHA1)
	t.deleteSong(id2)
	if err := compareQueryResults([]db.Song{},
		t.querySongs("album="+url.QueryEscape(s2.Album)), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestAndroid(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("posting songs")
	now := t.getNowFromServer()
	t.postSongs([]db.Song{LegacySong1, LegacySong2}, true, 0)
	if err := compareQueryResults([]db.Song{LegacySong1, LegacySong2},
		t.getSongsForAndroid(time.Time{}, getRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{LegacySong1, LegacySong2},
		t.getSongsForAndroid(now, getRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{},
		t.getSongsForAndroid(t.getNowFromServer(), getRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("rating a song")
	id := t.SongID(LegacySong1.SHA1)
	updatedLegacySong1 := LegacySong1
	updatedLegacySong1.Rating = 1.0
	now = t.getNowFromServer()
	t.doPost("rate_and_tag?songId="+id+"&rating=1.0", nil)
	if err := compareQueryResults([]db.Song{updatedLegacySong1},
		t.getSongsForAndroid(now, getRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	// Reporting a play shouldn't update the song's last-modified time.
	log.Print("reporting playback")
	p := db.NewPlay(time.Unix(1410746718, 0), "127.0.0.1")
	updatedLegacySong1.Plays = append(updatedLegacySong1.Plays, p)
	now = t.getNowFromServer()
	t.doPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, p.StartTime.Unix()), nil)
	if err := compareQueryResults([]db.Song{},
		t.getSongsForAndroid(now, getRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestTags(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("getting hopefully-empty tag list")
	if tags := t.getTags(false); len(tags) > 0 {
		tt.Errorf("got unexpected tags %q", tags)
	}

	log.Print("posting song and getting tags")
	t.postSongs([]db.Song{LegacySong1}, true, 0)
	if tags := t.getTags(false); tags != "electronic,instrumental" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("posting another song and getting tags")
	t.postSongs([]db.Song{LegacySong2}, true, 0)
	if tags := t.getTags(false); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("checking that tags are cached")
	if tags := t.getTags(true); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("checking that datastore cache is used after memcache miss")
	t.doPost("flush_cache?onlyMemcache=1", nil)
	if tags := t.getTags(true); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("adding tags and checking that they're returned")
	id := t.SongID(LegacySong1.SHA1)
	t.doPost("rate_and_tag?songId="+id+"&tags=electronic+instrumental+drums+idm", nil)
	if tags := t.getTags(false); tags != "drums,electronic,idm,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}
}

func TestCovers(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	createCover := func(fn string) {
		f, err := os.Create(filepath.Join(t.coverDir, fn))
		if err != nil {
			panic(err)
		}
		if err := f.Close(); err != nil {
			panic(err)
		}
	}

	log.Print("writing cover and updating songs")
	test.CopySongs(t.musicDir, Song0s.Filename, Song5s.Filename)
	s5 := Song5s
	s5.CoverFilename = fmt.Sprintf("%s.jpg", s5.AlbumID)
	createCover(s5.CoverFilename)
	t.updateSongs()
	if err := compareQueryResults([]db.Song{Song0s, s5}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("writing another cover and updating again")
	test.DeleteSongs(t.musicDir, Song0s.Filename)
	test.CopySongs(t.musicDir, Song0sUpdated.Filename)
	s0 := Song0sUpdated
	s0.CoverFilename = fmt.Sprintf("%s.jpg", s0.AlbumID)
	createCover(s0.CoverFilename)
	t.updateSongs()
	if err := compareQueryResults([]db.Song{s0, s5}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("writing cover named after recording id")
	test.CopySongs(t.musicDir, Song1s.Filename)
	s1 := Song1s
	s1.CoverFilename = fmt.Sprintf("%s.jpg", s1.RecordingID)
	createCover(s1.CoverFilename)
	test.DeleteSongs(t.coverDir, s0.CoverFilename)
	t.updateSongs()
	if err := compareQueryResults([]db.Song{s0, s1, s5}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that covers are dumped (or not) as requested")
	if err := test.CompareSongs([]db.Song{s0, s1, s5},
		t.dumpSongs(stripIDs, dumpCoversFlag), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	s0.CoverFilename = ""
	s1.CoverFilename = ""
	s5.CoverFilename = ""
	if err := test.CompareSongs([]db.Song{s0, s1, s5},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestJSONImport(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("importing songs from json file")
	t.importSongsFromJSONFile(test.WriteSongsToJSONFile(t.tempDir,
		[]db.Song{LegacySong1, LegacySong2}))
	if err := test.CompareSongs([]db.Song{LegacySong1, LegacySong2},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("updating song from json file")
	us := LegacySong1
	us.Artist += " bogus"
	us.Title += " bogus"
	us.Album += " bogus"
	us.Track += 1
	us.Disc += 1
	us.Length *= 2
	us.Rating /= 2.0
	us.Plays = us.Plays[0:1]
	us.Tags = []string{"bogus"}
	t.importSongsFromJSONFile(test.WriteSongsToJSONFile(t.tempDir,
		[]db.Song{us, LegacySong2}))
	if err := test.CompareSongs([]db.Song{us, LegacySong2},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("reporting play")
	id := t.SongID(us.SHA1)
	st := time.Unix(1410746718, 0)
	t.doPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, st.Unix()), nil)
	us.Plays = append(us.Plays, db.NewPlay(st, "127.0.0.1"))
	if err := test.CompareSongs([]db.Song{us, LegacySong2},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("updating song from json file but preserving user data")
	t.importSongsFromJSONFile(test.WriteSongsToJSONFile(t.tempDir,
		[]db.Song{LegacySong1, LegacySong2}),
		keepUserDataFlag)
	us2 := LegacySong1
	us2.Rating = us.Rating
	us2.Tags = us.Tags
	us2.Plays = us.Plays
	if err := test.CompareSongs([]db.Song{us2, LegacySong2},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestUpdateList(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	test.CopySongs(t.musicDir, Song0s.Filename, Song1s.Filename, Song5s.Filename)
	listPath := test.WriteSongPathsFile(t.tempDir, Song0s.Filename, Song5s.Filename)

	gs0 := Song0s
	gs0.TrackGain = -8.4
	gs0.AlbumGain = -7.6
	gs0.PeakAmp = 1.2

	gs5 := Song5s
	gs5.TrackGain = -6.3
	gs5.AlbumGain = -7.1
	gs5.PeakAmp = 0.9

	log.Print("updating songs from list")
	t.updateSongsFromList(listPath)
	if err := test.CompareSongs([]db.Song{Song0s, Song5s},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	// When a dump file is passed, its gain info should be sent to the server.
	log.Print("updating songs from list with dumped gains")
	dumpPath := test.WriteSongsToJSONFile(t.tempDir,
		[]db.Song{gs0, gs5})
	t.updateSongsFromList(listPath, dumpedGainsFlag(dumpPath))
	if err := test.CompareSongs([]db.Song{gs0, gs5},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestSorting(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

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

	log.Print("importing songs and checking sort order")
	t.importSongsFromJSONFile(test.WriteSongsToJSONFile(t.tempDir, songs))
	if err := compareQueryResults(songs, t.querySongs(), test.CompareOrder); err != nil {
		tt.Error(err)
	}
}

func TestDeleteSong(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("posting songs and deleting first song")
	postTime := t.getNowFromServer()
	t.postSongs([]db.Song{LegacySong1, LegacySong2}, true, 0)
	id1 := t.SongID(LegacySong1.SHA1)
	t.deleteSong(id1)

	log.Print("checking non-deleted song")
	if err := compareQueryResults([]db.Song{LegacySong2}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{LegacySong2},
		t.getSongsForAndroid(time.Time{}, getRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{LegacySong2},
		t.getSongsForAndroid(postTime, getRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := test.CompareSongs([]db.Song{LegacySong2},
		t.dumpSongs(stripIDs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that deleted song is in android query")
	deletedSongs := t.getSongsForAndroid(postTime, getDeletedSongs)
	if err := compareQueryResults([]db.Song{LegacySong1}, deletedSongs, test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if deletedSongs[0].SongID != id1 {
		tt.Errorf("deleted song's id (%v) didn't match original id (%v)",
			deletedSongs[0].SongID, id1)
	}

	log.Print("deleting second song")
	laterTime := t.getNowFromServer()
	id2 := t.SongID(LegacySong2.SHA1)
	t.deleteSong(id2)

	log.Print("checking no non-deleted songs")
	if err := compareQueryResults([]db.Song{}, t.querySongs(), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]db.Song{},
		t.getSongsForAndroid(time.Time{}, getRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := test.CompareSongs([]db.Song{}, t.dumpSongs(stripIDs),
		test.IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that both deleted songs are in android query")
	deletedSongs = t.getSongsForAndroid(laterTime, getDeletedSongs)
	if err := compareQueryResults([]db.Song{LegacySong2}, deletedSongs, test.IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if deletedSongs[0].SongID != id2 {
		tt.Errorf("deleted song's id (%v) didn't match original id (%v)",
			deletedSongs[0].SongID, id2)
	}
}
