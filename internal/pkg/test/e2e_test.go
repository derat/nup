// Copyright 2020 Daniel Erat.
// All rights reserved.

package test

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
	"testing"
	"time"

	"github.com/derat/nup/server/query"
	"github.com/derat/nup/server/types"
)

const (
	server      = "http://localhost:8080/"
	songBucket  = "song-bucket"
	coverBucket = "cover-bucket"
)

var binDir string = filepath.Join(os.Getenv("GOPATH"), "bin")

func setUpTest() *Tester {
	t := newTester(server, binDir)
	if err := t.PingServer(); err != nil {
		log.Printf("Unable to connect to server: %v\n", err)
		log.Printf("Run dev_appserver.py, maybe?")
		os.Exit(1)
	}
	log.Printf("clearing all data on %v", server)
	t.DoPost("clear", nil)
	t.DoPost("flush_cache", nil)

	b, err := json.Marshal(types.ServerConfig{
		SongBucket:  songBucket,
		CoverBucket: coverBucket,
	})
	if err != nil {
		panic(err)
	}
	t.DoPost("config", bytes.NewBuffer(b))

	return t
}

func cleanUpTest(t *Tester) {
	t.DoPost("config", nil)
	t.CleanUp()
}

func compareQueryResults(expected, actual []types.Song, order OrderPolicy) error {
	expectedCleaned := make([]types.Song, len(expected))
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

	actualCleaned := make([]types.Song, len(actual))
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

	return CompareSongs(expectedCleaned, actualCleaned, order)
}

func timeToSeconds(t time.Time) float64 {
	return float64(t.UnixNano()) / float64(time.Second/time.Nanosecond)
}

