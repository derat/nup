// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package cache sets and gets data from memcache and datastore.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/derat/nup/server/types"

	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/log"
	"google.golang.org/appengine/v2/memcache"
)

// Type describes a cache type.
type Type int

const (
	Memcache Type = iota
	Datastore
)

func (t Type) String() string {
	switch t {
	case Memcache:
		return "memcache"
	case Datastore:
		return "datastore"
	default:
		return strconv.Itoa(int(t))
	}
}

const (
	queryMapKind = "CachedQueries" // datastore kind for cached query results
	queryMapKey  = "queries"       // memcache key and datastore ID for cached query results
)

// queryMapDatastoreKey returns the datastore key for caching queries.
func queryMapDatastoreKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, queryMapKind, queryMapKey, 0, nil)
}

// query holds an individual query and its cached results.
type query struct {
	Query types.SongQuery
	IDs   []int64 // song IDs
}

// queryMap holds all cached queries keyed by SongQuery.Hash().
// It implements datastore.PropertyLoadSaver.
type queryMap map[string]query

func (m *queryMap) Load(props []datastore.Property) error {
	return loadJSONProp(props, m)
}
func (m *queryMap) Save() ([]datastore.Property, error) {
	return saveJSONProp(m)
}

// loadQueryMap returns the queryMap object from t.
// If the object isn't present, both returned values are nil.
func loadQueryMap(ctx context.Context, t Type) (queryMap, error) {
	var m queryMap
	var ok bool
	var err error
	switch t {
	case Memcache:
		ok, err = getMemcache(ctx, queryMapKey, &m)
	case Datastore:
		ok, err = getDatastore(ctx, queryMapDatastoreKey(ctx), &m)
	}
	if !ok {
		return nil, err
	}
	return m, nil
}

// updateQueryMap loads queryMap from t, passes them to f, and saves
// the updated queries back to t. If f returns types.ErrUnmodified, the queries
// won't be saved.
func updateQueryMap(ctx context.Context, f func(queryMap) error, t Type) error {
	m, err := loadQueryMap(ctx, t)
	if err != nil {
		return err
	}

	if m == nil { // cache miss
		m = make(queryMap)
	}
	if err := f(m); err == types.ErrUnmodified {
		return nil
	} else if err != nil {
		return err
	}

	switch t {
	case Memcache:
		return setMemcache(ctx, queryMapKey, m)
	case Datastore:
		return setDatastore(ctx, queryMapDatastoreKey(ctx), &m)
	default:
		return fmt.Errorf("invalid type %v", t)
	}
}

// GetQuery returns cached results for q from t.
// If the query isn't cached, then a nil slice is returned.
func GetQuery(ctx context.Context, q *types.SongQuery, t Type) ([]int64, error) {
	if m, err := loadQueryMap(ctx, t); err != nil {
		return nil, err
	} else if m == nil { // cache miss
		return nil, nil
	} else if q, ok := m[q.Hash()]; ok { // query among cached queries
		return q.IDs, nil
	}
	return nil, nil // query not among cached queries
}

// SetQuery caches ids as results for query in t.
func SetQuery(ctx context.Context, q *types.SongQuery, ids []int64, t Type) error {
	return updateQueryMap(ctx, func(m queryMap) error {
		m[q.Hash()] = query{Query: *q, IDs: ids}
		return nil
	}, t)
}

// FlushForUpdate deletes the appropriate cached data for an update of the supplied types.
func FlushForUpdate(ctx context.Context, ut types.UpdateTypes) error {
	var errs []error

	for _, t := range []Type{Memcache, Datastore} {
		errs = append(errs, updateQueryMap(ctx, func(m queryMap) error {
			flushed := 0
			for h, q := range m {
				if q.Query.ResultsInvalidated(ut) {
					delete(m, h)
					flushed++
				}
			}
			if flushed == 0 {
				return types.ErrUnmodified
			}
			log.Debugf(ctx, "Flushing %v cached query(s) from %v in response to update of type %v",
				flushed, t, ut)
			return nil
		}, t))
	}

	if ut&types.TagsUpdate != 0 || ut&types.MetadataUpdate != 0 {
		log.Debugf(ctx, "Flushing cached tags in response to update of type %v", ut)
		errs = append(errs, deleteMemcache(ctx, tagListKey))
		errs = append(errs, deleteDatastore(ctx, tagListDatastoreKey(ctx)))
	}

	return joinErrors(errs)
}

const (
	tagListKey  = "tags"       // memcache key and datastore ID for cached tags
	tagListKind = "CachedTags" // datastore kind for cached tags
)

// tagListDatastoreKey returns the datastore key for caching tags.
func tagListDatastoreKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, tagListKind, tagListKey, 0, nil)
}

// tagList holds the list of tags currently in use.
// It implements datastore.PropertyLoadSaver.
type tagList []string

func (t *tagList) Load(props []datastore.Property) error {
	return loadJSONProp(props, t)
}
func (t *tagList) Save() ([]datastore.Property, error) {
	return saveJSONProp(t)
}

