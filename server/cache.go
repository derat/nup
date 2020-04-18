package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/derat/nup/types"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
)

const (
	// Datastore kind for cached queries and tags.
	cachedQueriesKind = "CachedQueries"
	cachedTagsKind    = "CachedTags"

	// Memcache key (and also datastore ID :-/) for cached queries and tags.
	queriesCacheKey = "queries"
	tagsCacheKey    = "tags"

	// Memcache key prefix for cached songs and cover images.
	songCachePrefix  = "song-"
	coverCachePrefix = "cover-"
)

var jsonCodec = memcache.Codec{
	Marshal:   json.Marshal,
	Unmarshal: json.Unmarshal,
}

type cachedQuery struct {
	Query songQuery
	Ids   []int64
}

type cachedQueries map[string]cachedQuery

// Ugh.
type encodedCachedQueries struct {
	Data []byte
}

type cachedTags struct {
	Tags []string
}

func getDatastoreCachedQueriesKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, cachedQueriesKind, queriesCacheKey, 0, nil)
}

func shouldCacheQuery(q *songQuery) bool {
	return !q.HasMaxPlays && q.MinFirstStartTime.IsZero() && q.MaxLastStartTime.IsZero()
}

func computeQueryHash(q *songQuery) (string, error) {
	b, err := json.Marshal(q)
	if err != nil {
		return "", err
	}
	s := sha1.Sum(b)
	return hex.EncodeToString(s[:]), nil
}

func getAllCachedQueries(ctx context.Context) (cachedQueries, error) {
	queries := make(cachedQueries)
	switch getConfig(ctx).CacheQueries {
	case types.MemcacheCaching:
		if _, err := jsonCodec.Get(ctx, queriesCacheKey, &queries); err != nil && err != memcache.ErrCacheMiss {
			return nil, err
		}
	case types.DatastoreCaching:
		eq := encodedCachedQueries{}
		if err := datastore.Get(ctx, getDatastoreCachedQueriesKey(ctx), &eq); err == nil {
			if err := json.Unmarshal(eq.Data, &queries); err != nil {
				return nil, err
			}
		} else if err != datastore.ErrNoSuchEntity {
			return nil, err
		}
	}
	return queries, nil
}

func getCachedQueryResults(ctx context.Context, query *songQuery) ([]int64, error) {
	queries, err := getAllCachedQueries(ctx)
	if err != nil {
		return nil, err
	}
	if len(queries) == 0 {
		return nil, nil
	}

	queryHash, err := computeQueryHash(query)
	if err != nil {
		return nil, err
	}
	if q, ok := queries[queryHash]; ok {
		return q.Ids, nil
	}
	return nil, nil
}

func updateCachedQueries(ctx context.Context, f func(cachedQueries) error) error {
	queries, err := getAllCachedQueries(ctx)
	if err != nil {
		return err
	}

	if err := f(queries); err == errUnmodified {
		return nil
	} else if err != nil {
		return err
	}

	switch getConfig(ctx).CacheQueries {
	case types.MemcacheCaching:
		return jsonCodec.Set(ctx, &memcache.Item{Key: queriesCacheKey, Object: &queries})
	case types.DatastoreCaching:
		b, err := json.Marshal(queries)
		if err != nil {
			return err
		}
		_, err = datastore.Put(ctx, getDatastoreCachedQueriesKey(ctx), &encodedCachedQueries{b})
		return err
	default:
		return errors.New("query caching is disabled")
	}
}

func writeQueryResultsToCache(ctx context.Context, query *songQuery, ids []int64) error {
	return updateCachedQueries(ctx, func(queries cachedQueries) error {
		queryHash, err := computeQueryHash(query)
		if err != nil {
			return err
		}
		queries[queryHash] = cachedQuery{*query, ids}
		return nil
	})
}

func flushDataFromCacheForUpdate(ctx context.Context, updateType uint) error {
	numFlushed := 0
	if err := updateCachedQueries(ctx, func(queries cachedQueries) error {
		for k, cq := range queries {
			q := cq.Query
			if (updateType&metadataUpdate) != 0 ||
				((updateType&ratingUpdate) != 0 && (q.HasMinRating || q.Unrated)) ||
				((updateType&tagsUpdate) != 0 && (len(q.Tags) > 0 || len(q.NotTags) > 0)) ||
				((updateType&playUpdate) != 0 && (q.HasMaxPlays || !q.MinFirstStartTime.IsZero() || !q.MaxLastStartTime.IsZero())) {
				delete(queries, k)
				numFlushed++
			}
		}
		if numFlushed == 0 {
			return errUnmodified
		}
		return nil
	}); err != nil {
		return err
	}
	if numFlushed > 0 {
		log.Debugf(ctx, "Flushed %v cached query(s) in response to update of type %v", numFlushed, updateType)
	}

	if updateType&tagsUpdate != 0 || updateType&metadataUpdate != 0 {
		if err := flushTagsFromCache(ctx); err != nil {
			return err
		}
		log.Debugf(ctx, "Flushed cached tags in response to update of type %v", updateType)
	}

	return nil
}

func getDatastoreCachedTagsKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, cachedTagsKind, tagsCacheKey, 0, nil)
}

