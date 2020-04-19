package main

import (
	"context"
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

	coverExpiration = 24 * time.Hour
)

var jsonCodec = memcache.Codec{
	Marshal:   json.Marshal,
	Unmarshal: json.Unmarshal,
}

type cachedQuery struct {
	Query songQuery
	Ids   []int64 // song IDs
}

type cachedQueries map[string]cachedQuery // keys are from songQuery.hash()

// Wraps a JSON-encoded cachedQueries.
type encodedCachedQueries struct {
	Data []byte
}

type cachedTags struct {
	Tags []string
}

// datastoreCachedQueriesKey returns the datastore key for caching queries.
func datastoreCachedQueriesKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, cachedQueriesKind, queriesCacheKey, 0, nil)
}

// getAllCachedQueries returns all cached queries. It attempts to read queries
// from memcache first before falling back to datastore.
func getAllCachedQueries(ctx context.Context) (cachedQueries, error) {
	qs := make(cachedQueries)
	if _, err := jsonCodec.Get(ctx, queriesCacheKey, &qs); err == nil {
		return qs, nil
	} else if err != nil && err != memcache.ErrCacheMiss {
		log.Errorf(ctx, "Getting cached queries from memcache failed: %v", err) // ignore
	}

	var eqs encodedCachedQueries
	if err := datastore.Get(ctx, datastoreCachedQueriesKey(ctx), &eqs); err == nil {
		if err := json.Unmarshal(eqs.Data, &qs); err != nil {
			return nil, err
		}
	} else if err != datastore.ErrNoSuchEntity {
		return nil, err
	}
	return qs, nil
}

// getCachedQueryResults returns cached results for q.
// If the query isn't cached, then a nil slice is returned.
func getCachedQueryResults(ctx context.Context, q *songQuery) ([]int64, error) {
	qs, err := getAllCachedQueries(ctx)
	if err != nil {
		return nil, err
	}
	if len(qs) == 0 {
		return nil, nil
	}

	if q, ok := qs[q.hash()]; ok {
		return q.Ids, nil
	}
	return nil, nil
}

// updateCachedQueries loads all cached queries, passes them to f, and saves the
// updated queries to both memcache and datastore.
// If f returns errUnmodified, the queries won't be re-cached.
func updateCachedQueries(ctx context.Context, f func(cachedQueries) error) error {
	qs, err := getAllCachedQueries(ctx)
	if err != nil {
		return err
	}

	if err := f(qs); err == errUnmodified {
		return nil
	} else if err != nil {
		return err
	}

	// Update memcache.
	if err := jsonCodec.Set(ctx, &memcache.Item{
		Key:    queriesCacheKey,
		Object: &qs,
	}); err != nil {
		// Delete stale data if the update failed.
		log.Errorf(ctx, "Updating cached queries in memcache failed: %v", err)
		if err := memcache.Delete(ctx, queriesCacheKey); err != nil && err != memcache.ErrCacheMiss {
			log.Errorf(ctx, "Deleting cached queries in memcache failed: %v", err)
		}
	}

	// Update datastore.
	key := datastoreCachedQueriesKey(ctx)
	b, err := json.Marshal(qs)
	if err == nil {
		_, err = datastore.Put(ctx, key, &encodedCachedQueries{b})
	}
	if err != nil {
		// Delete stale data if the update failed.
		if derr := datastore.Delete(ctx, key); derr != nil && derr != datastore.ErrNoSuchEntity {
			log.Errorf(ctx, "Deleting cached queries in datastore failed: %v", derr)
		}
	}
	return err
}

// writeQueryResultsToCache caches ids as results for query.
func writeQueryResultsToCache(ctx context.Context, q *songQuery, ids []int64) error {
	return updateCachedQueries(ctx, func(qs cachedQueries) error {
		qs[q.hash()] = cachedQuery{*q, ids}
		return nil
	})
}

// flushCacheForUpdate deletes the appropriate cached data for an update of the
// supplied types.
func flushCacheForUpdate(ctx context.Context, ut updateTypes) error {
	flushed := 0
	if err := updateCachedQueries(ctx, func(qs cachedQueries) error {
		for k, cq := range qs {
			q := cq.Query
			if (ut&metadataUpdate) != 0 ||
				((ut&ratingUpdate) != 0 && (q.HasMinRating || q.Unrated)) ||
				((ut&tagsUpdate) != 0 && (len(q.Tags) > 0 || len(q.NotTags) > 0)) ||
				((ut&playUpdate) != 0 && (q.HasMaxPlays || !q.MinFirstStartTime.IsZero() || !q.MaxLastStartTime.IsZero())) {
				delete(qs, k)
				flushed++
			}
		}
		if flushed == 0 {
			return errUnmodified
		}
		return nil
	}); err != nil {
		return err
	}
	if flushed > 0 {
		log.Debugf(ctx, "Flushed %v cached query(s) in response to update of type %v", flushed, ut)
	}

	if ut&tagsUpdate != 0 || ut&metadataUpdate != 0 {
		if err := flushTagsFromCache(ctx); err != nil {
			return err
		}
		log.Debugf(ctx, "Flushed cached tags in response to update of type %v", ut)
	}

	return nil
}

func datastoreCachedTagsKey(ctx context.Context) *datastore.Key {
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
		if err := datastore.Get(ctx, datastoreCachedTagsKey(ctx), &t); err == datastore.ErrNoSuchEntity {
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
		_, err := datastore.Put(ctx, datastoreCachedTagsKey(ctx), &t)
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
		if err := datastore.Delete(ctx, datastoreCachedTagsKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
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
		Expiration: coverExpiration,
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
	if err := datastore.Delete(ctx, datastoreCachedQueriesKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
		return err
	}
	return nil
}
