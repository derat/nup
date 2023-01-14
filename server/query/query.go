// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package query loads songs and tags from datastore.
package query

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/derat/nup/server/cache"
	"github.com/derat/nup/server/db"

	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/log"
)

const (
	maxResults        = 100  // max songs to return for query
	truncateMapThresh = 1000 // max song ID map size for truncateIDsByLastStartTime
	shuffleSkew       = 0.25 // max offset to skew songs' positions when shuffling
)

// SongQuery describes a query returning a list of Songs.
type SongQuery struct {
	Artist   string // Song.Artist
	Title    string // Song.Title
	Album    string // Song.Album
	AlbumID  string // Song.AlbumID
	Filename string // song.Filename

	Keywords []string // Song.Keywords

	Rating    int  // Song.Rating (0 if unspecified; use Unrated for 0)
	MinRating int  // Song.Rating (0 if unspecified)
	MaxRating int  // Song.Rating (0 if unspecified)
	Unrated   bool // Song.Rating is 0

	MaxPlays int64 // Song.NumPlays (-1 if unspecified)

	MinFirstStartTime time.Time // Song.FirstStartTime
	MaxLastStartTime  time.Time // Song.LastStartTime

	Track int64 // Song.Track
	Disc  int64 // Song.Disc

	MinDate time.Time // Song.Date
	MaxDate time.Time // Song.Date

	Tags    []string // present in Song.Tags
	NotTags []string // not present in Song.Tags

	Shuffle              bool // randomize results set/order
	OrderByLastStartTime bool // order by Song.LastStartTime
}

func (q *SongQuery) hasMaxPlays() bool { return q.MaxPlays >= 0 }

// hash returns a string uniquely identifying q.
func (q *SongQuery) hash() (string, error) {
	b, err := json.Marshal(q)
	if err != nil {
		return "", err
	}
	s := sha1.Sum(b)
	return hex.EncodeToString(s[:]), nil
}

// canCache returns true if the query's results can be safely cached.
func (q *SongQuery) canCache() bool {
	return !q.hasMaxPlays() && q.MinFirstStartTime.IsZero() && q.MaxLastStartTime.IsZero() &&
		!q.OrderByLastStartTime
}

// resultsInvalidated returns true if the updates described by ut would
// invalidate q's cached results.
func (q *SongQuery) resultsInvalidated(ut UpdateTypes) bool {
	if (ut & MetadataUpdate) != 0 {
		return true
	}
	if (ut&RatingUpdate) != 0 && (q.Rating != 0 || q.MinRating != 0 || q.MaxRating != 0 || q.Unrated) {
		return true
	}
	if (ut&TagsUpdate) != 0 && (len(q.Tags) > 0 || len(q.NotTags) > 0) {
		return true
	}
	if (ut&PlaysUpdate) != 0 &&
		(q.hasMaxPlays() || !q.MinFirstStartTime.IsZero() || !q.MaxLastStartTime.IsZero() ||
			q.OrderByLastStartTime) {
		return true
	}
	return false
}

// UpdateTypes is a bitfield describing what was changed by an update.
// It is used for invalidating cached data.
type UpdateTypes uint32

const (
	MetadataUpdate UpdateTypes = 1 << iota // song metadata
	RatingUpdate
	TagsUpdate
	PlaysUpdate
)

// SongsFlags is a bitfield controlling the behavior of the Songs function.
type SongsFlags uint32

const (
	// CacheOnly indicates that empty results should be returned if the query's results aren't
	// already cached.
	CacheOnly SongsFlags = 1 << iota
	// ForceFallback indicates that the fallback mode that tries to avoid requiring composite
	// indexes should be used instead of the normal mode.
	ForceFallback
	// NoFallback indicates that the fallback mode should never be used.
	NoFallback
)

