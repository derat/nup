// Copyright 2022 Daniel Erat.
// All rights reserved.

package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/derat/nup/server/cache"
	"github.com/derat/nup/server/db"

	"google.golang.org/appengine/v2/datastore"
	"google.golang.org/appengine/v2/log"
)

// statsKey returns the key for the db.Stats singleton entity in datastore.
func statsKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, db.StatsKind, db.StatsKeyName, 0, nil)
}

// cachedStats wraps db.Stats and implements datastore.PropertyLoadSaver.
// This is pretty obnoxious, but it's needed since datastore doesn't support maps.
type cachedStats struct{ Stats *db.Stats }

func (s *cachedStats) Load(props []datastore.Property) error {
	return cache.LoadJSONProp(props, s)
}
func (s *cachedStats) Save() ([]datastore.Property, error) {
	return cache.SaveJSONProp(s)
}

// Get returns stats that were previously computed by the Update method.
func Get(ctx context.Context) (*db.Stats, error) {
	var stats db.Stats
	if ok, err := cache.GetMemcache(ctx, db.StatsKeyName, &stats); err != nil {
		log.Errorf(ctx, "Failed getting stats from memcache: %v", err)
	} else if ok {
		return &stats, nil
	}
	if err := datastore.Get(ctx, statsKey(ctx), &cachedStats{&stats}); err != nil {
		return nil, err
	}
	if err := cache.SetMemcache(ctx, db.StatsKeyName, &stats); err != nil {
		log.Errorf(ctx, "Failed saving stats to memcache: %v", err)
	}
	return &stats, nil
}

// Update reads all songs and plays and saves stats to datastore.
//
// This uses projection queries, which are counted as "small" datastore operations
// and are free in most (all?) regions, but it's still slow and should be called
// periodically by a cron job instead of interactively.
func Update(ctx context.Context) error {
	stats := db.NewStats()
	stats.UpdateTime = time.Now()

	songLengths := make(map[int64]float64) // keys are song IDs

	// Datastore doesn't seem to return any results when trying to project all three of these
	// properties at once (probably because Tags is array-valued), and including multiple properties
	// also requires additional indexes.
	type songFunc func(int64, *db.Song)
	songFuncs := map[string]songFunc{
		"Length": func(id int64, s *db.Song) {
			stats.Songs++
			stats.TotalSec += s.Length
			songLengths[id] = s.Length
		},
		"Rating": func(id int64, s *db.Song) {
			stats.Ratings[fmt.Sprintf("%.2f", s.Rating)]++
		},
		"Tags": func(id int64, s *db.Song) {
			for _, t := range s.Tags {
				stats.Tags[t]++
			}
		},
	}
	ch := make(chan error, len(songFuncs))
	for prop, fn := range songFuncs {
		func(prop string, fn songFunc) {
			start := time.Now()
			it := datastore.NewQuery(db.SongKind).Project(prop).Run(ctx)
			for {
				var s db.Song
				k, err := it.Next(&s)
				if err == datastore.Done {
					break
				} else if err != nil {
					ch <- fmt.Errorf("failed reading Song.%v: %v", prop, err)
					return
				}
				fn(k.IntID(), &s)
			}
			log.Debugf(ctx, "Computing Song.%v stats took %v ms",
				prop, time.Now().Sub(start).Milliseconds())
			ch <- nil
		}(prop, fn)
	}
	for i := 0; i < len(songFuncs); i++ {
		if err := <-ch; err != nil {
			return err
		}
	}

	start := time.Now()
	it := datastore.NewQuery(db.PlayKind).Project("StartTime").Run(ctx)
	for {
		var play db.Play
		key, err := it.Next(&play)
		if err == datastore.Done {
			break
		} else if err != nil {
			return err
		}

		year := play.StartTime.Local().Year()
		yearStats := stats.Years[year]
		yearStats.Plays++

		var songID int64
		if pk := key.Parent(); pk == nil {
			return fmt.Errorf("no parent key for play %v", key.IntID())
		} else {
			songID = pk.IntID()
		}
		if sec, ok := songLengths[songID]; !ok {
			return fmt.Errorf("missing song %v for play %v", songID, key.IntID())
		} else {
			yearStats.TotalSec += sec
		}

		stats.Years[year] = yearStats
	}
	log.Debugf(ctx, "Computing Play stats took %v ms", time.Now().Sub(start).Milliseconds())

	if err := cache.DeleteMemcache(ctx, db.StatsKeyName); err != nil {
		log.Errorf(ctx, "Failed deleting stats from memcache: %v", err)
	}
	_, err := datastore.Put(ctx, statsKey(ctx), &cachedStats{stats})
	return err
}

// Clear deletes previously-computed stats from datastore and memcache.
func Clear(ctx context.Context) error {
	if err := datastore.Delete(ctx, statsKey(ctx)); err != nil && err != datastore.ErrNoSuchEntity {
		return err
	}
	return cache.DeleteMemcache(ctx, db.StatsKeyName)
}
