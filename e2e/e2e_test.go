package e2e

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"erat.org/nup"
	"erat.org/nup/test"
)

var server string = "http://localhost:8080/"
var binDir string = filepath.Join(os.Getenv("GOPATH"), "bin")

func setUpTest() *Tester {
	t := newTester(server, binDir)
	log.Printf("clearing all data on %v", server)
	t.DoPost("clear")
	t.WaitForUpdate()
	return t
}

func compareQueryResults(expected, actual []nup.Song, compareOrder bool) error {
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

	return test.CompareSongs(expectedCleaned, actualCleaned, compareOrder)
}

func TestLegacy(tc *testing.T) {
	t := setUpTest()
	defer os.RemoveAll(t.TempDir)

	log.Print("importing songs from legacy db")
	t.ImportSongsFromLegacyDb("../test/data/legacy.db")
	t.WaitForUpdate()
	if err := test.CompareSongs([]nup.Song{test.LegacySong1, test.LegacySong2}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}

	log.Print("testing that play stats were generated correctly")
	firstPlay := test.LegacySong1.Plays[0].StartTime
	beforeFirstPlay := strconv.Itoa(int(time.Now().Sub(firstPlay)/time.Second) + 10)
	songs := t.QuerySongs("artist=" + url.QueryEscape(test.LegacySong1.Artist) + "&firstPlayed=" + beforeFirstPlay)
	if err := compareQueryResults([]nup.Song{test.LegacySong1}, songs, true); err != nil {
		tc.Error(err)
	}
	afterFirstPlay := strconv.Itoa(int(time.Now().Sub(firstPlay)/time.Second) - 10)
	songs = t.QuerySongs("artist=" + url.QueryEscape(test.LegacySong1.Artist) + "&firstPlayed=" + afterFirstPlay)
	if err := compareQueryResults([]nup.Song{}, songs, true); err != nil {
		tc.Error(err)
	}

	lastPlay := test.LegacySong1.Plays[1].StartTime
	beforeLastPlay := strconv.Itoa(int(time.Now().Sub(lastPlay)/time.Second) + 10)
	songs = t.QuerySongs("artist=" + url.QueryEscape(test.LegacySong1.Artist) + "&lastPlayed=" + beforeLastPlay)
	if err := compareQueryResults([]nup.Song{}, songs, true); err != nil {
		tc.Error(err)
	}
	afterLastPlay := strconv.Itoa(int(time.Now().Sub(lastPlay)/time.Second) - 10)
	songs = t.QuerySongs("artist=" + url.QueryEscape(test.LegacySong1.Artist) + "&lastPlayed=" + afterLastPlay)
	if err := compareQueryResults([]nup.Song{test.LegacySong1}, songs, true); err != nil {
		tc.Error(err)
	}

	if err := compareQueryResults([]nup.Song{}, t.QuerySongs("maxPlays=0"), true); err != nil {
		tc.Error(err)
	}
	if err := compareQueryResults([]nup.Song{test.LegacySong2}, t.QuerySongs("maxPlays=1"), true); err != nil {
		tc.Error(err)
	}
	if err := compareQueryResults([]nup.Song{test.LegacySong2, test.LegacySong1}, t.QuerySongs("maxPlays=2"), true); err != nil {
		tc.Error(err)
	}
}

func TestUpdate(tc *testing.T) {
	t := setUpTest()
	defer os.RemoveAll(t.TempDir)

	log.Print("importing songs from music dir")
	test.CopySongsToTempDir(t.MusicDir, test.Song0s.Filename, test.Song1s.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	if err := test.CompareSongs([]nup.Song{test.Song0s, test.Song1s}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}

	log.Print("importing another song")
	test.CopySongsToTempDir(t.MusicDir, test.Song5s.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	if err := test.CompareSongs([]nup.Song{test.Song0s, test.Song1s, test.Song5s}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}

	log.Print("updating a song")
	test.RemoveFromTempDir(t.MusicDir, test.Song0s.Filename)
	test.CopySongsToTempDir(t.MusicDir, test.Song0sUpdated.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	if err := test.CompareSongs([]nup.Song{test.Song0sUpdated, test.Song1s, test.Song5s}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}
}

func TestUserData(tc *testing.T) {
	t := setUpTest()
	defer os.RemoveAll(t.TempDir)

	log.Print("importing a song")
	test.CopySongsToTempDir(t.MusicDir, test.Song0s.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	id := t.DumpSongs(false)[0].SongId

	log.Print("rating and tagging")
	s := test.Song0s
	s.Rating = 0.75
	s.Tags = []string{"electronic", "instrumental"}
	t.DoPost("rate_and_tag?songId=" + id + "&rating=0.75&tags=electronic+instrumental")
	t.WaitForUpdate()
	if err := test.CompareSongs([]nup.Song{s}, t.DumpSongs(true), false); err != nil {
		tc.Fatal(err)
	}

	log.Print("reporting playback")
	s.Plays = []nup.Play{
		nup.Play{time.Unix(1410746718, 0), "127.0.0.1"},
		nup.Play{time.Unix(1410746923, 0), "127.0.0.1"},
		nup.Play{time.Unix(1410747184, 0), "127.0.0.1"},
	}
	for _, p := range s.Plays {
		t.DoPost("report_played?songId=" + id + "&startTime=" + strconv.FormatInt(p.StartTime.Unix(), 10))
	}
	t.WaitForUpdate()
	if err := test.CompareSongs([]nup.Song{s}, t.DumpSongs(true), false); err != nil {
		tc.Fatal(err)
	}

	log.Print("updating song and checking that user data is preserved")
	test.RemoveFromTempDir(t.MusicDir, s.Filename)
	us := test.Song0sUpdated
	us.Rating = s.Rating
	us.Tags = s.Tags
	us.Plays = s.Plays
	test.CopySongsToTempDir(t.MusicDir, us.Filename)
	t.UpdateSongs()
	t.WaitForUpdate()
	if err := test.CompareSongs([]nup.Song{us}, t.DumpSongs(true), false); err != nil {
		tc.Error(err)
	}

	log.Print("clearing tags")
	us.Tags = nil
	t.DoPost("rate_and_tag?songId=" + id + "&tags=")
	t.WaitForUpdate()
	if err := test.CompareSongs([]nup.Song{us}, t.DumpSongs(true), false); err != nil {
		tc.Fatal(err)
	}

	// TODO: check that play stats are updated
}

// TODO: miscellaneous queries
