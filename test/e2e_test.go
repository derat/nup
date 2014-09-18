package test

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

	"erat.org/nup"
)

var server string = "http://localhost:8080/"
var binDir string = filepath.Join(os.Getenv("GOPATH"), "bin")

func setUpTest() *Tester {
	t := newTester(server, binDir)
	log.Printf("clearing all data on %v", server)
	t.DoPost("clear", nil)
	t.WaitForUpdate()
	return t
}

func compareQueryResults(expected, actual []nup.Song) error {
	actualCleaned := make([]nup.Song, len(actual))
	for i := range actual {
		s := actual[i]

		if len(s.SongId) == 0 {
			return fmt.Errorf("song %v (%v) has no ID", i, s.Url)
		}
		s.SongId = ""

		if len(s.Tags) == 0 {
			s.Tags = nil
		}
		if i < len(expected) && strings.HasSuffix(s.Url, expected[i].Filename) {
			s.Url = ""
			s.Filename = expected[i].Filename
		}
		actualCleaned[i] = s
	}

	expectedCleaned := make([]nup.Song, len(expected))
	for i := range expected {
		s := expected[i]
		s.Sha1 = ""
		s.Plays = nil
		expectedCleaned[i] = s
	}

	return CompareSongs(expectedCleaned, actualCleaned, true)
}

func doPlayTimeQueries(tc *testing.T, t *Tester, s *nup.Song, queryPrefix string) {
	if s.Plays == nil || len(s.Plays) == 0 {
		panic("song has no plays")
	}

	plays := s.Plays
	sort.Sort(PlayArray(plays))

	firstPlay := plays[0].StartTime
	beforeFirstPlay := strconv.Itoa(int(time.Now().Sub(firstPlay)/time.Second) + 10)
	songs := t.QuerySongs(queryPrefix + "firstPlayed=" + beforeFirstPlay)
	if err := compareQueryResults([]nup.Song{*s}, songs); err != nil {
		tc.Error(err)
	}
	afterFirstPlay := strconv.Itoa(int(time.Now().Sub(firstPlay)/time.Second) - 10)
	songs = t.QuerySongs(queryPrefix + "firstPlayed=" + afterFirstPlay)
	if err := compareQueryResults([]nup.Song{}, songs); err != nil {
		tc.Error(err)
	}

	lastPlay := plays[len(plays)-1].StartTime
	beforeLastPlay := strconv.Itoa(int(time.Now().Sub(lastPlay)/time.Second) + 10)
	songs = t.QuerySongs(queryPrefix + "lastPlayed=" + beforeLastPlay)
	if err := compareQueryResults([]nup.Song{}, songs); err != nil {
		tc.Error(err)
	}
	afterLastPlay := strconv.Itoa(int(time.Now().Sub(lastPlay)/time.Second) - 10)
	songs = t.QuerySongs(queryPrefix + "lastPlayed=" + afterLastPlay)
	if err := compareQueryResults([]nup.Song{*s}, songs); err != nil {
		tc.Error(err)
	}
}

func TestLegacy(tc *testing.T) {
	t := setUpTest()
	defer os.RemoveAll(t.TempDir)

	log.Print("importing songs from legacy db")
	t.ImportSongsFromLegacyDb("../test/data/legacy.db")
	t.WaitForUpdate()
	if err := CompareSongs([]nup.Song{LegacySong1, LegacySong2}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}

	log.Print("checking that play stats were generated correctly")
	doPlayTimeQueries(tc, t, &LegacySong1, "artist="+url.QueryEscape(LegacySong1.Artist)+"&")
	if err := compareQueryResults([]nup.Song{}, t.QuerySongs("maxPlays=0")); err != nil {
		tc.Error(err)
	}
	if err := compareQueryResults([]nup.Song{LegacySong2}, t.QuerySongs("maxPlays=1")); err != nil {
		tc.Error(err)
	}
	if err := compareQueryResults([]nup.Song{LegacySong2, LegacySong1}, t.QuerySongs("maxPlays=2")); err != nil {
		tc.Error(err)
	}
}

