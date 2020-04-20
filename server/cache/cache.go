// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package cache sets and gets data from memcache and datastore.
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/derat/nup/server/common"

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
	Query common.SongQuery
	Ids   []int64 // song IDs
}

type cachedQueries map[string]cachedQuery // keys are from SongQuery.Hash()

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

// GetQueryResults returns cached results for q.
// If the query isn't cached, then a nil slice is returned.
// TODO: Split this into memcache vs. datastore.
func GetQueryResults(ctx context.Context, q *common.SongQuery) ([]int64, error) {
	qs, err := getAllCachedQueries(ctx)
	if err != nil {
		return nil, err
	}
	if len(qs) == 0 {
		return nil, nil
	}

	if q, ok := qs[q.Hash()]; ok {
		return q.Ids, nil
	}
	return nil, nil
}

// updateCachedQueries loads all cached queries, passes them to f, and saves the
// updated queries to both memcache and datastore.
// If f returns common.ErrUnmodified, the queries won't be re-cached.
func updateCachedQueries(ctx context.Context, f func(cachedQueries) error) error {
	qs, err := getAllCachedQueries(ctx)
	if err != nil {
		return err
	}

	if err := f(qs); err == common.ErrUnmodified {
		return nil
	} else if err != nil {
		return err
	}

	var errs []error
	errs = append(errs, setMemcache(ctx, cachedQueriesKey, &qs))
	if b, err := json.Marshal(qs); err != nil {
		errs = append(errs, err)
	} else {
		errs = append(errs, setDatastore(ctx, cachedQueriesDatastoreKey(ctx), &encodedCachedQueries{b}))
	}
	return joinErrors(errs)
}

// SetQueryResults caches ids as results for query.
func SetQueryResults(ctx context.Context, q *common.SongQuery, ids []int64) error {
	return updateCachedQueries(ctx, func(qs cachedQueries) error {
		qs[q.Hash()] = cachedQuery{*q, ids}
		return nil
	})
}

// FlushForUpdate deletes the appropriate cached data for an update of the supplied types.
func FlushForUpdate(ctx context.Context, ut common.UpdateTypes) error {
	var errs []error

	errs = append(errs, updateCachedQueries(ctx, func(qs cachedQueries) error {
		flushed := 0
		for i, q := range qs {
			if q.Query.ResultsInvalidated(ut) {
				delete(qs, i)
				flushed++
			}
		}
		if flushed == 0 {
			return common.ErrUnmodified
		}
		log.Debugf(ctx, "Flushing %v cached query(s) in response to update of type %v", flushed, ut)
		return nil
	}))

	if ut&common.TagsUpdate != 0 || ut&common.MetadataUpdate != 0 {
		log.Debugf(ctx, "Flushing cached tags in response to update of type %v", ut)
		errs = append(errs, deleteMemcache(ctx, cachedTagsKey))
		errs = append(errs, deleteDatastore(ctx, cachedTagsDatastoreKey(ctx)))
	}

	return joinErrors(errs)
}

// cachedTagsDatastoreKey returns the datastore key for caching tags.
func cachedTagsDatastoreKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, cachedTagsKind, cachedTagsKey, 0, nil)
}

// GetTagsMemcache attempts to get the list of in-use tags from memcache.
// On a cache miss, both returned values are nil.
func GetTagsMemcache(ctx context.Context) ([]string, error) {
	var t cachedTags
	if _, err := jsonCodec.Get(ctx, cachedTagsKey, &t); err == memcache.ErrCacheMiss {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return t.Tags, nil
}

// GetTagsDatastore attempts to get the list of in-use tags from datastore.
// On a cache miss, both returned values are nil.
func GetTagsDatastore(ctx context.Context) ([]string, error) {
	var t cachedTags
	if err := datastore.Get(ctx, cachedTagsDatastoreKey(ctx), &t); err == datastore.ErrNoSuchEntity {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return t.Tags, nil
}

func SetTagsMemcache(ctx context.Context, tags []string) error {
	return setMemcache(ctx, cachedTagsKey, &cachedTags{tags})
}

func SetTagsDatastore(ctx context.Context, tags []string) error {
	return setDatastore(ctx, cachedTagsDatastoreKey(ctx), &cachedTags{tags})
}

// coverCacheKey returns the memcache key that should be used for caching a
// cover image with the supplied filename and size (i.e. width/height).
func coverCacheKey(fn string, size int) string {
	// TODO: Hash the filename?
	// https://godoc.org/google.golang.org/appengine/memcache#Get says that the
	// key can be at most 250 bytes.
	return fmt.Sprintf("%s-%s-%d", coverCachePrefix, size, fn)
}

// SetCoverMemcache caches a cover image with the supplied filename,
// requested size, and raw data. size should be 0 when caching the original image.
func SetCoverMemcache(ctx context.Context, fn string, size int, data []byte) error {
	return memcache.Set(ctx, &memcache.Item{
		Key:        coverCacheKey(fn, size),
		Value:      data,
		Expiration: coverExpiration,
	})
}

// GetCoverMemcache attempts to look up raw data for the cover image with
// the supplied filename and size. If the image isn't present, both the returned
// byte slice and the error are nil.
func GetCoverMemcache(ctx context.Context, fn string, size int) ([]byte, error) {
	item, err := memcache.Get(ctx, coverCacheKey(fn, size))
	if err == memcache.ErrCacheMiss {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return item.Value, nil
}

// FlushMemcache deletes all cached objects from memcache.
func FlushMemcache(ctx context.Context) error {
	return memcache.Flush(ctx)
}

// FlushDatastore deletes all cached objects from datastore.
func FlushDatastore(ctx context.Context) error {
	var errs []error
	errs = append(errs, deleteDatastore(ctx, cachedQueriesDatastoreKey(ctx)))
	errs = append(errs, deleteDatastore(ctx, cachedTagsDatastoreKey(ctx)))
	return joinErrors(errs)
}

// setMemcache saves obj at key in memcache.
// If the update fails, the stale object (if present) is deleted.
func setMemcache(ctx context.Context, key string, obj interface{}) error {
	var errs []error
	if err := jsonCodec.Set(ctx, &memcache.Item{Key: key, Object: obj}); err != nil {
		errs = append(errs, fmt.Errorf("set failed: %v", err))
		if err := deleteMemcache(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete failed: %v", err))
		}
	}
	return joinErrors(errs)
}

// setDatastore saves obj at key in datastore.
// If the update fails, the stale object (if present) is deleted.
func setDatastore(ctx context.Context, key *datastore.Key, obj interface{}) error {
	var errs []error
	if _, err := datastore.Put(ctx, key, obj); err != nil {
		errs = append(errs, fmt.Errorf("put failed: %v", err))
		if err := deleteDatastore(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete failed: %v", err))
		}
	}
	return joinErrors(errs)
}

// deleteMemcache deletes key from memcache.
// nil is returned if the key isn't present.
func deleteMemcache(ctx context.Context, key string) error {
	if err := memcache.Delete(ctx, key); err != nil && err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}

// deleteDatastore deletes key from datastore.
// nil is returned if the key isn't present.
func deleteDatastore(ctx context.Context, key *datastore.Key) error {
	if err := datastore.Delete(ctx, key); err != nil && err != datastore.ErrNoSuchEntity {
		return err
	}
	return nil
}

// joinErrors returns a new error all messages from any non-nil errors in errs.
// If no non-nil errors are present, nil is returned.
// TODO: Delete this?
func joinErrors(errs []error) error {
	var all error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if all == nil {
			all = err
		} else {
			all = fmt.Errorf("%v; %v", all.Error(), err.Error())
		}
	}
	return all
}
