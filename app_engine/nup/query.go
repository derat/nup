package appengine

import (
	"appengine"
	"appengine/datastore"
	"math/rand"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"erat.org/cloud"
	"erat.org/nup"
)

const (
	// Maximum number of results to return for a search.
	maxQueryResults = 100
)

type songQuery struct {
	Artist, Title, Album string

	Keywords []string

	MinRating    float64
	HasMinRating bool
	Unrated      bool

	MaxPlays    int64
	HasMaxPlays bool

	MinFirstStartTime time.Time
	MaxLastStartTime  time.Time

	Track, Disc int64

	Tags    []string
	NotTags []string

	Shuffle bool
}

// From https://groups.google.com/forum/#!topic/golang-nuts/tyDC4S62nPo.
type int64Array []int64

func (a int64Array) Len() int           { return len(a) }
func (a int64Array) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64Array) Less(i, j int) bool { return a[i] < a[j] }

type songArray []nup.Song

func (a songArray) Len() int      { return len(a) }
func (a songArray) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a songArray) Less(i, j int) bool {
	if a[i].Album < a[j].Album {
		return true
	} else if a[i].Album > a[j].Album {
		return false
	}
	if a[i].Disc < a[j].Disc {
		return true
	} else if a[i].Disc > a[j].Disc {
		return false
	}
	return a[i].Track < a[j].Track
}

// encodePathForCloudStorage converts the passed-in original Unix filename to the appropriate path for accessing the file via Cloud Storage.
// This includes:
// - the initial escaping performed by the cloud_sync program (a subset of query escaping),
// - regular query escaping, and
// - replacing "+" with "%20" because Cloud Storage seems unhappy otherwise.
func encodePathForCloudStorage(p string) string {
	return strings.Replace(url.QueryEscape(cloud.EscapeObjectName(p)), "+", "%20", -1)
}

func prepareSongForSearchResult(s *nup.Song, id int64, baseSongUrl, baseCoverUrl string) {
	// Set fields that are only present in search results (i.e. not in Datastore).
	s.SongId = strconv.FormatInt(id, 10)
	if len(s.Filename) > 0 {
		s.Url = baseSongUrl + encodePathForCloudStorage(s.Filename)
	}
	if len(s.CoverFilename) > 0 {
		s.CoverUrl = baseCoverUrl + encodePathForCloudStorage(s.CoverFilename)
	}

	// Create an empty tags slice so that clients don't need to check for null.
	if s.Tags == nil {
		s.Tags = make([]string, 0)
	}

	// Clear fields that are passed for updates (and hence not excluded from JSON)
	// but that aren't needed in search results.
	s.Sha1 = ""
	s.Filename = ""
	s.CoverFilename = ""
	s.Plays = s.Plays[:0]
}

func getTags(c appengine.Context) ([]string, error) {
	tags := make(map[string]bool)
	it := datastore.NewQuery(songKind).Project("Tags").Distinct().Run(c)
	for {
		song := &nup.Song{}
		if _, err := it.Next(song); err == nil {
			for _, t := range song.Tags {
				tags[t] = true
			}
		} else if err == datastore.Done {
			break
		} else {
			return nil, err
		}
	}

	res := make([]string, len(tags))
	i := 0
	for t := range tags {
		res[i] = t
		i++
	}
	return res, nil
}

func runQueriesAndGetIds(c appengine.Context, qs []*datastore.Query) ([][]int64, error) {
	type idsAndError struct {
		ids []int64
		err error
	}
	ch := make(chan idsAndError)

	for _, q := range qs {
		go func(q *datastore.Query) {
			ids := make([]int64, 0)
			it := q.Run(c)
			for {
				if k, err := it.Next(nil); err == nil {
					ids = append(ids, k.IntID())
				} else if err == datastore.Done {
					break
				} else {
					ch <- idsAndError{nil, err}
					return
				}
			}
			sort.Sort(int64Array(ids))
			ch <- idsAndError{ids, nil}
		}(q)
	}

	res := make([][]int64, len(qs))
	for i := range qs {
		iae := <-ch
		if iae.err != nil {
			return nil, iae.err
		}
		res[i] = iae.ids
	}
	return res, nil
}

// intersectSortedIds returns the intersection of two sorted arrays that don't have duplicate values.
func intersectSortedIds(a, b []int64) []int64 {
	m := make([]int64, 0)
	var i, j int
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			m = append(m, a[i])
			i++
			j++
		} else if a[i] < b[j] {
			i++
		} else {
			j++
		}
	}
	return m
}

