package appengine

import (
	"appengine"
	"appengine/datastore"
	"appengine/memcache"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"erat.org/nup"
)

const (
	// Datastore kind for cached queries.
	cachedQueriesKind = "CachedQueries"

	// Memcache key (and also datastore ID :-/) for cached queries.
	queriesCacheKey = "queries"

	// Memcache key prefix for cached songs.
	songCachePrefix = "song-"
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

func getDatastoreCachedQueriesKey(c appengine.Context) *datastore.Key {
	return datastore.NewKey(c, cachedQueriesKind, queriesCacheKey, 0, nil)
}

func shouldCacheQuery(q *songQuery) bool {
	return !q.HasMaxPlays && q.MinFirstStartTime.IsZero() || q.MaxLastStartTime.IsZero()
}

func computeQueryHash(q *songQuery) (string, error) {
	b, err := json.Marshal(q)
	if err != nil {
		return "", err
	}
	s := sha1.Sum(b)
	return hex.EncodeToString(s[:]), nil
}

func getAllCachedQueries(c appengine.Context) (cachedQueries, error) {
	queries := make(cachedQueries)
	if getConfig(c).UseDatastoreForCachedQueries {
		eq := encodedCachedQueries{}
		if err := datastore.Get(c, getDatastoreCachedQueriesKey(c), &eq); err == nil {
			if err := json.Unmarshal(eq.Data, &queries); err != nil {
				return nil, err
			}
		} else if err != datastore.ErrNoSuchEntity {
			return nil, err
		}
	} else {
		if _, err := jsonCodec.Get(c, queriesCacheKey, &queries); err != nil && err != memcache.ErrCacheMiss {
			return nil, err
		}
	}
	return queries, nil
}

func getCachedQueryResults(c appengine.Context, query *songQuery) ([]int64, error) {
	queries, err := getAllCachedQueries(c)
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

func updateCachedQueries(c appengine.Context, f func(cachedQueries) error) error {
	queries, err := getAllCachedQueries(c)
	if err != nil {
		return err
	}

	if err := f(queries); err == ErrUnmodified {
		return nil
	} else if err != nil {
		return err
	}

	if getConfig(c).UseDatastoreForCachedQueries {
		b, err := json.Marshal(queries)
		if err != nil {
			return err
		}
		_, err = datastore.Put(c, getDatastoreCachedQueriesKey(c), &encodedCachedQueries{b})
		return err
	} else {
		return jsonCodec.Set(c, &memcache.Item{Key: queriesCacheKey, Object: &queries})
	}
}

func writeQueryResultsToCache(c appengine.Context, query *songQuery, ids []int64) error {
	return updateCachedQueries(c, func(queries cachedQueries) error {
		queryHash, err := computeQueryHash(query)
		if err != nil {
			return err
		}
		queries[queryHash] = cachedQuery{*query, ids}
		return nil
	})
}

func flushQueriesFromCacheForUpdate(c appengine.Context, updateType uint) error {
	numFlushed := 0
	if err := updateCachedQueries(c, func(queries cachedQueries) error {
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
			return ErrUnmodified
		}
		return nil
	}); err != nil {
		return err
	}
	if numFlushed > 0 {
		c.Debugf("Flushed %v cached query(s) in response to update of type %v", numFlushed, updateType)
	}
	return nil
}

func getSongCacheKey(id int64) string {
	return songCachePrefix + strconv.FormatInt(id, 10)
}

func flushSongFromCache(c appengine.Context, id int64) error {
	if err := memcache.Delete(c, getSongCacheKey(id)); err != nil && err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}

func getSongsFromCache(c appengine.Context, ids []int64) (songs map[int64]nup.Song, err error) {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = getSongCacheKey(id)
	}

	// Uh, no memcache.Codec.GetMulti()?
	songs = make(map[int64]nup.Song)
	items, err := memcache.GetMulti(c, keys)
	if err != nil {
		return nil, err
	}

	for idStr, item := range items {
		if !strings.HasPrefix(idStr, songCachePrefix) {
			return nil, fmt.Errorf("Got unexpected key %q from cache", idStr)
		}
		id, err := strconv.ParseInt(idStr[len(songCachePrefix):], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Failed to parse key %q: %v", idStr, err)
		}
		s := nup.Song{}
		if err = json.Unmarshal(item.Value, &s); err != nil {
			return nil, fmt.Errorf("Failed to unmarshal cached song %v: %v", id, err)
		}
		songs[id] = s
	}
	return songs, nil
}

func flushSongsFromCacheAfterMultiError(c appengine.Context, ids []int64, me appengine.MultiError) error {
	for i, err := range me {
		id := ids[i]
		if err == memcache.ErrNotStored {
			c.Debugf("Song %v already present in cache; flushing", id)
			if err := flushSongFromCache(c, id); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func writeSongsToCache(c appengine.Context, ids []int64, songs []nup.Song, flushIfAlreadyPresent bool) error {
	if len(ids) != len(songs) {
		return fmt.Errorf("Got request to write %v ID(s) with %v song(s) to cache", len(ids), len(songs))
	}

	items := make([]*memcache.Item, len(songs))
	for i, id := range ids {
		items[i] = &memcache.Item{Key: getSongCacheKey(id), Object: &songs[i]}
	}
	if err := jsonCodec.AddMulti(c, items); err != nil {
		// Some of the songs might've been cached in response to a query in the meantime.
		// memcache.Delete() is missing a lock duration (https://code.google.com/p/googleappengine/issues/detail?id=10983),
		// so just do the best we can and try to delete the possibly-stale cached values.
		if me, ok := err.(appengine.MultiError); ok && flushIfAlreadyPresent {
			return flushSongsFromCacheAfterMultiError(c, ids, me)
		}
		return err
	}
	return nil
}

func flushCache(c appengine.Context) error {
	if err := memcache.Flush(c); err != nil {
		return err
	}
	if err := datastore.Delete(c, getDatastoreCachedQueriesKey(c)); err != nil && err != datastore.ErrNoSuchEntity {
		return err
	}
	return nil
}
