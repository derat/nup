// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package query loads songs and tags from datastore.
package query

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/derat/nup/internal/pkg/types"
	"github.com/derat/nup/server/cache"
	"github.com/derat/nup/server/common"

	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

const maxResults = 100 // max songs to return for query

// Tags returns the full set of tags present across all songs.
// It attempts to return cached data before falling back to scanning all songs.
// If songs are scanned, the resulting tags are cached.
// If requireCache is true, an error is returned if tags aren't cached.
func Tags(ctx context.Context, requireCache bool) ([]string, error) {
	var tags []string
	var err error

	// Check memcache first and then datastore.
	var cacheWriteTypes []cache.Type // caches to write to
	for _, t := range []cache.Type{cache.Memcache, cache.Datastore} {
		startTime := time.Now()
		if tags, err = cache.GetTags(ctx, t); err != nil {
			log.Errorf(ctx, "Got error while getting cached tags from %v: %v", t, err)
		} else if tags == nil {
			log.Debugf(ctx, "Cache miss from %v took %v ms", t, msecSince(startTime))
			cacheWriteTypes = append(cacheWriteTypes, t)
		} else {
			log.Debugf(ctx, "Got %v cached tag(s) from %v in %v ms", len(tags), t, msecSince(startTime))
			break
		}
	}
	if tags == nil && requireCache {
		return nil, errors.New("tags not cached")
	}

	// If tags weren't cached, fall back to running a slow query across all songs.
	if tags == nil {
		startTime := time.Now()
		tagMap := make(map[string]struct{})
		it := datastore.NewQuery(common.SongKind).Project("Tags").Distinct().Run(ctx)
		for {
			var song types.Song
			if _, err := it.Next(&song); err == datastore.Done {
				break
			} else if err != nil {
				return nil, err
			}
			for _, t := range song.Tags {
				tagMap[t] = struct{}{}
			}
		}
		tags = make([]string, len(tagMap))
		i := 0
		for t := range tagMap {
			tags[i] = t
			i++
		}
		sort.Strings(tags)
		log.Debugf(ctx, "Queried %v tag(s) from datastore in %v ms", len(tags), msecSince(startTime))
	}

	// Write the tags to any caches that didn't have them already.
	// These writes can be slow and will block the HTTP response, but callers
	// should be getting tags asynchronously anyway.
	if len(cacheWriteTypes) > 0 {
		cacheWriteDone := make(chan struct{}, len(cacheWriteTypes))
		for _, t := range cacheWriteTypes {
			go func(t cache.Type) {
				startTime := time.Now()
				if err := cache.SetTags(ctx, tags, t); err != nil {
					log.Errorf(ctx, "Failed to cache tags to %v: %v", t, err)
				} else {
					log.Debugf(ctx, "Cached tags to %v in %v ms", t, msecSince(startTime))
				}
				cacheWriteDone <- struct{}{}
			}(t)
		}
		startTime := time.Now()
		for range cacheWriteTypes {
			<-cacheWriteDone
		}
		log.Debugf(ctx, "Waited %v ms for cache write(s)", msecSince(startTime))
	}

	return tags, nil
}