// filterSortedIds returns values present in a but not in b (i.e. the intersection of a and !b).
// Both arrays must be sorted.
func filterSortedIds(a, b []int64) []int64 {
	m := make([]int64, 0)
	var i, j int
	for i < len(a) {
		for j < len(b) && a[i] > b[j] {
			j++
		}
		if j >= len(b) || a[i] != b[j] {
			m = append(m, a[i])
		}
		i++
	}
	return m
}

// shufflePartial randomly swaps into the first n positions using elements from the entire slice.
func shufflePartial(a []int64, n int) {
	for i := 0; i < n; i++ {
		j := i + rand.Intn(len(a)-i)
		a[i], a[j] = a[j], a[i]
	}
}

func doQuery(c appengine.Context, query *songQuery, baseSongUrl, baseCoverUrl string) ([]nup.Song, error) {
	// First, build a base query with all of the equality filters.
	bq := datastore.NewQuery(songKind).KeysOnly()
	if len(query.Artist) > 0 {
		bq = bq.Filter("ArtistLower =", strings.ToLower(query.Artist))
	}
	if len(query.Title) > 0 {
		bq = bq.Filter("TitleLower =", strings.ToLower(query.Title))
	}
	if len(query.Album) > 0 {
		bq = bq.Filter("AlbumLower =", strings.ToLower(query.Album))
	}
	for _, w := range query.Keywords {
		bq = bq.Filter("Keywords =", strings.ToLower(w))
	}
	if query.Unrated && !query.HasMinRating {
		bq = bq.Filter("Rating =", -1.0)
	}
	if query.Track > 0 {
		bq = bq.Filter("Track =", query.Track)
	}
	if query.Disc > 0 {
		bq = bq.Filter("Disc =", query.Disc)
	}
	for _, t := range query.Tags {
		bq = bq.Filter("Tags =", t)
	}

	// Datastore doesn't allow multiple inequality filters on different properties.
	// Run a separate query in parallel for each filter and then merge the results.
	qs := make([]*datastore.Query, 0)
	if query.HasMinRating {
		qs = append(qs, bq.Filter("Rating >=", query.MinRating))
	}
	if query.HasMaxPlays {
		qs = append(qs, bq.Filter("NumPlays <=", query.MaxPlays))
	}
	if !query.MinFirstStartTime.IsZero() {
		qs = append(qs, bq.Filter("FirstStartTime >=", query.MinFirstStartTime))
	}
	if !query.MaxLastStartTime.IsZero() {
		qs = append(qs, bq.Filter("LastStartTime <=", query.MaxLastStartTime))
	}
	if len(qs) == 0 {
		qs = []*datastore.Query{bq}
	}

	// Also run queries for tags that shouldn't be present.
	negativeQueryStart := len(qs)
	for _, t := range query.NotTags {
		qs = append(qs, bq.Filter("Tags =", t))
	}

	startTime := time.Now()
	unmergedIds, err := runQueriesAndGetIds(c, qs)
	if err != nil {
		return nil, err
	}
	c.Debugf("Ran %v query(s) in %v ms", len(qs), time.Now().Sub(startTime).Nanoseconds()/(1000*1000))

	var mergedIds []int64
	for i, a := range unmergedIds {
		if i == 0 {
			mergedIds = a
		} else if i < negativeQueryStart {
			mergedIds = intersectSortedIds(mergedIds, a)
		} else {
			mergedIds = filterSortedIds(mergedIds, a)
		}
	}

	// Oh, for generics...
	numResults := len(mergedIds)
	if numResults > maxQueryResults {
		numResults = maxQueryResults
	}

	if query.Shuffle {
		shufflePartial(mergedIds, numResults)
	}

	startTime = time.Now()
	keys := make([]*datastore.Key, numResults)
	for i, id := range mergedIds[:numResults] {
		keys[i] = datastore.NewKey(c, songKind, "", id, nil)
	}
	songs := make([]nup.Song, numResults)
	if err = datastore.GetMulti(c, keys, songs); err != nil {
		return nil, err
	}
	c.Debugf("Fetched %v song(s) in %v ms", len(songs), time.Now().Sub(startTime).Nanoseconds()/(1000*1000))

	for i := range songs {
		prepareSongForSearchResult(&songs[i], keys[i].IntID(), baseSongUrl, baseCoverUrl)
	}
	if !query.Shuffle {
		sort.Sort(songArray(songs))
	}
	return songs, nil
}
