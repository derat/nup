package appengine

import (
	"appengine"
	"appengine/memcache"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"erat.org/nup"
)

const (
	songCachePrefix = "song-"
)

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
	codec := memcache.Codec{Marshal: json.Marshal, Unmarshal: json.Unmarshal}
	if err := codec.AddMulti(c, items); err != nil {
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
