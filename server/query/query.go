// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package query loads songs and tags from datastore.
package query

import (
	"context"
	"errors"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/derat/nup/cloudutil"
	"github.com/derat/nup/server/cache"
	"github.com/derat/nup/server/common"
	"github.com/derat/nup/types"

	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
)

const maxQueryResults = 100 // max songs to return for query

// Tags returns the full set of tags present across all songs.
// It attempts to return cached data before falling back to scanning all songs.
// If songs are scanned, the resulting tags are cached.
// If requireCache is true, an error is returned if tags aren't cached.
func Tags(ctx context.Context, requireCache bool) (tags []string, err error) {
	// Check memcache first.
	startTime := time.Now()
	if tags, err = cache.GetTagsMemcache(ctx); err != nil {
		log.Errorf(ctx, "Got error while getting cached tags from memcache: %v", err)
	} else if tags == nil {
		log.Debugf(ctx, "Memcache cache miss took %v ms", msecSince(startTime))
	} else {
		log.Debugf(ctx, "Got %v cached tag(s) from memcache in %v ms",
			len(tags), msecSince(startTime))
		return tags, nil
	}

	// If memcache didn't have the tags, schedule writing them there on our way out.
	saveToMemcache := false
	defer func() {
		if saveToMemcache {
			startTime := time.Now()
			if err := cache.SetTagsMemcache(ctx, tags); err != nil {
				log.Errorf(ctx, "Failed to cache tags to memcache: %v", err)
			} else {
				log.Debugf(ctx, "Cached tags to memcache in %v ms", msecSince(startTime))
			}
		}
	}()

	// Try to get the cached tags from datastore.
	startTime = time.Now()
	if tags, err = cache.GetTagsDatastore(ctx); err != nil {
		log.Errorf(ctx, "Got error while getting cached tags from datastore: %v", err)
	} else if tags == nil {
		log.Debugf(ctx, "Datastore cache miss took %v ms", msecSince(startTime))
	} else {
		log.Debugf(ctx, "Got %v cached tag(s) from datastore in %v ms",
			len(tags), msecSince(startTime))
		saveToMemcache = true
		return tags, nil
	}

	if requireCache {
		return nil, errors.New("tags not cached")
	}

	// Fall back to running a slow query across all songs.
	startTime = time.Now()
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
	log.Debugf(ctx, "Queried %v tag(s) from datastore in %v ms",
		len(tags), msecSince(startTime))
	saveToMemcache = true

	// This write can be slow and will block the HTTP response, but callers
	// should be getting the tags asynchronously anyway.
	startTime = time.Now()
	if err := cache.SetTagsDatastore(ctx, tags); err != nil {
		log.Errorf(ctx, "Failed to cache tags to datastore: %v", err)
	} else {
		log.Debugf(ctx, "Cached tags to datastore in %v ms", msecSince(startTime))
	}

	return tags, nil
}

// Songs executes the supplied query and returns matching songs.
// If cacheOnly is true, empty results are returned if the query's results
// aren't cached.
func Songs(ctx context.Context, query *common.SongQuery, cacheOnly bool) ([]types.Song, error) {
	var ids []int64
	var err error

	cfg := common.Config(ctx)
	startTime := time.Now()
	ids, err = cache.GetQueryResults(ctx, query)
	if err != nil {
		log.Errorf(ctx, "Got error while getting cached query: %v", err)
	} else if ids != nil {
		log.Debugf(ctx, "Got cached query result with %v song(s) in %v ms",
			len(ids), msecSince(startTime))
	}

	if ids == nil {
		if cacheOnly {
			ids = make([]int64, 0)
		} else {
			if ids, err = runQuery(ctx, query); err != nil {
				return nil, err
			}
			if query.CanCache() {
				startTime := time.Now()
				if err = cache.SetQueryResults(ctx, query, ids); err != nil {
					log.Errorf(ctx, "Got error while caching query result: %v", err)
				} else {
					log.Debugf(ctx, "Cached query result with %v song(s) in %v ms", len(ids),
						msecSince(startTime))
				}
			}
		}
	}

	numResults := len(ids)
	if numResults > maxQueryResults {
		numResults = maxQueryResults
	}

	if query.Shuffle {
		shufflePartial(ids, numResults)
	}

	ids = ids[:numResults]
	songs := make([]types.Song, numResults)

	if numResults == 0 {
		return songs, nil
	}

	cachedSongs := make(map[int64]types.Song)
	storedSongs := make([]types.Song, 0)

	// Get the remaining songs from datastore and write them back to memcache.
	if len(cachedSongs) < numResults {
		startTime := time.Now()
		numStored := numResults - len(cachedSongs)
		storedIds := make([]int64, 0, numStored)
		keys := make([]*datastore.Key, 0, numStored)
		for _, id := range ids {
			if _, ok := cachedSongs[id]; !ok {
				storedIds = append(storedIds, id)
				keys = append(keys, datastore.NewKey(ctx, common.SongKind, "", id, nil))
			}
		}
		storedSongs = make([]types.Song, len(keys))
		if err = datastore.GetMulti(ctx, keys, storedSongs); err != nil {
			return nil, err
		}
		log.Debugf(ctx, "Fetched %v song(s) from datastore in %v ms",
			len(storedSongs), msecSince(startTime))
	}

	storedIndex := 0
	for i, id := range ids {
		if s, ok := cachedSongs[id]; ok {
			songs[i] = s
		} else {
			songs[i] = storedSongs[storedIndex]
			storedIndex++
		}
		common.PrepareSongForClient(&songs[i], id, cfg, cloudutil.WebClient)
	}

	if !query.Shuffle {
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
	return songs, nil
}

func runQueriesAndGetIds(ctx context.Context, qs []*datastore.Query) ([][]int64, error) {
	type queryResult struct {
		Index int
		Ids   []int64
		Error error
	}
	ch := make(chan queryResult)

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
	unmergedIds, err := runQueriesAndGetIds(ctx, qs)
	if err != nil {
		return nil, err
	}
	log.Debugf(ctx, "Ran %v query(s) in %v ms", len(qs), msecSince(startTime))

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
	return mergedIds, nil
}

// Returns the number of elapsed milliseconds since t.
func msecSince(t time.Time) int64 {
	return time.Now().Sub(t).Nanoseconds() / int64(time.Millisecond/time.Nanosecond)
}