// GetTags attempts to get the list of in-use tags from t.
// On a cache miss, both returned values are nil.
func GetTags(ctx context.Context, t Type) ([]string, error) {
	var tags tagList
	var ok bool
	var err error
	switch t {
	case Memcache:
		ok, err = getMemcache(ctx, tagListKey, &tags)
	case Datastore:
		ok, err = getDatastore(ctx, tagListDatastoreKey(ctx), &tags)
	}
	if !ok {
		return nil, err
	}
	return tags, nil
}

// SetTags saves the list of in-use tags to t.
func SetTags(ctx context.Context, tags []string, t Type) error {
	switch t {
	case Memcache:
		return setMemcache(ctx, tagListKey, tags)
	case Datastore:
		return setDatastore(ctx, tagListDatastoreKey(ctx), (*tagList)(&tags))
	default:
		return fmt.Errorf("invalid type %v", t)
	}
}

const (
	coverKeyPrefix  = "cover"        // memcache key prefix
	coverExpiration = 24 * time.Hour // memcache expiration
)

// coverKey returns the memcache key that should be used for caching a
// cover image with the supplied filename and size (i.e. width/height).
func coverKey(fn string, size int) string {
	// TODO: Hash the filename?
	// https://godoc.org/google.golang.org/appengine/memcache#Get says that the
	// key can be at most 250 bytes.
	return fmt.Sprintf("%s-%d-%s", coverKeyPrefix, size, fn)
}

// SetCover caches a cover image with the supplied filename, requested size, and
// raw data. size should be 0 when caching the original image.
func SetCover(ctx context.Context, fn string, size int, data []byte) error {
	return memcache.Set(ctx, &memcache.Item{
		Key:        coverKey(fn, size),
		Value:      data,
		Expiration: coverExpiration,
	})
}

// GetCover attempts to look up raw data for the cover image with the supplied
// filename and size. If the image isn't present, both the returned byte slice
// and the error are nil.
func GetCover(ctx context.Context, fn string, size int) ([]byte, error) {
	item, err := memcache.Get(ctx, coverKey(fn, size))
	if err == memcache.ErrCacheMiss {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return item.Value, nil
}

// Flush deletes all cached objects from t.
func Flush(ctx context.Context, t Type) error {
	switch t {
	case Memcache:
		return memcache.Flush(ctx)
	case Datastore:
		var errs []error
		errs = append(errs, deleteDatastore(ctx, queryMapDatastoreKey(ctx)))
		errs = append(errs, deleteDatastore(ctx, tagListDatastoreKey(ctx)))
		return joinErrors(errs)
	default:
		return fmt.Errorf("invalid type %v", t)
	}
}

// jsonCodec marshals and unmarshals objects for memcache.
var jsonCodec = memcache.Codec{
	Marshal:   json.Marshal,
	Unmarshal: json.Unmarshal,
}

// getMemcache fetches an object from memcache and saves it to dst.
// If the object isn't present, ok is false and err is nil.
func getMemcache(ctx context.Context, key string, dst interface{}) (ok bool, err error) {
	if _, err := jsonCodec.Get(ctx, key, dst); err == memcache.ErrCacheMiss {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// setMemcache saves src at key in memcache.
// If the update fails, the stale object (if present) is deleted.
func setMemcache(ctx context.Context, key string, src interface{}) error {
	var errs []error
	if err := jsonCodec.Set(ctx, &memcache.Item{Key: key, Object: src}); err != nil {
		errs = append(errs, fmt.Errorf("set failed: %v", err))
		if err := deleteMemcache(ctx, key); err != nil {
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

// getDatastore fetches an object from datastore and saves it to dst.
// If the object isn't present, ok is false and err is nil.
func getDatastore(ctx context.Context, key *datastore.Key, dst interface{}) (ok bool, err error) {
	if err := datastore.Get(ctx, key, dst); err == datastore.ErrNoSuchEntity {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

// setDatastore saves src at key in datastore.
// If the update fails, the stale object (if present) is deleted.
func setDatastore(ctx context.Context, key *datastore.Key, src interface{}) error {
	var errs []error
	if _, err := datastore.Put(ctx, key, src); err != nil {
		errs = append(errs, fmt.Errorf("put failed: %v", err))
		if err := deleteDatastore(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("delete failed: %v", err))
		}
	}
	return joinErrors(errs)
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

// Datastore property name used when serializing objects to JSON.
const jsonPropName = "json"

// loadJSONProp implements datastore.PropertyLoadSaver's Load method.
func loadJSONProp(props []datastore.Property, dst interface{}) error {
	if len(props) != 1 {
		return fmt.Errorf("bad property count %v", len(props))
	}
	if props[0].Name != jsonPropName {
		return fmt.Errorf("bad property name %q", props[0].Name)
	}
	b, ok := props[0].Value.([]byte)
	if !ok {
		return errors.New("property value is not byte array")
	}
	return json.Unmarshal(b, dst)
}

// saveJSONProp implements datastore.PropertyLoadSaver's Save method.
func saveJSONProp(src interface{}) ([]datastore.Property, error) {
	b, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	return []datastore.Property{datastore.Property{
		Name:    jsonPropName,
		Value:   b,
		NoIndex: true},
	}, nil
}