// Songs executes the supplied query and returns matching songs.
// If cacheOnly is true, empty results are returned if the query's results
// aren't cached.
func Songs(ctx context.Context, query *common.SongQuery, cacheOnly bool) ([]types.Song, error) {
	var ids []int64
	var err error

	// Check memcache first and then datastore.
	var cacheWriteTypes []cache.Type // caches to write to
	for _, t := range []cache.Type{cache.Memcache, cache.Datastore} {
		startTime := time.Now()
		if ids, err = cache.GetQuery(ctx, query, t); err != nil {
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
	cacheWriteDone := make(chan struct{}, len(cacheWriteTypes))
	if !query.CanCache() {
		cacheWriteTypes = nil
	} else {
		for _, t := range cacheWriteTypes {
			go func(t cache.Type, ids []int64) {
				startTime := time.Now()
				if err = cache.SetQuery(ctx, query, ids, t); err != nil {
					log.Errorf(ctx, "Got error while caching results to %v: %v", t, err)
				} else {
					log.Debugf(ctx, "Cached results to %v in %v ms", t, msecSince(startTime))
				}
				cacheWriteDone <- struct{}{}
			}(t, append([]int64{}, ids...)) // duplicate since mutated in main body
		}
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

	songs := make([]types.Song, numResults)
	if numResults == 0 {
		return songs, nil
	}

	// Get the songs from datastore.
	startTime := time.Now()
	keys := make([]*datastore.Key, 0, len(songs))
	for _, id := range ids {
		keys = append(keys, datastore.NewKey(ctx, common.SongKind, "", id, nil))
	}
	if err = datastore.GetMulti(ctx, keys, songs); err != nil {
		return nil, err
	}
	log.Debugf(ctx, "Fetched %v song(s) from datastore in %v ms", len(songs), msecSince(startTime))

	// Prepare the results for the client.
	for i, id := range ids {
		CleanSong(&songs[i], id)
	}
	if !query.Shuffle {
		sortSongs(songs)
	}

	// Wait for async cache writes to finish.
	if len(cacheWriteTypes) > 0 {
		startTime := time.Now()
		for range cacheWriteTypes {
			<-cacheWriteDone
		}
		log.Debugf(ctx, "Waited %v ms for cache write(s)", msecSince(startTime))
	}

	return songs, nil
}

// runQueriesAndGetIDs runs the provided queries in parallel and returns the
// results from each.
func runQueriesAndGetIDs(ctx context.Context, qs []*datastore.Query) ([][]int64, error) {
	type queryResult struct {
		Index int
		IDs   []int64
		Error error
	}
	ch := make(chan queryResult, len(qs))

	for i, q := range qs {
		go func(index int, q *datastore.Query) {
			ids := make([]int64, 0)
			it := q.Run(ctx)
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
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			ch <- queryResult{index, ids, nil}
		}(i, q)
	}

	res := make([][]int64, len(qs))
	for _ = range qs {
		qr := <-ch
		if qr.Error != nil {
			return nil, qr.Error
		}
		res[qr.Index] = qr.IDs
	}
	return res, nil
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

func runQuery(ctx context.Context, query *common.SongQuery) ([]int64, error) {
	// First, build a base query with all of the equality filters.
	bq := datastore.NewQuery(common.SongKind).KeysOnly()
	if len(query.Artist) > 0 {
		bq = bq.Filter("ArtistLower =", strings.ToLower(query.Artist))
	}
	if len(query.Title) > 0 {
		bq = bq.Filter("TitleLower =", strings.ToLower(query.Title))
	}
	if len(query.Album) > 0 {
		bq = bq.Filter("AlbumLower =", strings.ToLower(query.Album))
	}
	if len(query.AlbumID) > 0 {
		bq = bq.Filter("AlbumId =", query.AlbumID)
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
	unmergedIDs, err := runQueriesAndGetIDs(ctx, qs)
	if err != nil {
		return nil, err
	}
	log.Debugf(ctx, "Ran %v query(s) in %v ms", len(qs), msecSince(startTime))

	var mergedIDs []int64
	for i, a := range unmergedIDs {
		if i == 0 {
			mergedIDs = a
		} else if i < negativeQueryStart {
			mergedIDs = intersectSortedIDs(mergedIDs, a)
		} else {
			mergedIDs = filterSortedIDs(mergedIDs, a)
		}
	}
	return mergedIDs, nil
}

// CleanSong prepares s to be returned in results.
// This is exported so it can be called by tests.
func CleanSong(s *types.Song, id int64) {
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
func sortSongs(songs []types.Song) {
	sort.Slice(songs, func(i, j int) bool {
		si := songs[i]
		sj := songs[j]
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

// msecSince returns the number of elapsed milliseconds since t.
func msecSince(t time.Time) int64 {
	return time.Now().Sub(t).Nanoseconds() / int64(time.Millisecond/time.Nanosecond)
}
