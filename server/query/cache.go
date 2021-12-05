// Copyright 2021 Daniel Erat.
// All rights reserved.

package query

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/derat/nup/server/cache"

	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/log"
)

const (
	cachedQueriesKind = "CachedQueries" // datastore kind for cached query results
	cachedQueriesKey  = "queries"       // memcache key and datastore ID for cached query results

	cachedTagsKind = "CachedTags" // datastore kind for cached tags
	cachedTagsKey  = "tags"       // memcache key and datastore ID for cached tags
)

// cachedQueriesDatastoreKey returns the datastore key for caching queries.
func cachedQueriesDatastoreKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, cachedQueriesKey, cachedQueriesKey, 0, nil)
}

// cachedTagsDatastoreKey returns the datastore key for caching tags.
func cachedTagsDatastoreKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, cachedTagsKind, cachedTagsKey, 0, nil)
}

// cachedQuery holds an individual query and its cached results.
type cachedQuery struct {
	Query SongQuery
	IDs   []int64 // song IDs
}

// cachedQueries holds all cached queries keyed by SongQuery.hash().
// It implements datastore.PropertyLoadSaver.
type cachedQueries map[string]cachedQuery

func (m *cachedQueries) Load(props []datastore.Property) error {
	return cache.LoadJSONProp(props, m)
}
func (m *cachedQueries) Save() ([]datastore.Property, error) {
	return cache.SaveJSONProp(m)
}

// loadCachedQueries returns the cachedQueries object from t.
// If the object isn't present, both returned values are nil.
func loadCachedQueries(ctx context.Context, t cache.Type) (cachedQueries, error) {
	var m cachedQueries
	var ok bool
	var err error
	switch t {
	case cache.Memcache:
		ok, err = cache.GetMemcache(ctx, cachedQueriesKey, &m)
	case cache.Datastore:
		ok, err = cache.GetDatastore(ctx, cachedQueriesDatastoreKey(ctx), &m)
	}
	if !ok {
		return nil, err
	}
	return m, nil
}

var errResultsUnchanged = errors.New("query results unchanged")

// updateCachedQueries loads cachedQueries from t, passes it to f, and saves the updated queries back to t.
// If f returns errResultsUnchanged, the queries won't be saved.
func updateCachedQueries(ctx context.Context, f func(cachedQueries) error, t cache.Type) error {
	m, err := loadCachedQueries(ctx, t)
	if err != nil {
		return err
	}

	if m == nil { // cache miss
		m = make(cachedQueries)
	}
	if err := f(m); err == errResultsUnchanged {
		return nil
	} else if err != nil {
		return err
	}

	switch t {
	case cache.Memcache:
		return cache.SetMemcache(ctx, cachedQueriesKey, m)
	case cache.Datastore:
		return cache.SetDatastore(ctx, cachedQueriesDatastoreKey(ctx), &m)
	default:
		return fmt.Errorf("invalid type %v", t)
	}
}

// getCachedResults returns cached results for q from t.
// If the query isn't cached, then a nil slice is returned.
func getCachedResults(ctx context.Context, q *SongQuery, t cache.Type) ([]int64, error) {
	hash, err := q.hash()
	if err != nil {
		return nil, err
	}
	if m, err := loadCachedQueries(ctx, t); err != nil {
		return nil, err
	} else if m == nil { // cache miss
		return nil, nil
	} else if q, ok := m[hash]; ok { // query among cached queries
		return q.IDs, nil
	}
	return nil, nil // query not among cached queries
}

// setCachedResults caches ids as results for query in t.
func setCachedResults(ctx context.Context, q *SongQuery, ids []int64, t cache.Type) error {
	hash, err := q.hash()
	if err != nil {
		return err
	}
	return updateCachedQueries(ctx, func(m cachedQueries) error {
		m[hash] = cachedQuery{Query: *q, IDs: ids}
		return nil
	}, t)
}

// cachedTags holds the list of tags currently in use.
// It implements datastore.PropertyLoadSaver.
type cachedTags []string

func (t *cachedTags) Load(props []datastore.Property) error {
	return cache.LoadJSONProp(props, t)
}
func (t *cachedTags) Save() ([]datastore.Property, error) {
	return cache.SaveJSONProp(t)
}

// getCachedTags attempts to get the list of in-use tags from t.
// On a cache miss, both returned values are nil.
func getCachedTags(ctx context.Context, t cache.Type) ([]string, error) {
	var tags cachedTags
	var ok bool
	var err error
	switch t {
	case cache.Memcache:
		ok, err = cache.GetMemcache(ctx, cachedTagsKey, &tags)
	case cache.Datastore:
		ok, err = cache.GetDatastore(ctx, cachedTagsDatastoreKey(ctx), &tags)
	}
	if !ok {
		return nil, err
	}
	return tags, nil
}

// setCachedTags saves the list of in-use tags to t.
func setCachedTags(ctx context.Context, tags []string, t cache.Type) error {
	switch t {
	case cache.Memcache:
		return cache.SetMemcache(ctx, cachedTagsKey, tags)
	case cache.Datastore:
		return cache.SetDatastore(ctx, cachedTagsDatastoreKey(ctx), (*cachedTags)(&tags))
	default:
		return fmt.Errorf("invalid type %v", t)
	}
}

// FlushCacheForUpdate deletes the appropriate cached queries for an update of the supplied types.
func FlushCacheForUpdate(ctx context.Context, ut UpdateTypes) error {
	var errs []string

	for _, t := range []cache.Type{cache.Memcache, cache.Datastore} {
		if err := updateCachedQueries(ctx, func(m cachedQueries) error {
			flushed := 0
			for h, q := range m {
				if q.Query.resultsInvalidated(ut) {
					delete(m, h)
					flushed++
				}
			}
			if flushed == 0 {
				return errResultsUnchanged
			}
			log.Debugf(ctx, "Flushing %v cached query(s) from %v in response to update of type %v",
				flushed, t, ut)
			return nil
		}, t); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if ut&TagsUpdate != 0 || ut&MetadataUpdate != 0 {
		log.Debugf(ctx, "Flushing cached tags in response to update of type %v", ut)
		if err := cache.DeleteMemcache(ctx, cachedTagsKey); err != nil {
			errs = append(errs, err.Error())
		}
		if err := cache.DeleteDatastore(ctx, cachedTagsDatastoreKey(ctx)); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) != 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// FlushCache deletes all cached queries and tags from t.
func FlushCache(ctx context.Context, t cache.Type) error {
	switch t {
	case cache.Memcache:
		for _, key := range []string{cachedQueriesKey, cachedTagsKey} {
			if err := cache.DeleteMemcache(ctx, key); err != nil {
				return err
			}
		}
		return nil
	case cache.Datastore:
		for _, key := range []*datastore.Key{
			cachedQueriesDatastoreKey(ctx),
			cachedTagsDatastoreKey(ctx),
		} {
			if err := cache.DeleteDatastore(ctx, key); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("invalid type %v", t)
	}
}
