package appengine

import (
	"appengine"
	"appengine/datastore"
	"math/rand"
	"sort"
	"strings"
	"time"

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
	type queryResult struct {
		Index int
		Ids   []int64
		Error error
	}
	ch := make(chan queryResult)

	for i, q := range qs {
		go func(index int, q *datastore.Query) {
			ids := make([]int64, 0)
			it := q.Run(c)
			for {
				if k, err := it.Next(nil); err == nil {
					ids = append(ids, k.IntID())
				} else if err == datastore.Done {
					break
				} else {
					ch <- queryResult{index, nil, err}
					return
				}
			}
			sort.Sort(int64Array(ids))
			ch <- queryResult{index, ids, nil}
		}(i, q)
	}

	res := make([][]int64, len(qs))
	for _ = range qs {
		qr := <-ch
		if qr.Error != nil {
			return nil, qr.Error
		}
		res[qr.Index] = qr.Ids
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

func getSongsForQuery(c appengine.Context, query *songQuery) ([]nup.Song, error) {
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
	c.Debugf("Ran %v query(s) in %v ms", len(qs), getMsecSinceTime(startTime))

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

	ids := mergedIds[:numResults]
	songs := make([]nup.Song, numResults)

	cachedSongs := make(map[int64]nup.Song)
	storedSongs := make([]nup.Song, 0)

	// Get whatever we can from memcache.
	if numResults > 0 {
		startTime = time.Now()
		if hits, err := getSongsFromCache(c, ids); err != nil {
			c.Errorf("Got error while querying cache: %v", err)
		} else {
			c.Debugf("Got %v of %v song(s) from cache in %v ms", len(hits), len(ids), getMsecSinceTime(startTime))
			cachedSongs = hits
		}
	}

	// Get the remaining songs from datastore and write them back to memcache.
	if len(cachedSongs) < numResults {
		startTime = time.Now()
		numStored := numResults - len(cachedSongs)
		storedIds := make([]int64, 0, numStored)
		keys := make([]*datastore.Key, 0, numStored)
		for _, id := range ids {
			if _, ok := cachedSongs[id]; !ok {
				storedIds = append(storedIds, id)
				keys = append(keys, datastore.NewKey(c, songKind, "", id, nil))
			}
		}
		storedSongs = make([]nup.Song, len(keys))
		if err = datastore.GetMulti(c, keys, storedSongs); err != nil {
			return nil, err
		}
		c.Debugf("Fetched %v song(s) from datastore in %v ms", len(storedSongs), getMsecSinceTime(startTime))

		startTime = time.Now()
		if err := writeSongsToCache(c, storedIds, storedSongs, false); err != nil {
			c.Errorf("Failed to write just-fetched song(s) to cache: %v", err)
		} else {
			c.Debugf("Wrote %v song(s) to cache in %v ms", len(storedSongs), getMsecSinceTime(startTime))
		}
	}

	storedIndex := 0
	for i, id := range ids {
		if s, ok := cachedSongs[id]; ok {
			songs[i] = s
		} else {
			songs[i] = storedSongs[storedIndex]
			storedIndex++
		}
		prepareSongForClient(&songs[i], id, webClient)
	}

	if !query.Shuffle {
		sort.Sort(songArray(songs))
	}
	return songs, nil
}
