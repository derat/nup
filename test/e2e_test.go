package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"erat.org/nup"
)

const (
	server      = "http://localhost:8080/"
	songBucket  = "song-bucket"
	coverBucket = "cover-bucket"
)

var binDir string = filepath.Join(os.Getenv("GOPATH"), "bin")

func setUpTest() *Tester {
	t := newTester(server, binDir)
	log.Printf("clearing all data on %v", server)
	t.DoPost("clear", nil)
	t.DoPost("flush_cache", nil)

	b, err := json.Marshal(nup.ServerConfig{
		SongBucket:                   songBucket,
		CoverBucket:                  coverBucket,
		CacheSongs:                   false,
		CacheQueries:                 false,
		UseDatastoreForCachedQueries: false,
	})
	if err != nil {
		panic(err)
	}
	t.DoPost("config", bytes.NewBuffer(b))

	return t
}

// extractFilePathFromUrl extracts the (escaped for Cloud Storage but un-query-escaped) original file path from a URL.
func extractFilePathFromUrl(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("unable to parse URL")
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("non-HTTPS scheme %q", u.Scheme)
	}
	if u.Host == "storage.cloud.google.com" {
		return regexp.MustCompile("^/[^/]+/").ReplaceAllLiteralString(u.Path, ""), nil
	} else if strings.HasSuffix(u.Host, ".storage.googleapis.com") && len(u.Path) > 0 {
		return u.Path[1:], nil
	} else {
		return "", fmt.Errorf("unrecognized URL")
	}
}

func compareQueryResults(expected, actual []nup.Song, compareOrder bool, client nup.ClientType) error {
	expectedCleaned := make([]nup.Song, len(expected))
	for i := range expected {
		s := expected[i]
		s.Sha1 = ""
		s.Plays = nil
		s.Url = nup.GetCloudStorageUrl(songBucket, s.Filename, client)
		s.Filename = ""
		expectedCleaned[i] = s
	}

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

		actualCleaned[i] = s
	}

	return CompareSongs(expectedCleaned, actualCleaned, compareOrder)
}

func doPlayTimeQueries(tt *testing.T, t *Tester, s *nup.Song, queryPrefix string) {
	if s.Plays == nil || len(s.Plays) == 0 {
		panic("song has no plays")
	}

	plays := s.Plays
	sort.Sort(nup.PlayArray(plays))

	firstPlaySec := nup.TimeToSeconds(plays[0].StartTime)
	beforeFirstPlay := strconv.FormatFloat(firstPlaySec-10, 'f', -1, 64)
	songs := t.QuerySongs(queryPrefix + "minFirstPlayed=" + beforeFirstPlay)
	if err := compareQueryResults([]nup.Song{*s}, songs, false, nup.WebClient); err != nil {
		tt.Error(err)
	}
	afterFirstPlay := strconv.FormatFloat(firstPlaySec+10, 'f', -1, 64)
	songs = t.QuerySongs(queryPrefix + "minFirstPlayed=" + afterFirstPlay)
	if err := compareQueryResults([]nup.Song{}, songs, false, nup.WebClient); err != nil {
		tt.Error(err)
	}

	lastPlaySec := nup.TimeToSeconds(plays[len(plays)-1].StartTime)
	beforeLastPlay := strconv.FormatFloat(lastPlaySec-10, 'f', -1, 64)
	songs = t.QuerySongs(queryPrefix + "maxLastPlayed=" + beforeLastPlay)
	if err := compareQueryResults([]nup.Song{}, songs, false, nup.WebClient); err != nil {
		tt.Error(err)
	}
	afterLastPlay := strconv.FormatFloat(lastPlaySec+10, 'f', -1, 64)
	songs = t.QuerySongs(queryPrefix + "maxLastPlayed=" + afterLastPlay)
	if err := compareQueryResults([]nup.Song{*s}, songs, false, nup.WebClient); err != nil {
		tt.Error(err)
	}
}

func TestLegacy(tt *testing.T) {
	t := setUpTest()
	defer t.CleanUp()

	log.Print("importing songs from legacy db")
	t.ImportSongsFromLegacyDb(filepath.Join(GetDataDir(), "legacy.db"))
	if err := CompareSongs([]nup.Song{LegacySong1, LegacySong2}, t.DumpSongs(true), false); err != nil {
		tt.Error(err)
	}

	log.Print("checking that play stats were generated correctly")
	doPlayTimeQueries(tt, t, &LegacySong1, "tags=electronic&")
	if err := compareQueryResults([]nup.Song{}, t.QuerySongs("maxPlays=0"), true, nup.WebClient); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]nup.Song{LegacySong2}, t.QuerySongs("maxPlays=1"), true, nup.WebClient); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]nup.Song{LegacySong2, LegacySong1}, t.QuerySongs("maxPlays=2"), true, nup.WebClient); err != nil {
		tt.Error(err)
	}
}

