package appengine

import (
	"appengine"
	"appengine/memcache"
	"encoding/json"
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

func getSongsFromCache(c appengine.Context, ids []int64) map[int64]nup.Song {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = getSongCacheKey(id)
	}

	// Uh, no memcache.Codec.GetMulti()?
	songs := make(map[int64]nup.Song)
	items, err := memcache.GetMulti(c, keys)
	if err != nil {
		c.Errorf("Cache query for %d song(s) failed: %v", len(keys), err)
		return songs
	}

	for idStr, item := range items {
		if !strings.HasPrefix(idStr, songCachePrefix) {
			c.Errorf("Got unexpected key %q from cache", idStr)
			continue
		}
		id, err := strconv.ParseInt(idStr[len(songCachePrefix):], 10, 64)
		if err != nil {
			c.Errorf("Failed to parse key %q from cache: %v", idStr, err)
			continue
		}
		s := nup.Song{}
		if err = json.Unmarshal(item.Value, &s); err != nil {
			c.Errorf("Failed to unmarshal cached song %v: %v", id, err)
			continue
		}
		songs[id] = s
	}
	return songs
}

func writeSongsToCache(c appengine.Context, ids []int64, songs []nup.Song) {
	if len(ids) != len(songs) {
		c.Errorf("Got request to write %v ID(s) with %v song(s) to cache", len(ids), len(songs))
		return
	}

	items := make([]*memcache.Item, len(songs))
	for i, id := range ids {
		items[i] = &memcache.Item{Key: getSongCacheKey(id), Object: &songs[i]}
	}
	codec := memcache.Codec{Marshal: json.Marshal, Unmarshal: json.Unmarshal}
	if err := codec.SetMulti(c, items); err != nil {
		c.Errorf("Failed to write %v song(s) to cache: %v", len(items), err)
	}
}
