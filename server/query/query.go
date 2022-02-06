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
	maxResults  = 100  // max songs to return for query
	shuffleSkew = 0.25 // max offset to skew songs' positions when shuffling
)

// SongQuery describes a query returning a list of Songs.
type SongQuery struct {
	Artist  string // Song.Artist
	Title   string // Song.Title
	Album   string // Song.Album
	AlbumID string // Song.AlbumID

	Keywords []string // Song.Keywords

	MinRating    float64 // Song.Rating
	HasMinRating bool    // MinRating is set
	Unrated      bool    // Song.Rating is -1

	MaxPlays    int64 // Song.NumPlays
	HasMaxPlays bool  // MaxPlays is set

	MinFirstStartTime time.Time // Song.FirstStartTime
	MaxLastStartTime  time.Time // Song.LastStartTime

	Track int64 // Song.Track
	Disc  int64 // Song.Disc (may be 0 or 1 for single-disc albums)

	MaxDisc    int64 // Song.Disc
	HasMaxDisc bool  // MaxDisc is set

	Tags    []string // present in Song.Tags
	NotTags []string // not present in Song.Tags

	Shuffle              bool // randomize results set/order
	OrderByLastStartTime bool // order by Song.LastStartTime
}

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
	return !q.HasMaxPlays && q.MinFirstStartTime.IsZero() && q.MaxLastStartTime.IsZero() &&
		q.OrderByLastStartTime == false
}

// resultsInvalidated returns true if the updates described by ut would
// invalidate q's cached results.
func (q *SongQuery) resultsInvalidated(ut UpdateTypes) bool {
	if (ut & MetadataUpdate) != 0 {
		return true
	}
	if (ut&RatingUpdate) != 0 && (q.HasMinRating || q.Unrated) {
		return true
	}
	if (ut&TagsUpdate) != 0 && (len(q.Tags) > 0 || len(q.NotTags) > 0) {
		return true
	}
	if (ut&PlaysUpdate) != 0 &&
		(q.HasMaxPlays || !q.MinFirstStartTime.IsZero() || !q.MaxLastStartTime.IsZero() ||
			q.OrderByLastStartTime) {
		return true
	}
	return false
}

// UpdateTypes is a bitfield describing what was changed by an update.
// It is used for invalidating cached data.
type UpdateTypes uint8

const (
	MetadataUpdate UpdateTypes = 1 << iota // song metadata
	RatingUpdate
	TagsUpdate
	PlaysUpdate
)

// Songs executes the supplied query and returns matching songs.
// If cacheOnly is true, empty results are returned if the query's results
// aren't cached.
func Songs(ctx context.Context, query *SongQuery, cacheOnly bool) ([]*db.Song, error) {
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
	if ids == nil && cacheOnly {
		ids = make([]int64, 0)
		cacheWriteTypes = nil // don't write empty results to cache
	}

	// If we still don't have results, actually run the query against datastore.
	if ids == nil {
		if ids, err = runQuery(ctx, query); err != nil {
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

// filterSortedIDs returns values present in a but not in b (i.e. the intersection of a and !b).
// Both arrays must be sorted.
func filterSortedIDs(a, b []int64) []int64 {
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

func runQuery(ctx context.Context, query *SongQuery) ([]int64, error) {
	// First, build a base query with all of the equality filters.
	bq := datastore.NewQuery(db.SongKind).KeysOnly()

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
				bq = bq.Filter(t.expr, norm)
			}
		}
	}

	if len(query.AlbumID) > 0 {
		bq = bq.Filter("AlbumId =", query.AlbumID)
	}
	if query.HasMinRating {
		switch query.MinRating {
		case 1.0:
			bq = bq.Filter("Rating =", 1.0)
		case 0.75:
			bq = bq.Filter("RatingAtLeast75 =", true)
		case 0.5:
			bq = bq.Filter("RatingAtLeast50 =", true)
		case 0.25:
			bq = bq.Filter("RatingAtLeast25 =", true)
		case 0.0:
			bq = bq.Filter("RatingAtLeast0 =", true)
		default:
			return nil, fmt.Errorf("rating %v not in [1, 0.75, 0.5, 0.25, 0]", query.MinRating)
		}
	} else if query.Unrated {
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

	// This is incompatible with any of the inequality filters below.
	if query.OrderByLastStartTime {
		bq = bq.Order("LastStartTime").Limit(maxResults)
	}

	// Datastore doesn't allow multiple inequality filters on different properties.
	// Run a separate query in parallel for each filter and then intersect the results.
	var qs []*datastore.Query
	if query.HasMaxPlays {
		qs = append(qs, bq.Filter("NumPlays <=", query.MaxPlays))
	}
	if query.HasMaxDisc {
		qs = append(qs, bq.Filter("Disc <=", query.MaxDisc))
	}
	if !query.MinFirstStartTime.IsZero() {
		qs = append(qs, bq.Filter("FirstStartTime >=", query.MinFirstStartTime))
	}
	if !query.MaxLastStartTime.IsZero() {
		qs = append(qs, bq.Filter("LastStartTime <=", query.MaxLastStartTime))
	}

	if len(qs) == 0 {
		qs = append(qs, bq)
	}

	// Also run queries for tags that shouldn't be present and subtract the results.
	negativeQueryStart := len(qs)
	for _, t := range query.NotTags {
		qs = append(qs, bq.Filter("Tags =", t))
	}

	// If we're not shuffling the results, don't waste time getting IDs for songs that
	// we won't return. I'm only doing this if there's a single query, since there's no
	// guarantee about which rows will be returned, and intersections and subtractions
	// won't work right without full results.
	if len(qs) == 1 && !query.Shuffle && !query.OrderByLastStartTime {
		qs[0] = qs[0].Limit(maxResults)
	}

	startTime := time.Now()
	unmergedIDs, times, err := runQueriesAndGetIDs(ctx, qs)
	if err != nil {
		return nil, err
	}
	details := make([]string, len(unmergedIDs))
	for i, ids := range unmergedIDs {
		details[i] = fmt.Sprintf("%v (%v ms)", len(ids), times[i].Milliseconds())
	}
	log.Debugf(ctx, "Ran %v query(s) in %v ms: %v",
		len(qs), msecSince(startTime), strings.Join(details, ", "))

	startTime = time.Now()
	var mergedIDs []int64
	for i, ids := range unmergedIDs {
		if i == 0 {
			mergedIDs = ids
		} else if i < negativeQueryStart {
			mergedIDs = intersectSortedIDs(mergedIDs, ids)
		} else {
			mergedIDs = filterSortedIDs(mergedIDs, ids)
		}
	}
	log.Debugf(ctx, "Merged to %d result(s) in %v ms", len(mergedIDs), msecSince(startTime))
	return mergedIDs, nil
}

// CleanSong prepares s to be returned in results.
// This is exported so it can be called by tests.
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

// sortSongs sorts songs appropriately for the client.
func sortSongs(songs []*db.Song) {
	sort.Slice(songs, func(i, j int) bool {
		si, sj := songs[i], songs[j]
		if si.AlbumLower < sj.AlbumLower {
			return true
		} else if si.AlbumLower > sj.AlbumLower {
			return false
		}
		if si.AlbumID < sj.AlbumID {
			return true
		} else if si.AlbumID > sj.AlbumID {
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

	shuf(songs, func(s *db.Song) string { return s.ArtistLower },
		func(s *db.Song) string { return s.AlbumLower })
}

// msecSince returns the number of elapsed milliseconds since t.
func msecSince(t time.Time) int64 {
	return time.Now().Sub(t).Nanoseconds() / int64(time.Millisecond/time.Nanosecond)
}
