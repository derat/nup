package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
)

const (
	// Datastore kind for cached queries and tags.
	cachedQueriesKind = "CachedQueries"
	cachedTagsKind    = "CachedTags"

	// Memcache key (and also datastore ID) for cached queries and tags.
	cachedQueriesKey = "queries"
	cachedTagsKey    = "tags"

	// Memcache key prefix and expiration for cached cover images.
	coverCachePrefix = "cover-"
	coverExpiration  = 24 * time.Hour
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

// cachedQueriesDatastoreKey returns the datastore key for caching queries.
func cachedQueriesDatastoreKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, cachedQueriesKind, cachedQueriesKey, 0, nil)
}

// getAllCachedQueries returns all cached queries. It attempts to read queries
// from memcache first before falling back to datastore. If datastore fails, an
// error is returned.
func getAllCachedQueries(ctx context.Context) (cachedQueries, error) {
	qs := make(cachedQueries)
	if _, err := jsonCodec.Get(ctx, cachedQueriesKey, &qs); err == nil {
		return qs, nil
	} else if err != nil && err != memcache.ErrCacheMiss {
		log.Errorf(ctx, "Getting cached queries from memcache failed: %v", err) // ignore
	}

	var eqs encodedCachedQueries
	if err := datastore.Get(ctx, cachedQueriesDatastoreKey(ctx), &eqs); err == datastore.ErrNoSuchEntity {
		return qs, nil
	} else if err != nil {
		return nil, err
	}
	err := json.Unmarshal(eqs.Data, &qs)
	return qs, err
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

	var errs []error
	errs = append(errs, writeToMemcache(ctx, cachedQueriesKey, &qs))
	if b, err := json.Marshal(qs); err != nil {
		errs = append(errs, err)
	} else {
		errs = append(errs, writeToDatastoreCache(ctx, cachedQueriesDatastoreKey(ctx), &encodedCachedQueries{b}))
	}
	return joinErrors(errs)
}

// writeCachedQuery caches ids as results for query.
func writeCachedQuery(ctx context.Context, q *songQuery, ids []int64) error {
	return updateCachedQueries(ctx, func(qs cachedQueries) error {
		qs[q.hash()] = cachedQuery{*q, ids}
		return nil
	})
}

// flushCacheForUpdate deletes the appropriate cached data for an update of the
// supplied types.
func flushCacheForUpdate(ctx context.Context, ut updateTypes) error {
	var errs []error

	errs = append(errs, updateCachedQueries(ctx, func(qs cachedQueries) error {
		flushed := 0
		for i, q := range qs {
			if q.Query.resultsInvalidated(ut) {
				delete(qs, i)
				flushed++
			}
		}
		if flushed == 0 {
			return errUnmodified
		}
		log.Debugf(ctx, "Flushing %v cached query(s) in response to update of type %v", flushed, ut)
		return nil
	}))

	if ut&tagsUpdate != 0 || ut&metadataUpdate != 0 {
		log.Debugf(ctx, "Flushing cached tags in response to update of type %v", ut)
		errs = append(errs, deleteFromMemcache(ctx, cachedTagsKey))
		errs = append(errs, deleteFromDatastoreCache(ctx, cachedTagsDatastoreKey(ctx)))
	}

	return joinErrors(errs)
}

// cachedTagsDatastoreKey returns the datastore key for caching tags.
func cachedTagsDatastoreKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, cachedTagsKind, cachedTagsKey, 0, nil)
}

// readCachedTagsFromMemcache attempts to get the list of in-use tags from memcache.
// On a cache miss, both returned values are nil.
func readCachedTagsFromMemcache(ctx context.Context) ([]string, error) {
	var t cachedTags
	if _, err := jsonCodec.Get(ctx, cachedTagsKey, &t); err == memcache.ErrCacheMiss {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return t.Tags, nil
}

// readCachedTagsFromDatastore attempts to get the list of in-use tags from datastore.
// On a cache miss, both returned values are nil.
func readCachedTagsFromDatastore(ctx context.Context) ([]string, error) {
	var t cachedTags
	if err := datastore.Get(ctx, cachedTagsDatastoreKey(ctx), &t); err == datastore.ErrNoSuchEntity {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return t.Tags, nil
}

func writeCachedTagsToMemcache(ctx context.Context, tags []string) error {
	return writeToMemcache(ctx, cachedTagsKey, &cachedTags{tags})
}

func writeCachedTagsToDatastore(ctx context.Context, tags []string) error {
	return writeToDatastoreCache(ctx, cachedTagsDatastoreKey(ctx), &cachedTags{tags})
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

// flushMemcacheCache deletes all cached objects from memcache.
func flushMemcacheCache(ctx context.Context) error {
	return memcache.Flush(ctx)
}

// flushDatastoreCache deletes all cached objects from datastore.
func flushDatastoreCache(ctx context.Context) error {
	var errs []error
	errs = append(errs, deleteFromDatastoreCache(ctx, cachedQueriesDatastoreKey(ctx)))
	errs = append(errs, deleteFromDatastoreCache(ctx, cachedTagsDatastoreKey(ctx)))
	return joinErrors(errs)
}

// writeToMemcache saves obj at key in memcache.
// If the update fails, the stale object (if present) is deleted.
func writeToMemcache(ctx context.Context, key string, obj interface{}) error {
	var errs []error
	if err := jsonCodec.Set(ctx, &memcache.Item{Key: key, Object: obj}); err != nil {
		errs = append(errs, fmt.Errorf("set failed: %v", err))
		if err := deleteFromMemcache(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete failed: %v", err))
		}
	}
	return joinErrors(errs)
}

// writeToDatastoreCache saves obj at key in datastore.
// If the update fails, the stale object (if present) is deleted.
func writeToDatastoreCache(ctx context.Context, key *datastore.Key, obj interface{}) error {
	var errs []error
	if _, err := datastore.Put(ctx, key, obj); err != nil {
		errs = append(errs, fmt.Errorf("put failed: %v", err))
		if err := deleteFromDatastoreCache(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete failed: %v", err))
		}
	}
	return joinErrors(errs)
}

// deleteFromMemcache deletes key from memcache.
// nil is returned if the key isn't present.
func deleteFromMemcache(ctx context.Context, key string) error {
	if err := memcache.Delete(ctx, key); err != nil && err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}

// deleteFromDatastoreCache deletes key from datastore.
// nil is returned if the key isn't present.
func deleteFromDatastoreCache(ctx context.Context, key *datastore.Key) error {
	if err := datastore.Delete(ctx, key); err != nil && err != datastore.ErrNoSuchEntity {
		return err
	}
	return nil
}