func TestUpdate(tt *testing.T) {
	t := setUpTest()
	defer t.CleanUp()

	log.Print("importing songs from music dir")
	CopySongsToTempDir(t.MusicDir, Song0s.Filename, Song1s.Filename)
	t.UpdateSongs()
	if err := CompareSongs([]nup.Song{Song0s, Song1s}, t.DumpSongs(true), false); err != nil {
		tt.Error(err)
	}

	log.Print("importing another song")
	CopySongsToTempDir(t.MusicDir, Song5s.Filename)
	t.UpdateSongs()
	if err := CompareSongs([]nup.Song{Song0s, Song1s, Song5s}, t.DumpSongs(true), false); err != nil {
		tt.Error(err)
	}

	log.Print("updating a song")
	RemoveFromTempDir(t.MusicDir, Song0s.Filename)
	CopySongsToTempDir(t.MusicDir, Song0sUpdated.Filename)
	t.UpdateSongs()
	if err := CompareSongs([]nup.Song{Song0sUpdated, Song1s, Song5s}, t.DumpSongs(true), false); err != nil {
		tt.Error(err)
	}
}

func TestUserData(tt *testing.T) {
	t := setUpTest()
	defer t.CleanUp()

	log.Print("importing a song")
	CopySongsToTempDir(t.MusicDir, Song0s.Filename)
	t.UpdateSongs()
	id := t.GetSongId(Song0s.Sha1)

	log.Print("rating and tagging")
	s := Song0s
	s.Rating = 0.75
	s.Tags = []string{"electronic", "instrumental"}
	t.DoPost("rate_and_tag?songId="+id+"&rating=0.75&tags=electronic+instrumental", nil)
	if err := CompareSongs([]nup.Song{s}, t.DumpSongs(true), false); err != nil {
		tt.Fatal(err)
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
	if err := CompareSongs([]nup.Song{s}, t.DumpSongs(true), false); err != nil {
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
	if err := CompareSongs([]nup.Song{us}, t.DumpSongs(true), false); err != nil {
		tt.Error(err)
	}

	log.Print("clearing tags")
	us.Tags = nil
	t.DoPost("rate_and_tag?songId="+id+"&tags=", nil)
	if err := CompareSongs([]nup.Song{us}, t.DumpSongs(true), false); err != nil {
		tt.Fatal(err)
	}

	log.Print("checking that play stats were updated")
	doPlayTimeQueries(tt, t, &us, "")
	for i := 0; i < 3; i++ {
		if err := compareQueryResults([]nup.Song{}, t.QuerySongs("maxPlays="+strconv.Itoa(i)), false, nup.WebClient); err != nil {
			tt.Error(err)
		}
	}
	if err := compareQueryResults([]nup.Song{us}, t.QuerySongs("maxPlays=3"), false, nup.WebClient); err != nil {
		tt.Error(err)
	}
}

func TestQueries(tt *testing.T) {
	t := setUpTest()
	defer t.CleanUp()

	log.Print("posting some songs")
	t.PostSongs([]nup.Song{LegacySong1, LegacySong2}, true, 0)
	t.PostSongs([]nup.Song{Song0s}, false, 0)

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
		if err := compareQueryResults(q.ExpectedSongs, t.QuerySongs(q.Query), true, nup.WebClient); err != nil {
			tt.Errorf("%v: %v", q.Query, err)
		}
	}
}

func TestCaching(tt *testing.T) {
	t := setUpTest()
	defer t.CleanUp()

	log.Print("posting and querying a song")
	t.PostSongs([]nup.Song{LegacySong1}, true, 0)
	if err := compareQueryResults([]nup.Song{LegacySong1}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}

	log.Print("rating and re-querying")
	id := t.GetSongId(LegacySong1.Sha1)
	s := LegacySong1
	s.Rating = 1.0
	t.DoPost("rate_and_tag?songId="+id+"&rating=1.0", nil)
	if err := compareQueryResults([]nup.Song{s}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}

	log.Print("updating and re-querying")
	s.Artist = "The Artist Formerly Known As " + s.Artist
	t.PostSongs([]nup.Song{s}, false, 0)
	if err := compareQueryResults([]nup.Song{s}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}

	log.Print("flushing cache and re-querying")
	t.DoPost("flush_cache", nil)
	if err := compareQueryResults([]nup.Song{s}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}

	log.Print("posting another song and querying")
	t.PostSongs([]nup.Song{LegacySong2}, true, 0)
	if err := compareQueryResults([]nup.Song{LegacySong2, s}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}
}

func TestCacheRace(tt *testing.T) {
	t := setUpTest()
	defer t.CleanUp()

	log.Print("posting a song")
	t.PostSongs([]nup.Song{LegacySong1}, true, 0)

	// Start a goroutine that posts a request that should:
	// a) Drop the song from the cache.
	// b) Sleep for one second.
	// c) Update the song.
	// d) See that the song has been re-added to the cache by the subsequent query and drop it again.
	log.Print("starting slow background update")
	id := t.GetSongId(LegacySong1.Sha1)
	s := LegacySong1
	s.Rating = 1.0
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		t.DoPost("rate_and_tag?songId="+id+"&rating=1.0&updateDelayNsec=1000000000", nil)
		wg.Done()
	}()

	// Meanwhile, sleep half a second and do a query. This should execute between b) and c) above (i.e.
	// the original song should be returned), and it should also result in the original song being re-cached.
	log.Print("querying for pre-update song")
	time.Sleep(time.Duration(500) * time.Millisecond)
	if err := compareQueryResults([]nup.Song{LegacySong1}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}

	// After the update finally runs, the server should see that the song got re-cached in the meantime
	// and drop it from the cache to ensure that stale data won't be served.
	log.Print("waiting for update to finish and querying for post-update song")
	wg.Wait()
	if err := compareQueryResults([]nup.Song{s}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}

	// Do the same thing with an import.
	log.Print("starting slow background import")
	s2 := s
	s2.Artist = "Some Other Artist"
	wg.Add(1)
	go func() {
		t.PostSongs([]nup.Song{s2}, false, time.Second)
		wg.Done()
	}()

	log.Print("querying for pre-import song")
	time.Sleep(time.Duration(500) * time.Millisecond)
	if err := compareQueryResults([]nup.Song{s}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}

	log.Print("waiting for import to finish and querying for post-import song")
	wg.Wait()
	if err := compareQueryResults([]nup.Song{s2}, t.QuerySongs(""), false, nup.WebClient); err != nil {
		tt.Error(err)
	}
}

