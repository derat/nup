// Copyright 2022 Daniel Erat.
// All rights reserved.

package query

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/derat/nup/server/cache"
	"github.com/derat/nup/server/db"

	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/log"
)

// Tags returns the full set of tags present across all songs.
// It attempts to return cached data before falling back to scanning all songs.
// If songs are scanned, the resulting tags are cached.
// If requireCache is true, an error is returned if tags aren't cached.
func Tags(ctx context.Context, requireCache bool) ([]string, error) {
	var tags []string
	var err error

	// Check memcache first and then datastore.
	var cacheWriteTypes []cache.Type // caches to write to
	for _, t := range []cache.Type{cache.Memcache, cache.Datastore} {
		startTime := time.Now()
		if tags, err = getCachedTags(ctx, t); err != nil {
			log.Errorf(ctx, "Got error while getting cached tags from %v: %v", t, err)
		} else if tags == nil {
			log.Debugf(ctx, "Cache miss from %v took %v ms", t, msecSince(startTime))
			cacheWriteTypes = append(cacheWriteTypes, t)
		} else {
			log.Debugf(ctx, "Got %v cached tag(s) from %v in %v ms", len(tags), t, msecSince(startTime))
			break
		}
	}
	if tags == nil && requireCache {
		return nil, errors.New("tags not cached")
	}

	// If tags weren't cached, fall back to running a slow query across all songs.
	if tags == nil {
		startTime := time.Now()
		tagMap := make(map[string]struct{})
		it := datastore.NewQuery(db.SongKind).Project("Tags").Distinct().Run(ctx)
		for {
			var song db.Song
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
		log.Debugf(ctx, "Queried %v tag(s) from datastore in %v ms", len(tags), msecSince(startTime))
	}

	// Write the tags to any caches that didn't have them already.
	// These writes can be slow and will block the HTTP response, but callers
	// should be getting tags asynchronously anyway.
	if len(cacheWriteTypes) > 0 {
		cacheWriteDone := make(chan struct{}, len(cacheWriteTypes))
		for _, t := range cacheWriteTypes {
			go func(t cache.Type) {
				startTime := time.Now()
				if err := setCachedTags(ctx, tags, t); err != nil {
					log.Errorf(ctx, "Failed to cache tags to %v: %v", t, err)
				} else {
					log.Debugf(ctx, "Cached tags to %v in %v ms", t, msecSince(startTime))
				}
				cacheWriteDone <- struct{}{}
			}(t)
		}
		startTime := time.Now()
		for range cacheWriteTypes {
			<-cacheWriteDone
		}
		log.Debugf(ctx, "Waited %v ms for cache write(s)", msecSince(startTime))
	}

	return tags, nil
}