func TestUpdate(tc *testing.T) {
	t := setUpTest()
	defer os.RemoveAll(t.TempDir)

	log.Print("importing songs from music dir")
	CopySongsToTempDir(t.MusicDir, Song0s.Filename, Song1s.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	if err := CompareSongs([]nup.Song{Song0s, Song1s}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}

	log.Print("importing another song")
	CopySongsToTempDir(t.MusicDir, Song5s.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	if err := CompareSongs([]nup.Song{Song0s, Song1s, Song5s}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}

	log.Print("updating a song")
	RemoveFromTempDir(t.MusicDir, Song0s.Filename)
	CopySongsToTempDir(t.MusicDir, Song0sUpdated.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	if err := CompareSongs([]nup.Song{Song0sUpdated, Song1s, Song5s}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}
}

func TestUserData(tc *testing.T) {
	t := setUpTest()
	defer os.RemoveAll(t.TempDir)

	log.Print("importing a song")
	CopySongsToTempDir(t.MusicDir, Song0s.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	id := t.DumpSongs(false)[0].SongId

	log.Print("rating and tagging")
	s := Song0s
	s.Rating = 0.75
	s.Tags = []string{"electronic", "instrumental"}
	t.DoPost("rate_and_tag?songId="+id+"&rating=0.75&tags=electronic+instrumental", nil)
	t.WaitForUpdate()
	if err := CompareSongs([]nup.Song{s}, t.DumpSongs(true), false); err != nil {
		tc.Fatal(err)
	}

	log.Print("reporting playback")
	s.Plays = []nup.Play{
		nup.Play{time.Unix(1410746718, 0), "127.0.0.1"},
		nup.Play{time.Unix(1410746923, 0), "127.0.0.1"},
		nup.Play{time.Unix(1410747184, 0), "127.0.0.1"},
	}
	for _, p := range s.Plays {
		t.DoPost("report_played?songId="+id+"&startTime="+strconv.FormatInt(p.StartTime.Unix(), 10), nil)
	}
	t.WaitForUpdate()
	if err := CompareSongs([]nup.Song{s}, t.DumpSongs(true), false); err != nil {
		tc.Fatal(err)
	}

	log.Print("updating song and checking that user data is preserved")
	RemoveFromTempDir(t.MusicDir, s.Filename)
	us := Song0sUpdated
	us.Rating = s.Rating
	us.Tags = s.Tags
	us.Plays = s.Plays
	CopySongsToTempDir(t.MusicDir, us.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	if err := CompareSongs([]nup.Song{us}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}

	log.Print("clearing tags")
	us.Tags = nil
	t.DoPost("rate_and_tag?songId="+id+"&tags=", nil)
	t.WaitForUpdate()
	if err := CompareSongs([]nup.Song{us}, t.DumpSongs(true), false); err != nil {
		tc.Fatal(err)
	}

	log.Print("checking that play stats were updated")
	doPlayTimeQueries(tc, t, &us, "artist="+url.QueryEscape(us.Artist)+"&")
	for i := 0; i < 3; i++ {
		if err := compareQueryResults([]nup.Song{}, t.QuerySongs("maxPlays="+strconv.Itoa(i))); err != nil {
			tc.Error(err)
		}
	}
	if err := compareQueryResults([]nup.Song{us}, t.QuerySongs("maxPlays=3")); err != nil {
		tc.Error(err)
	}
}

func TestQueries(tc *testing.T) {
	t := setUpTest()
	defer os.RemoveAll(t.TempDir)

	log.Print("posting some songs")
	t.PostSongs([]nup.Song{LegacySong1, LegacySong2}, true)
	t.PostSongs([]nup.Song{Song0s}, false)
	t.WaitForUpdate()

	log.Print("doing a bunch of queries")
	for _, q := range []struct {
		Query         string
		ExpectedSongs []nup.Song
	}{
		{"artist=AROVANE", []nup.Song{LegacySong1}},
		{"title=thaem+nue", []nup.Song{LegacySong1}},
		{"album=ATOL+scrap", []nup.Song{LegacySong1}},
		{"keywords=arovane+thaem+atol", []nup.Song{LegacySong1}},
		{"keywords=arovane+foo", []nup.Song{}},
		{"minRating=1.0", []nup.Song{}},
		{"minRating=0.75", []nup.Song{LegacySong1}},
		{"minRating=0.5", []nup.Song{LegacySong2, LegacySong1}},
		{"minRating=0.0", []nup.Song{LegacySong2, LegacySong1}},
		{"unrated=1", []nup.Song{Song0s}},
		{"tags=instrumental", []nup.Song{LegacySong2, LegacySong1}},
		{"tags=electronic+instrumental", []nup.Song{LegacySong1}},
		{"tags=-electronic+instrumental", []nup.Song{LegacySong2}},
		{"tags=instrumental&minRating=0.75", []nup.Song{LegacySong1}},
	} {
		if err := compareQueryResults(q.ExpectedSongs, t.QuerySongs(q.Query)); err != nil {
			tc.Errorf("%v: %v", q.Query, err)
		}
	}
}

// TODO: android stuff