func TestAndroid(tt *testing.T) {
	t := setUpTest()
	defer t.CleanUp()

	log.Print("posting songs")
	now := t.GetNowFromServer()
	t.PostSongs([]nup.Song{LegacySong1, LegacySong2}, true, 0)
	if err := compareQueryResults([]nup.Song{LegacySong1, LegacySong2}, t.GetSongsForAndroid(time.Time{}), false, nup.AndroidClient); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]nup.Song{LegacySong1, LegacySong2}, t.GetSongsForAndroid(now), false, nup.AndroidClient); err != nil {
		tt.Error(err)
	}
	if err := compareQueryResults([]nup.Song{}, t.GetSongsForAndroid(t.GetNowFromServer()), false, nup.AndroidClient); err != nil {
		tt.Error(err)
	}

	log.Print("rating a song")
	id := t.GetSongId(LegacySong1.Sha1)
	updatedLegacySong1 := LegacySong1
	updatedLegacySong1.Rating = 1.0
	now = t.GetNowFromServer()
	t.DoPost("rate_and_tag?songId="+id+"&rating=1.0", nil)
	if err := compareQueryResults([]nup.Song{updatedLegacySong1}, t.GetSongsForAndroid(now), false, nup.AndroidClient); err != nil {
		tt.Error(err)
	}

	// Reporting a play shouldn't update the song's last-modified time.
	log.Print("reporting playback")
	p := nup.Play{time.Unix(1410746718, 0), "127.0.0.1"}
	updatedLegacySong1.Plays = append(updatedLegacySong1.Plays, p)
	now = t.GetNowFromServer()
	t.DoPost("report_played?songId="+id+"&startTime="+strconv.FormatInt(p.StartTime.Unix(), 10), nil)
	if err := compareQueryResults([]nup.Song{}, t.GetSongsForAndroid(now), false, nup.AndroidClient); err != nil {
		tt.Error(err)
	}
}

// TODO: covers