func TestUpdate(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("importing songs from music dir")
	CopySongsToTempDir(t.MusicDir, Song0s.Filename, Song1s.Filename)
	t.UpdateSongs()
	if err := CompareSongs([]types.Song{Song0s, Song1s},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("importing another song")
	CopySongsToTempDir(t.MusicDir, Song5s.Filename)
	t.UpdateSongs()
	if err := CompareSongs([]types.Song{Song0s, Song1s, Song5s},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("updating a song")
	RemoveFromTempDir(t.MusicDir, Song0s.Filename)
	CopySongsToTempDir(t.MusicDir, Song0sUpdated.Filename)
	t.UpdateSongs()
	if err := CompareSongs([]types.Song{Song0sUpdated, Song1s, Song5s},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestUserData(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("importing a song")
	CopySongsToTempDir(t.MusicDir, Song0s.Filename)
	t.UpdateSongs()
	id := t.SongID(Song0s.SHA1)

	log.Print("rating and tagging")
	s := Song0s
	s.Rating = 0.75
	s.Tags = []string{"electronic", "instrumental"}
	t.DoPost("rate_and_tag?songId="+id+"&rating=0.75&tags=electronic+instrumental", nil)
	if err := CompareSongs([]types.Song{s},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	log.Print("reporting playback")
	s.Plays = []types.Play{
		types.NewPlay(time.Unix(1410746718, 0), "127.0.0.1"),
		types.NewPlay(time.Unix(1410746923, 0), "127.0.0.1"),
		types.NewPlay(time.Unix(1410747184, 0), "127.0.0.1"),
	}
	for _, p := range s.Plays {
		t.DoPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, p.StartTime.Unix()), nil)
	}
	if err := CompareSongs([]types.Song{s},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	log.Print("updating song and checking that user data is preserved")
	RemoveFromTempDir(t.MusicDir, s.Filename)
	us := Song0sUpdated
	us.Rating = s.Rating
	us.Tags = s.Tags
	us.Plays = s.Plays
	CopySongsToTempDir(t.MusicDir, us.Filename)
	t.UpdateSongs()
	if err := CompareSongs([]types.Song{us},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that duplicate plays are ignored")
	t.DoPost(fmt.Sprintf("played?songId=%v&startTime=%v",
		id, s.Plays[len(us.Plays)-1].StartTime.Unix()), nil)
	if err := CompareSongs([]types.Song{us},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	log.Print("checking that duplicate tags are ignored")
	us.Tags = []string{"electronic", "rock"}
	t.DoPost("rate_and_tag?songId="+id+"&tags=electronic+electronic+rock+electronic", nil)
	if err := CompareSongs([]types.Song{us},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	log.Print("clearing tags")
	us.Tags = nil
	t.DoPost("rate_and_tag?songId="+id+"&tags=", nil)
	if err := CompareSongs([]types.Song{us},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Fatal(err)
	}

	plays := us.Plays
	sort.Sort(types.PlayArray(plays))

	log.Print("checking first-played queries")
	firstPlaySec := timeToSeconds(plays[0].StartTime)
	if err := compareQueryResults([]types.Song{us},
		t.QuerySongs(fmt.Sprintf("minFirstPlayed=%.1f", firstPlaySec-10)),
		IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{},
		t.QuerySongs(fmt.Sprintf("minFirstPlayed=%.1f", firstPlaySec+10)),
		IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking last-played queries")
	lastPlaySec := timeToSeconds(plays[len(plays)-1].StartTime)
	if err := compareQueryResults([]types.Song{},
		t.QuerySongs(fmt.Sprintf("maxLastPlayed=%.1f", lastPlaySec-10)),
		IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{us},
		t.QuerySongs(fmt.Sprintf("maxLastPlayed=%.1f", lastPlaySec+10)),
		IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that play stats were updated")
	for i := 0; i < 3; i++ {
		if err := compareQueryResults([]types.Song{},
			t.QuerySongs("maxPlays="+strconv.Itoa(i)), IgnoreOrder); err != nil {
			tt.Error(err)
		}
	}
	if err := compareQueryResults([]types.Song{us},
		t.QuerySongs("maxPlays=3"), IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestQueries(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("posting some songs")
	t.PostSongs([]types.Song{LegacySong1, LegacySong2}, replaceUserData, 0)
	t.PostSongs([]types.Song{Song0s, Song1s, Song5s}, keepUserData, 0)

	log.Print("doing a bunch of queries")
	for _, q := range []struct {
		params []string
		exp    []types.Song
	}{
		{[]string{"artist=AROVANE"}, []types.Song{LegacySong1}},
		{[]string{"title=thaem+nue"}, []types.Song{LegacySong1}},
		{[]string{"album=ATOL+scrap"}, []types.Song{LegacySong1}},
		{[]string{"albumId=1e477f68-c407-4eae-ad01-518528cedc2c"}, []types.Song{Song0s, Song1s}},
		{[]string{"album=Another+Album", "albumId=a1d2405b-afe0-4e28-a935-b5b256f68131"}, []types.Song{Song5s}},
		{[]string{"keywords=arovane+thaem+atol"}, []types.Song{LegacySong1}},
		{[]string{"keywords=arovane+foo"}, []types.Song{}},
		{[]string{"minRating=1.0"}, []types.Song{}},
		{[]string{"minRating=0.75"}, []types.Song{LegacySong1}},
		{[]string{"minRating=0.5"}, []types.Song{LegacySong2, LegacySong1}},
		{[]string{"minRating=0.0"}, []types.Song{LegacySong2, LegacySong1}},
		{[]string{"unrated=1"}, []types.Song{Song5s, Song0s, Song1s}},
		{[]string{"tags=instrumental"}, []types.Song{LegacySong2, LegacySong1}},
		{[]string{"tags=electronic+instrumental"}, []types.Song{LegacySong1}},
		{[]string{"tags=-electronic+instrumental"}, []types.Song{LegacySong2}},
		{[]string{"tags=instrumental", "minRating=0.75"}, []types.Song{LegacySong1}},
	} {
		if err := compareQueryResults(q.exp, t.QuerySongs(q.params...), CompareOrder); err != nil {
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
	t.PostSongs([]types.Song{s1}, replaceUserData, 0)
	if err := compareQueryResults([]types.Song{s1}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	// After rating the song, the query results should still be served from the cache.
	log.Print("rating and re-querying")
	id1 := t.SongID(s1.SHA1)
	s1.Rating = 1.0
	t.DoPost("rate_and_tag?songId="+id1+"&rating=1.0", nil)
	if err := compareQueryResults([]types.Song{s1}, t.QuerySongs(cacheParam), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	// After updating metadata, the updated song should be returned (indicating
	// that the cached results were dropped).
	log.Print("updating and re-querying")
	s1.Artist = "The Artist Formerly Known As " + s1.Artist
	t.PostSongs([]types.Song{s1}, keepUserData, 0)
	if err := compareQueryResults([]types.Song{s1}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that time-based queries aren't cached")
	timeParam := fmt.Sprintf("maxLastPlayed=%d", s1.Plays[1].StartTime.Unix()+1)
	if err := compareQueryResults([]types.Song{s1}, t.QuerySongs(timeParam), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{}, t.QuerySongs(timeParam, cacheParam), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that play-count-based queries aren't cached")
	playParam := "maxPlays=10"
	if err := compareQueryResults([]types.Song{s1}, t.QuerySongs(playParam), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{}, t.QuerySongs(playParam, cacheParam), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that datastore cache is used after memcache miss")
	if err := compareQueryResults([]types.Song{s1}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	t.DoPost("flush_cache?onlyMemcache=1", nil)
	if err := compareQueryResults([]types.Song{s1}, t.QuerySongs(cacheParam), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that posting a song drops cached queries")
	s2 := LegacySong2
	t.PostSongs([]types.Song{s2}, replaceUserData, 0)
	if err := compareQueryResults([]types.Song{s1, s2}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that deleting a song drops cached queries")
	if err := compareQueryResults([]types.Song{s2},
		t.QuerySongs("album="+url.QueryEscape(s2.Album)), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	id2 := t.SongID(s2.SHA1)
	t.DeleteSong(id2)
	if err := compareQueryResults([]types.Song{},
		t.QuerySongs("album="+url.QueryEscape(s2.Album)), IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestAndroid(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("posting songs")
	now := t.GetNowFromServer()
	t.PostSongs([]types.Song{LegacySong1, LegacySong2}, replaceUserData, 0)
	if err := compareQueryResults([]types.Song{LegacySong1, LegacySong2},
		t.GetSongsForAndroid(time.Time{}, getRegularSongs), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{LegacySong1, LegacySong2},
		t.GetSongsForAndroid(now, getRegularSongs), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{},
		t.GetSongsForAndroid(t.GetNowFromServer(), getRegularSongs), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("rating a song")
	id := t.SongID(LegacySong1.SHA1)
	updatedLegacySong1 := LegacySong1
	updatedLegacySong1.Rating = 1.0
	now = t.GetNowFromServer()
	t.DoPost("rate_and_tag?songId="+id+"&rating=1.0", nil)
	if err := compareQueryResults([]types.Song{updatedLegacySong1},
		t.GetSongsForAndroid(now, getRegularSongs), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	// Reporting a play shouldn't update the song's last-modified time.
	log.Print("reporting playback")
	p := types.NewPlay(time.Unix(1410746718, 0), "127.0.0.1")
	updatedLegacySong1.Plays = append(updatedLegacySong1.Plays, p)
	now = t.GetNowFromServer()
	t.DoPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, p.StartTime.Unix()), nil)
	if err := compareQueryResults([]types.Song{},
		t.GetSongsForAndroid(now, getRegularSongs), IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestTags(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("getting hopefully-empty tag list")
	if tags := t.GetTags(false); len(tags) > 0 {
		tt.Errorf("got unexpected tags %q", tags)
	}

	log.Print("posting song and getting tags")
	t.PostSongs([]types.Song{LegacySong1}, replaceUserData, 0)
	if tags := t.GetTags(false); tags != "electronic,instrumental" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("posting another song and getting tags")
	t.PostSongs([]types.Song{LegacySong2}, replaceUserData, 0)
	if tags := t.GetTags(false); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("checking that tags are cached")
	if tags := t.GetTags(true); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("checking that datastore cache is used after memcache miss")
	t.DoPost("flush_cache?onlyMemcache=1", nil)
	if tags := t.GetTags(true); tags != "electronic,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}

	log.Print("adding tags and checking that they're returned")
	id := t.SongID(LegacySong1.SHA1)
	t.DoPost("rate_and_tag?songId="+id+"&tags=electronic+instrumental+drums+idm", nil)
	if tags := t.GetTags(false); tags != "drums,electronic,idm,instrumental,rock" {
		tt.Errorf("got tags %q", tags)
	}
}

func TestCovers(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	createCover := func(fn string) {
		f, err := os.Create(filepath.Join(t.CoverDir, fn))
		if err != nil {
			panic(err)
		}
		f.Close()
	}

	log.Print("writing cover and updating songs")
	CopySongsToTempDir(t.MusicDir, Song0s.Filename, Song5s.Filename)
	s5 := Song5s
	s5.CoverFilename = fmt.Sprintf("%s.jpg", s5.AlbumID)
	createCover(s5.CoverFilename)
	t.UpdateSongs()
	if err := compareQueryResults([]types.Song{Song0s, s5}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("writing another cover and updating again")
	RemoveFromTempDir(t.MusicDir, Song0s.Filename)
	CopySongsToTempDir(t.MusicDir, Song0sUpdated.Filename)
	s0 := Song0sUpdated
	s0.CoverFilename = fmt.Sprintf("%s.jpg", s0.AlbumID)
	createCover(s0.CoverFilename)
	t.UpdateSongs()
	if err := compareQueryResults([]types.Song{s0, s5}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("writing cover named after recording id")
	CopySongsToTempDir(t.MusicDir, Song1s.Filename)
	s1 := Song1s
	s1.CoverFilename = fmt.Sprintf("%s.jpg", s1.RecordingID)
	createCover(s1.CoverFilename)
	RemoveFromTempDir(t.CoverDir, s0.CoverFilename)
	t.UpdateSongs()
	if err := compareQueryResults([]types.Song{s0, s1, s5}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that covers are dumped (or not) as requested")
	if err := CompareSongs([]types.Song{s0, s1, s5},
		t.DumpSongs(stripIDs, getCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	s0.CoverFilename = ""
	s1.CoverFilename = ""
	s5.CoverFilename = ""
	if err := CompareSongs([]types.Song{s0, s1, s5},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestJSONImport(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("importing songs from json file")
	t.ImportSongsFromJSONFile(WriteSongsToJSONFile(t.TempDir, []types.Song{LegacySong1, LegacySong2}),
		replaceUserData)
	if err := CompareSongs([]types.Song{LegacySong1, LegacySong2},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
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
	t.ImportSongsFromJSONFile(WriteSongsToJSONFile(t.TempDir, []types.Song{us, LegacySong2}),
		replaceUserData)
	if err := CompareSongs([]types.Song{us, LegacySong2},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("reporting play")
	id := t.SongID(us.SHA1)
	st := time.Unix(1410746718, 0)
	t.DoPost(fmt.Sprintf("played?songId=%v&startTime=%v", id, st.Unix()), nil)
	us.Plays = append(us.Plays, types.NewPlay(st, "127.0.0.1"))
	if err := CompareSongs([]types.Song{us, LegacySong2},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("updating song from json file but preserving user data")
	t.ImportSongsFromJSONFile(WriteSongsToJSONFile(t.TempDir, []types.Song{LegacySong1, LegacySong2}),
		keepUserData)
	us2 := LegacySong1
	us2.Rating = us.Rating
	us2.Tags = us.Tags
	us2.Plays = us.Plays
	if err := CompareSongs([]types.Song{us2, LegacySong2},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}
}

func TestSorting(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	songs := make([]types.Song, 0)
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
		songs = append(songs, types.Song{
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
	t.ImportSongsFromJSONFile(WriteSongsToJSONFile(t.TempDir, songs), replaceUserData)
	if err := compareQueryResults(songs, t.QuerySongs(), CompareOrder); err != nil {
		tt.Error(err)
	}
}

func TestDeleteSong(tt *testing.T) {
	t := setUpTest()
	defer cleanUpTest(t)

	log.Print("posting songs and deleting first song")
	postTime := t.GetNowFromServer()
	t.PostSongs([]types.Song{LegacySong1, LegacySong2}, replaceUserData, 0)
	id1 := t.SongID(LegacySong1.SHA1)
	t.DeleteSong(id1)

	log.Print("checking non-deleted song")
	if err := compareQueryResults([]types.Song{LegacySong2}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{LegacySong2},
		t.GetSongsForAndroid(time.Time{}, getRegularSongs), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{LegacySong2},
		t.GetSongsForAndroid(postTime, getRegularSongs), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := CompareSongs([]types.Song{LegacySong2},
		t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that deleted song is in android query")
	deletedSongs := t.GetSongsForAndroid(postTime, getDeletedSongs)
	if err := compareQueryResults([]types.Song{LegacySong1}, deletedSongs, IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if deletedSongs[0].SongID != id1 {
		tt.Errorf("deleted song's id (%v) didn't match original id (%v)",
			deletedSongs[0].SongID, id1)
	}

	log.Print("deleting second song")
	laterTime := t.GetNowFromServer()
	id2 := t.SongID(LegacySong2.SHA1)
	t.DeleteSong(id2)

	log.Print("checking no non-deleted songs")
	if err := compareQueryResults([]types.Song{}, t.QuerySongs(), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]types.Song{},
		t.GetSongsForAndroid(time.Time{}, getRegularSongs), IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if err := CompareSongs([]types.Song{}, t.DumpSongs(stripIDs, skipCovers), IgnoreOrder); err != nil {
		tt.Error(err)
	}

	log.Print("checking that both deleted songs are in android query")
	deletedSongs = t.GetSongsForAndroid(laterTime, getDeletedSongs)
	if err := compareQueryResults([]types.Song{LegacySong2}, deletedSongs, IgnoreOrder); err != nil {
		tt.Error(err)
	}
	if deletedSongs[0].SongID != id2 {
		tt.Errorf("deleted song's id (%v) didn't match original id (%v)",
			deletedSongs[0].SongID, id2)
	}
}