func getTagsFromCache(ctx context.Context) ([]string, error) {
	t := cachedTags{}
	switch getConfig(ctx).CacheTags {
	case types.MemcacheCaching:
		if _, err := jsonCodec.Get(ctx, tagsCacheKey, &t); err != nil && err != memcache.ErrCacheMiss {
			return nil, err
		}
	case types.DatastoreCaching:
		if err := datastore.Get(ctx, getDatastoreCachedTagsKey(ctx), &t); err == datastore.ErrNoSuchEntity {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
	}
	return t.Tags, nil
}

func writeTagsToCache(ctx context.Context, tags []string) error {
	t := cachedTags{tags}
	switch getConfig(ctx).CacheTags {
	case types.MemcacheCaching:
		return jsonCodec.Set(ctx, &memcache.Item{Key: tagsCacheKey, Object: &t})
	case types.DatastoreCaching:
		_, err := datastore.Put(ctx, getDatastoreCachedTagsKey(ctx), &t)
		return err
	default:
		return errors.New("tag caching is disabled")
	}
}

func flushTagsFromCache(ctx context.Context) error {
	switch getConfig(ctx).CacheTags {
	case types.MemcacheCaching:
		if err := memcache.Delete(ctx, tagsCacheKey); err != nil && err != memcache.ErrCacheMiss {
			return err
		}
	case types.DatastoreCaching:
		if err := datastore.Delete(ctx, getDatastoreCachedTagsKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
	}
	return nil
}

func songCacheKey(id int64) string {
	return songCachePrefix + strconv.FormatInt(id, 10)
}

func flushSongFromMemcache(ctx context.Context, id int64) error {
	if err := memcache.Delete(ctx, songCacheKey(id)); err != nil && err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}

func getSongsFromMemcache(ctx context.Context, ids []int64) (songs map[int64]types.Song, err error) {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = songCacheKey(id)
	}

	// Uh, no memcache.Codec.GetMulti()?
	songs = make(map[int64]types.Song)
	items, err := memcache.GetMulti(ctx, keys)
	if err != nil {
		return nil, err
	}

	for idStr, item := range items {
		if !strings.HasPrefix(idStr, songCachePrefix) {
			return nil, fmt.Errorf("got unexpected key %q from cache", idStr)
		}
		id, err := strconv.ParseInt(idStr[len(songCachePrefix):], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse key %q: %v", idStr, err)
		}
		s := types.Song{}
		if err = json.Unmarshal(item.Value, &s); err != nil {
			return nil, fmt.Errorf("failed to unmarshal cached song %v: %v", id, err)
		}
		songs[id] = s
	}
	return songs, nil
}

func flushSongsFromMemcacheAfterMultiError(ctx context.Context, ids []int64, me appengine.MultiError) error {
	for i, err := range me {
		id := ids[i]
		if err == memcache.ErrNotStored {
			log.Debugf(ctx, "Song %v already present in cache; flushing", id)
			if err := flushSongFromMemcache(ctx, id); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func writeSongsToMemcache(ctx context.Context, ids []int64, songs []types.Song, flushIfAlreadyPresent bool) error {
	if len(ids) != len(songs) {
		return fmt.Errorf("got request to write %v ID(s) with %v song(s) to cache", len(ids), len(songs))
	}

	items := make([]*memcache.Item, len(songs))
	for i, id := range ids {
		items[i] = &memcache.Item{Key: songCacheKey(id), Object: &songs[i]}
	}
	if err := jsonCodec.AddMulti(ctx, items); err != nil {
		// Some of the songs might've been cached in response to a query in the meantime.
		// memcache.Delete() is missing a lock duration (https://code.google.com/p/googleappengine/issues/detail?id=10983),
		// so just do the best we can and try to delete the possibly-stale cached values.
		if me, ok := err.(appengine.MultiError); ok && flushIfAlreadyPresent {
			return flushSongsFromMemcacheAfterMultiError(ctx, ids, me)
		}
		return err
	}
	return nil
}

// coverCacheKey returns the memcache key that should be used for caching a
// cover image with the supplied filename and size (i.e. width/height).
func coverCacheKey(fn string, size int) string {
	// TODO: Hash the filename?
	// https://godoc.org/google.golang.org/appengine/memcache#Get says that the
	// key can be at most 250 bytes.
	return fmt.Sprintf("%s-%s-%d", coverCachePrefix, size, fn)
}

// writeCoverToMemcache caches a cover image with the supplied filename,
// requested size, and raw data. size should be 0 when caching the original image.
func writeCoverToMemcache(ctx context.Context, fn string, size int, data []byte) error {
	return memcache.Set(ctx, &memcache.Item{
		Key:        coverCacheKey(fn, size),
		Value:      data,
		Expiration: 24 * time.Hour,
	})
}

// getCoverFromMemcache attempts to look up raw data for the cover image with
// the supplied filename and size. If the image isn't present, both the returned
// byte slice and the error are nil.
func getCoverFromMemcache(ctx context.Context, fn string, size int) ([]byte, error) {
	item, err := memcache.Get(ctx, coverCacheKey(fn, size))
	if err == memcache.ErrCacheMiss {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return item.Value, nil
}

func flushCache(ctx context.Context) error {
	// This is only used by tests, so we flush both memcache and datastore
	// regardless of what the config says.
	if err := memcache.Flush(ctx); err != nil {
		return err
	}
	if err := datastore.Delete(ctx, getDatastoreCachedQueriesKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
		return err
	}
	return nil
}