// Songs executes the supplied query and returns matching songs.
func Songs(ctx context.Context, query *SongQuery, flags SongsFlags) ([]*db.Song, error) {
	var ids []int64
	var err error

	// Check memcache first and then datastore.
	var cacheWriteTypes []cache.Type // caches to write to
	for _, t := range []cache.Type{cache.Memcache, cache.Datastore} {
		startTime := time.Now()
		if ids, err = getCachedResults(ctx, query, t); err != nil {
			log.Errorf(ctx, "Got error while getting cached results from %v: %v", t, err)
		} else if ids == nil {
			log.Debugf(ctx, "Cache miss from %v took %v ms", t, msecSince(startTime))
			cacheWriteTypes = append(cacheWriteTypes, t)
		} else {
			log.Debugf(ctx, "Got %v cached result(s) from %v in %v ms", len(ids), t, msecSince(startTime))
			break
		}
	}

	// If we were asked to only return cached results, create an empty result set.
	if ids == nil && flags&CacheOnly != 0 {
		ids = make([]int64, 0)
		cacheWriteTypes = nil // don't write empty results to cache
	}

	// If we still don't have results, actually run the query against datastore.
	if ids == nil {
		forceFallback := flags&ForceFallback != 0
		noFallback := flags&NoFallback != 0
		if ids, err = runQuery(ctx, query, forceFallback); err != nil {
			// Error code 4 corresponds to "NEED_INDEX":
			// https://github.com/golang/appengine/blob/8f83b321/internal/datastore/datastore_v3.proto#L351
			if code, ok := getErrorCode(err); ok && code == 4 && !forceFallback && !noFallback {
				log.Debugf(ctx, "Rerunning query due to missing composite index")
				ids, err = runQuery(ctx, query, true)
			}
		}
		if err != nil {
			return nil, err
		}
	}

	// Asynchronously cache the results.
	if query.canCache() && len(cacheWriteTypes) > 0 {
		cacheWriteDone := make(chan struct{}, len(cacheWriteTypes))
		for _, t := range cacheWriteTypes {
			go func(t cache.Type, ids []int64) {
				startTime := time.Now()
				if err = setCachedResults(ctx, query, ids, t); err != nil {
					log.Errorf(ctx, "Got error while caching results to %v: %v", t, err)
				} else {
					log.Debugf(ctx, "Cached results to %v in %v ms", t, msecSince(startTime))
				}
				cacheWriteDone <- struct{}{}
			}(t, append([]int64{}, ids...)) // duplicate since mutated in main body
		}

		// Wait for async cache writes to finish before returning. Otherwise, App Engine will cancel
		// the writes when the context is canceled.
		// TODO: Will App Engine send the response before the handler has returned? If so,
		// it'd probably be faster to return this function so the caller can defer it instead.
		defer func() {
			startTime := time.Now()
			for range cacheWriteTypes {
				<-cacheWriteDone
			}
			log.Debugf(ctx, "Waited %v ms for %v cache write(s)", msecSince(startTime), len(cacheWriteTypes))
		}()
	}

	if len(ids) == 0 {
		return []*db.Song{}, nil // ugly: can't return nil slice since it messes up JSON response
	}

	// Shuffle and truncate the results if needed.
	numResults := len(ids)
	if numResults > maxResults {
		numResults = maxResults
	}
	if query.Shuffle {
		shufflePartial(ids, numResults)
	}
	ids = ids[:numResults]

	// Get the songs from datastore.
	startTime := time.Now()
	songs := make([]*db.Song, numResults)
	keys := make([]*datastore.Key, 0, len(songs))
	for _, id := range ids {
		keys = append(keys, datastore.NewKey(ctx, db.SongKind, "", id, nil))
	}
	if err = datastore.GetMulti(ctx, keys, songs); err != nil {
		return nil, err
	}
	log.Debugf(ctx, "Fetched %v song(s) from datastore in %v ms", len(songs), msecSince(startTime))

	// Prepare the results for the client.
	for i, id := range ids {
		CleanSong(songs[i], id)
	}
	if query.Shuffle {
		spreadSongs(songs)
	} else if query.OrderByLastStartTime {
		sort.Slice(songs, func(i, j int) bool { return songs[i].LastStartTime.Before(songs[j].LastStartTime) })
	} else {
		sortSongs(songs)
	}

	return songs, nil
}

