package appengine

import (
	"appengine"
	"appengine/datastore"
	"fmt"
	"time"

	"erat.org/nup"
)

const (
	// Maximum batch size when returning songs to Android.
	androidSongBatchSize = 100

	keyProperty = "__key__"
)

func dumpEntities(c appengine.Context, kind string, entities []interface{}, cursor string) (ids, parentIds []int64, nextCursor string, err error) {
	q := datastore.NewQuery(kind).KeysOnly().Order(keyProperty)
	if len(cursor) > 0 {
		dc, err := datastore.DecodeCursor(cursor)
		if err != nil {
			return nil, nil, "", fmt.Errorf("Unable to decode %v cursor %q: %v", kind, cursor, err)
		}
		q = q.Start(dc)
	}
	it := q.Run(c)

	keys := make([]*datastore.Key, 0, len(entities))
	ids = make([]int64, 0, len(entities))
	parentIds = make([]int64, 0, len(entities))

	for true {
		k, err := it.Next(nil)
		if err == datastore.Done {
			break
		} else if err != nil {
			return nil, nil, "", err
		}

		keys = append(keys, k)
		ids = append(ids, k.IntID())

		var pid int64
		if pk := k.Parent(); pk != nil {
			pid = pk.IntID()
		}
		parentIds = append(parentIds, pid)

		if len(keys) == len(entities) {
			nc, err := it.Cursor()
			if err != nil {
				return nil, nil, "", fmt.Errorf("Unable to get %v cursor: %v", kind, err)
			}
			nextCursor = nc.String()
			break
		}
	}

	entities = entities[0:len(keys)]
	if err := datastore.GetMulti(c, keys, entities); err != nil {
		return nil, nil, "", fmt.Errorf("Failed to get %v %v entities: %v", len(keys), kind, err)
	}
	return ids, parentIds, nextCursor, nil
}

func getSongsForAndroid(c appengine.Context, minLastModified time.Time, startCursor, baseSongUrl, baseCoverUrl string) (songs []nup.Song, nextCursor string, err error) {
	q := datastore.NewQuery(songKind).Filter("LastModifiedTime >= ", minLastModified)
	if len(startCursor) > 0 {
		sc, err := datastore.DecodeCursor(startCursor)
		if err != nil {
			return nil, "", fmt.Errorf("Unable to decode cursor %q: %v", startCursor, err)
		}
		q = q.Start(sc)
	}

	it := q.Run(c)
	songs = make([]nup.Song, 0)
	for true {
		s := nup.Song{}
		sk, err := it.Next(&s)
		if err == datastore.Done {
			break
		} else if err != nil {
			return nil, "", fmt.Errorf("Unable to read song: %v", err)
		}

		prepareSongForSearchResult(&s, sk.IntID(), baseSongUrl, baseCoverUrl)
		songs = append(songs, s)

		if len(songs) == androidSongBatchSize {
			nc, err := it.Cursor()
			if err != nil {
				return nil, "", fmt.Errorf("Unable to get new cursor")
			}
			nextCursor = nc.String()
			break
		}
	}

	return songs, nextCursor, nil
}

func getMaxLastModifiedTime(c appengine.Context) (time.Time, error) {
	songs := make([]nup.Song, 0)
	if _, err := datastore.NewQuery(songKind).Order("-LastModifiedTime").Project("LastModifiedTime").Limit(1).GetAll(c, &songs); err != nil {
		return time.Time{}, err
	}
	if len(songs) == 0 {
		return time.Time{}, nil
	}
	return songs[0].LastModifiedTime, nil
}
