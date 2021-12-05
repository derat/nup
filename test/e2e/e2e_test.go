// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package e2e contains end-to-end tests between the server and command-line tools.
package e2e

import (
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

	"github.com/derat/nup/server/config"
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

func initTest(t *testing.T) (tester *test.Tester, done func()) {
	tester = test.NewTester(t, server, binDir)
	done = func() {
		tester.SendConfig(nil)
		tester.Close()
	}

	// Clean up if we fail.
	success := false
	defer func() {
		if !success {
			done()
		}
	}()

	log.Printf("Configuring and clearing %v", server)
	tester.SendConfig(&config.Config{
		BasicAuthUsers: []config.BasicAuthInfo{{Username: test.Username, Password: test.Password}},
		SongBucket:     songBucket,
		CoverBucket:    coverBucket,
	})
	tester.DoPost("clear", nil)
	tester.DoPost("flush_cache", nil)

	success = true
	return tester, done
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
	dumpPath, err := test.WriteSongsToJSONFile(t.TempDir, []db.Song{gs5})
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
	s.Rating = 0.75
	s.Tags = []string{"electronic", "instrumental"}
	t.DoPost("rate_and_tag?songId="+id+"&rating=0.75&tags=electronic+instrumental", nil)
	if err := test.CompareSongs([]db.Song{s}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after rating and tagging: ", err)
	}

	log.Print("Reporting play")
	s.Plays = []db.Play{
		db.NewPlay(time.Unix(1410746718, 0), "127.0.0.1"),
		db.NewPlay(time.Unix(1410746923, 0), "127.0.0.1"),
		db.NewPlay(time.Unix(1410747184, 0), "127.0.0.1"),
	}
	for _, p := range s.Plays {
		t.DoPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, p.StartTime.Unix()), nil)
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
	t.DoPost(fmt.Sprintf("played?songId=%v&startTime=%v",
		id, s.Plays[len(us.Plays)-1].StartTime.Unix()), nil)
	if err := test.CompareSongs([]db.Song{us}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after duplicate play: ", err)
	}

	log.Print("Checking that duplicate tags are ignored")
	us.Tags = []string{"electronic", "rock"}
	t.DoPost("rate_and_tag?songId="+id+"&tags=electronic+electronic+rock+electronic", nil)
	if err := test.CompareSongs([]db.Song{us}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after duplicate tags: ", err)
	}

	log.Print("Clearing tags")
	us.Tags = nil
	t.DoPost("rate_and_tag?songId="+id+"&tags=", nil)
	if err := test.CompareSongs([]db.Song{us}, t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Fatal("Bad songs after clearing tags: ", err)
	}

	plays := us.Plays
	sort.Sort(db.PlayArray(plays))

	log.Print("Checking first-played queries")
	firstPlaySec := timeToSeconds(plays[0].StartTime)
	query := fmt.Sprintf("minFirstPlayed=%.1f", firstPlaySec-10)
	if err := compareQueryResults([]db.Song{us}, t.QuerySongs(query), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", query, err)
	}
	query = fmt.Sprintf("minFirstPlayed=%.1f", firstPlaySec+10)
	if err := compareQueryResults([]db.Song{}, t.QuerySongs(query), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", query, err)
	}

	log.Print("Checking last-played queries")
	lastPlaySec := timeToSeconds(plays[len(plays)-1].StartTime)
	query = fmt.Sprintf("maxLastPlayed=%.1f", lastPlaySec-10)
	if err := compareQueryResults([]db.Song{}, t.QuerySongs(query), test.IgnoreOrder); err != nil {
		tt.Errorf("Bad results for %q: %v", query, err)
	}
	query = fmt.Sprintf("maxLastPlayed=%.1f", lastPlaySec+10)
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

func TestQueries(tt *testing.T) {
	t, done := initTest(tt)
	defer done()

	log.Print("Posting some songs")
	t.PostSongs([]db.Song{LegacySong1, LegacySong2}, true, 0)
	t.PostSongs([]db.Song{Song0s, Song1s, Song5s}, false, 0)

	log.Print("Doing a bunch of queries")
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
		if err := compareQueryResults(q.exp, t.QuerySongs(q.params...), test.CompareOrder); err != nil {
			tt.Errorf("%v: %v", q.params, err)
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
	s1.Rating = 1.0
	t.DoPost("rate_and_tag?songId="+id1+"&rating=1.0", nil)
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
	timeParam := fmt.Sprintf("maxLastPlayed=%d", s1.Plays[1].StartTime.Unix()+1)
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
	t.DoPost("flush_cache?onlyMemcache=1", nil)
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
	updatedLegacySong1.Rating = 1.0
	now = t.GetNowFromServer()
	t.DoPost("rate_and_tag?songId="+id+"&rating=1.0", nil)
	if err := compareQueryResults([]db.Song{updatedLegacySong1},
		t.GetSongsForAndroid(now, test.GetRegularSongs), test.IgnoreOrder); err != nil {
		tt.Error("Bad results after rating and tagging: ", err)
	}

	// Reporting a play shouldn't update the song's last-modified time.
	log.Print("Reporting play")
	p := db.NewPlay(time.Unix(1410746718, 0), "127.0.0.1")
	updatedLegacySong1.Plays = append(updatedLegacySong1.Plays, p)
	now = t.GetNowFromServer()
	t.DoPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, p.StartTime.Unix()), nil)
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
	t.DoPost("flush_cache?onlyMemcache=1", nil)
	if tags := t.GetTags(true); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("Adding tags and checking that they're returned")
	id := t.SongID(LegacySong1.SHA1)
	t.DoPost("rate_and_tag?songId="+id+"&tags=electronic+instrumental+drums+idm", nil)
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

	log.Print("Checking that covers are dumped (or not) as requested")
	if err := test.CompareSongs([]db.Song{s0, s1, s5},
		t.DumpSongs(test.StripIDs, test.DumpCoversFlag), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs when dumping covers: ", err)
	}
	s0.CoverFilename = ""
	s1.CoverFilename = ""
	s5.CoverFilename = ""
	if err := test.CompareSongs([]db.Song{s0, s1, s5},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs when not dumping covers: ", err)
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
	us.Rating /= 2.0
	us.Plays = us.Plays[0:1]
	us.Tags = []string{"bogus"}
	t.ImportSongsFromJSONFile([]db.Song{us, LegacySong2})
	if err := test.CompareSongs([]db.Song{us, LegacySong2},
		t.DumpSongs(test.StripIDs), test.IgnoreOrder); err != nil {
		tt.Error("Bad songs after updating from JSON: ", err)
	}

	log.Print("Reporting play")
	id := t.SongID(us.SHA1)
	st := time.Unix(1410746718, 0)
	t.DoPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, st.Unix()), nil)
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
	listPath, err := test.WriteSongPathsFile(t.TempDir, Song0s.Filename, Song5s.Filename)
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
	dumpPath, err := test.WriteSongsToJSONFile(t.TempDir, []db.Song{gs0, gs5})
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