// runQueriesAndGetIDs runs the provided queries in parallel and returns the results from each.
// Each result set (consisting of key integer IDs) is sorted in ascending order.
func runQueriesAndGetIDs(ctx context.Context, qs []*datastore.Query) ([][]int64, []time.Duration, error) {
	type queryResult struct {
		idx  int
		ids  []int64
		time time.Duration
		err  error
	}
	ch := make(chan queryResult, len(qs))

	for i, q := range qs {
		go func(idx int, q *datastore.Query) {
			start := time.Now()
			ids := make([]int64, 0)
			it := q.Run(ctx)
			for {
				if k, err := it.Next(nil); err == nil {
					ids = append(ids, k.IntID())
				} else if err == datastore.Done {
					break
				} else {
					ch <- queryResult{idx, nil, 0, err}
					return
				}
			}
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			ch <- queryResult{idx, ids, time.Now().Sub(start), nil}
		}(i, q)
	}

	res := make([][]int64, len(qs))
	times := make([]time.Duration, len(qs))
	for _ = range qs {
		qr := <-ch
		if qr.err != nil {
			return nil, nil, qr.err
		}
		res[qr.idx] = qr.ids
		times[qr.idx] = qr.time
	}
	return res, times, nil
}

// intersectSortedIDs returns the intersection of two sorted arrays that don't have duplicate values.
func intersectSortedIDs(a, b []int64) []int64 {
	ml := len(a)
	if len(b) < ml {
		ml = len(b)
	}
	m := make([]int64, 0, ml)

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

// subtractSortedIDs returns values present in a but not in b (i.e. the intersection of a and !b).
// Both arrays must be sorted.
func subtractSortedIDs(a, b []int64) []int64 {
	m := make([]int64, 0, len(a))
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

// runQuery performs the supplied query against datastore and returns the corresponding songs'
// integer IDs in an unspecified order. The results are not necessarily truncated to maxResults
// songs yet (since e.g. the full result set is needed when shuffling).
//
// If fallback is true, each inequality filter is executed in its own query. This is slow (since
// some queries may match all rows), but it will hopefully work even if an appropriate composite
// index isn't present: https://cloud.google.com/datastore/docs/concepts/indexes
func runQuery(ctx context.Context, query *SongQuery, fallback bool) ([]int64, error) {
	// First, build a base query with all of the equality filters.
	eq := datastore.NewQuery(db.SongKind).KeysOnly()

	type term struct{ expr, val string }
	terms := []term{
		{"ArtistLower =", query.Artist},
		{"TitleLower =", query.Title},
		{"AlbumLower =", query.Album},
	}
	for _, w := range query.Keywords {
		terms = append(terms, term{"Keywords =", w})
	}
	for _, t := range terms {
		if t.val != "" {
			if norm, err := db.Normalize(t.val); err != nil {
				return nil, fmt.Errorf("normalizing %q: %v", t.val, err)
			} else {
				eq = eq.Filter(t.expr, norm)
			}
		}
	}

	if query.AlbumID != "" {
		eq = eq.Filter("AlbumId =", query.AlbumID)
	}
	if query.Filename != "" {
		eq = eq.Filter("Filename =", query.Filename)
	}

	if query.Rating != 0 {
		if query.Rating >= 1 && query.Rating <= 5 {
			eq = eq.Filter("Rating =", query.Rating)
		} else {
			return nil, fmt.Errorf("rating %v not in [1, 5]", query.MaxRating)
		}
	} else if query.MinRating != 0 {
		switch query.MinRating {
		case 1:
			eq = eq.Filter("RatingAtLeast1 =", true)
		case 2:
			eq = eq.Filter("RatingAtLeast2 =", true)
		case 3:
			eq = eq.Filter("RatingAtLeast3 =", true)
		case 4:
			eq = eq.Filter("RatingAtLeast4 =", true)
		case 5:
			eq = eq.Filter("Rating =", 5)
		default:
			return nil, fmt.Errorf("min rating %v not in [1, 5]", query.MinRating)
		}
	} else if query.MaxRating != 0 {
		switch query.MaxRating {
		case 1:
			eq = eq.Filter("RatingAtLeast2 =", false)
		case 2:
			eq = eq.Filter("RatingAtLeast3 =", false)
		case 3:
			eq = eq.Filter("RatingAtLeast4 =", false)
		case 4:
			// handled later as "Rating < 5" inequality filter
		case 5:
			// no-op
		default:
			return nil, fmt.Errorf("max rating %v not in [1, 5]", query.MaxRating)
		}
	} else if query.Unrated {
		eq = eq.Filter("Rating =", 0)
	}

	if query.MaxPlays == 0 {
		eq = eq.Filter("NumPlays =", 0)
	}
	if query.Track > 0 {
		eq = eq.Filter("Track =", query.Track)
	}
	if query.Disc > 0 {
		eq = eq.Filter("Disc =", query.Disc)
	}
	for _, t := range query.Tags {
		eq = eq.Filter("Tags =", t)
	}

	var qs []*datastore.Query // underlying queries to run in parallel

	// Now add inequality filters. Datastore doesn't allow multiple inequality filters on different
	// properties, so we run a separate query in parallel for each filter and then intersect the
	// results.
	var iq *datastore.Query // base query
	if fallback {
		// If we already determined that we don't have the proper composite index needed to mix
		// equality and inequality filters, then run a separate slow query for each inequality
		// filter.
		qs = append(qs, eq)
		iq = datastore.NewQuery(db.SongKind).KeysOnly()
	} else {
		// Otherwise, include the equality filters with the inequality filters.
		iq = eq
	}

	if !query.MinDate.IsZero() || !query.MaxDate.IsZero() {
		dq := iq
		if !query.MinDate.IsZero() {
			dq = dq.Filter("Date >=", query.MinDate)
		}
		if !query.MaxDate.IsZero() {
			dq = dq.Filter("Date <=", query.MaxDate)
			if query.MinDate.IsZero() {
				dq = dq.Filter("Date >", time.Time{}) // exclude unset dates
			}
		}
		qs = append(qs, dq)
	}
	if query.MaxPlays >= 1 {
		qs = append(qs, iq.Filter("NumPlays <=", query.MaxPlays))
	}
	if !query.MinFirstStartTime.IsZero() {
		qs = append(qs, iq.Filter("FirstStartTime >=", query.MinFirstStartTime))
	}
	if !query.MaxLastStartTime.IsZero() {
		qs = append(qs, iq.Filter("LastStartTime <=", query.MaxLastStartTime))
	}
	if query.MaxRating == 4 {
		qs = append(qs, iq.Filter("Rating <", 5))
	}

	// If we don't have any queries that incorporate the equality filters and inequality filters,
	// just run a query with the equality filters by itself.
	if len(qs) == 0 {
		q := eq
		// Limit the number of the results if we know that we we won't need to intersect multiple
		// queries or shuffle a big result set.
		if query.OrderByLastStartTime {
			q = q.Order("LastStartTime").Limit(maxResults)
		} else if len(query.NotTags) == 0 && !query.Shuffle {
			q = q.Limit(maxResults)
		}
		qs = append(qs, q)
	}

	// Also run a query for each tag that shouldn't be present and subtract it from the results.
	negativeQueryStart := len(qs)
	for _, t := range query.NotTags {
		qs = append(qs, eq.Filter("Tags =", t))
	}

	start := time.Now()
	unmerged, times, err := runQueriesAndGetIDs(ctx, qs)
	if err != nil {
		return nil, err
	}
	details := make([]string, len(unmerged))
	for i, ids := range unmerged {
		details[i] = fmt.Sprintf("%v (%v ms)", len(ids), times[i].Milliseconds())
	}
	log.Debugf(ctx, "Ran %v query(s) in %v ms: %v",
		len(qs), msecSince(start), strings.Join(details, ", "))

	// Intersect and subtract the queries to get a single ordered result set.
	merged := unmerged[0]
	if len(unmerged) > 1 {
		start := time.Now()
		for i, ids := range unmerged[1:] {
			if i+1 < negativeQueryStart {
				merged = intersectSortedIDs(merged, ids)
			} else {
				merged = subtractSortedIDs(merged, ids)
			}
		}
		log.Debugf(ctx, "Merged to %d result(s) in %v ms", len(merged), msecSince(start))
	}

	// If we weren't able to use datastore to limit the number of results,
	// do another query to get the correct ordering so we can truncate.
	if query.OrderByLastStartTime && len(merged) > maxResults {
		start := time.Now()
		if merged, err = truncateIDsByLastStartTime(ctx, merged); err != nil {
			return nil, err
		}
		log.Debugf(ctx, "Truncated by last start time to %d result(s) in %v ms",
			len(merged), msecSince(start))
	}

	return merged, nil
}

// truncateIDsByLastStartTime returns the first maxResults (at most) of the supplied sorted
// IDs after ordering by LastStartTime.
func truncateIDsByLastStartTime(ctx context.Context, ids []int64) ([]int64, error) {
	// If we don't have many IDs, we'll probably need to read a bunch of the LastStartTime
	// results before get to maxResults songs. Put the IDs into a map so we don't need to
	// binary search over and over.
	var check func(int64) bool
	if len(ids) <= truncateMapThresh {
		idsMap := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			idsMap[id] = struct{}{}
		}
		check = func(id int64) bool {
			_, ok := idsMap[id]
			return ok
		}
	} else {
		check = func(id int64) bool {
			i := sort.Search(len(ids), func(i int) bool { return ids[i] >= id })
			return i < len(ids) && ids[i] == id
		}
	}

	// This query matches all entities, but its running time fortunately appears to depend on the
	// number of results that we read (which depends on how soon we encounter maxResults of the
	// passed-in songs).
	res := make([]int64, 0, maxResults)
	it := datastore.NewQuery(db.SongKind).KeysOnly().Order("LastStartTime").Run(ctx)
	for len(res) < maxResults {
		if k, err := it.Next(nil); err == datastore.Done {
			break
		} else if err != nil {
			return nil, err
		} else {
			id := k.IntID()
			if check(id) {
				res = append(res, id)
			}
		}
	}
	return res, nil
}

// CleanSong prepares s to be returned in results.
// This is exported so it can be called by tests in other packages.
func CleanSong(s *db.Song, id int64) {
	s.SongID = strconv.FormatInt(id, 10)

	// Create an empty tags slice so that clients don't need to check for null.
	if s.Tags == nil {
		s.Tags = make([]string, 0)
	}

	// Clear fields that are passed for updates (and hence not excluded from JSON)
	// but that aren't needed in search results.
	s.SHA1 = ""
	s.Plays = s.Plays[:0]
}

// sortSongs sorts songs for the client.
// Songs are sorted by album artist, album release date, album name,
// and finally by disc and track.
func sortSongs(songs []*db.Song) {
	albumIDArtists := make(map[string]string)
	albumIDDates := make(map[string]string)
	for _, s := range songs {
		if s.AlbumID == "" {
			continue
		}
		if _, ok := albumIDArtists[s.AlbumID]; !ok {
			if s.AlbumArtist != "" {
				albumIDArtists[s.AlbumID] = s.AlbumArtist
			} else {
				albumIDArtists[s.AlbumID] = s.Artist
			}
		}
		if _, ok := albumIDDates[s.AlbumID]; !ok && !s.Date.IsZero() {
			albumIDDates[s.AlbumID] = s.Date.Format("20060102")
		}
	}

	cmp := func(a, b string) int {
		if a != "" && (b == "" || a < b) {
			return -1
		} else if b != "" && (a == "" || b < a) {
			return 1
		}
		return 0
	}

	sort.Slice(songs, func(i, j int) bool {
		si, sj := songs[i], songs[j]

		if res := cmp(albumIDArtists[si.AlbumID], albumIDArtists[sj.AlbumID]); res < 0 {
			return true
		} else if res > 0 {
			return false
		}
		if res := cmp(albumIDDates[si.AlbumID], albumIDDates[sj.AlbumID]); res < 0 {
			return true
		} else if res > 0 {
			return false
		}
		if res := cmp(si.AlbumLower, sj.AlbumLower); res < 0 {
			return true
		} else if res > 0 {
			return false
		}
		if si.Disc < sj.Disc {
			return true
		} else if si.Disc > sj.Disc {
			return false
		}
		return si.Track < sj.Track
	})
}

// spreadSongs reorders songs in-place to make it unlikely that songs by the same artist will appear
// close to each other or that an album will be repeated for a given artist.
//
// It assumes that the supplied slice has already been randomly shuffled (e.g. using Fisher-Yates).
//
// More discussion of the approach used here:
//  http://keyj.emphy.de/balanced-shuffle/
//  https://engineering.atspotify.com/2014/02/28/how-to-shuffle-songs/
func spreadSongs(songs []*db.Song) {
	type keyFunc func(s *db.Song) string // returns a key for grouping s
	var shuf func([]*db.Song, keyFunc, keyFunc)
	shuf = func(songs []*db.Song, outer, inner keyFunc) {
		// Group songs using the key function.
		groups := make(map[string][]*db.Song)
		for _, s := range songs {
			key := outer(s)
			groups[key] = append(groups[key], s)
		}

		// Spread out each group across the entire range.
		dists := make(map[*db.Song]float64, len(songs))
		for _, group := range groups {
			// Recursively spread out the songs within the group first if needed.
			if inner != nil {
				shuf(group, inner, nil)
			}
			// Apply a random offset at the beginning and then further skew each song's position.
			glen := float64(len(group))
			off := (1 - shuffleSkew) * rand.Float64()
			for i, s := range group {
				dists[s] = (off + float64(i) + shuffleSkew*rand.Float64()) / glen
			}
		}
		sort.Slice(songs, func(i, j int) bool { return dists[songs[i]] < dists[songs[j]] })
	}

	shuf(songs, func(s *db.Song) string {
		// Try to group songs by by "Foo" and "Foo feat. Bar" together: if Artist is prefixed
		// by AlbumArtist, just use the normalized (lowercased) version of AlbumArtist.
		if s.AlbumArtist != "" && strings.HasPrefix(s.Artist, s.AlbumArtist) {
			if n, err := db.Normalize(s.AlbumArtist); err == nil {
				return n
			}
		}
		return s.ArtistLower
	}, func(s *db.Song) string { return s.AlbumLower })
}

// msecSince returns the number of elapsed milliseconds since t.
func msecSince(t time.Time) int64 { return time.Since(t).Milliseconds() }

// getErrorCode attempts to extract an internal datastore error code from an error returned by the
// google.golang.org/appengine/v2/datastore package.
//
// Codes correspond to the ErrorCode enum:
// https://github.com/golang/appengine/blob/8f83b321/internal/datastore/datastore_v3.proto#L347
//
// It's really annoying that the package doesn't export a dedicated error for "NEED_INDEX".
func getErrorCode(err error) (code int, ok bool) {
	ev := reflect.Indirect(reflect.ValueOf(err))
	if ev.Kind() != reflect.Struct {
		return 0, false
	}
	fv := ev.FieldByName("Code")
	// TODO: Use CanInt() in Go 1.18 (field is currently int32).
	if k := fv.Kind(); k != reflect.Int && k != reflect.Int32 && k != reflect.Int64 {
		return 0, false
	}
	return int(fv.Int()), true
}
