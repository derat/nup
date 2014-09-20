package appengine

import (
	"appengine"
	"appengine/datastore"
	"fmt"
	"strconv"
	"time"

	"erat.org/nup"
)

const (
	// Maximum batch size when returning songs to Android.
	androidSongBatchSize = 100

	keyProperty = "__key__"
)

func dumpEntities(c appengine.Context, q *datastore.Query, cursor string, entities []interface{}) (ids, parentIds []int64, nextCursor string, err error) {
	if len(cursor) > 0 {
		dc, err := datastore.DecodeCursor(cursor)
		if err != nil {
			return nil, nil, "", fmt.Errorf("Unable to decode cursor %q: %v", cursor, err)
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
				return nil, nil, "", fmt.Errorf("Unable to get cursor: %v", err)
			}
			nextCursor = nc.String()
			break
		}
	}

	entities = entities[0:len(keys)]
	if len(keys) > 0 {
		if err := datastore.GetMulti(c, keys, entities); err != nil {
			return nil, nil, "", fmt.Errorf("Failed to get %v entities: %v", len(keys), err)
		}
	}
	return ids, parentIds, nextCursor, nil
}

func dumpSongs(c appengine.Context, max int64, cursor string) (songs []nup.Song, nextCursor string, err error) {
	songs = make([]nup.Song, max)
	songPtrs := make([]interface{}, max)
	for i := range songs {
		songPtrs[i] = &songs[i]
	}

	q := datastore.NewQuery(songKind).KeysOnly().Order(keyProperty)
	ids, _, nextCursor, err := dumpEntities(c, q, cursor, songPtrs)
	if err != nil {
		return nil, "", err
	}

	songs = songs[0:len(ids)]
	for i, id := range ids {
		s := &songs[i]
		s.SongId = strconv.FormatInt(id, 10)
		s.CoverFilename = ""
	}
	return songs, nextCursor, nil
}

func dumpPlays(c appengine.Context, max int64, cursor string) (plays []nup.PlayDump, nextCursor string, err error) {
	plays = make([]nup.PlayDump, max)
	playPtrs := make([]interface{}, max)
	for i := range plays {
		playPtrs[i] = &plays[i].Play
	}

	q := datastore.NewQuery(playKind).KeysOnly().Order(keyProperty)
	_, pids, nextCursor, err := dumpEntities(c, q, cursor, playPtrs)
	if err != nil {
		return nil, "", err
	}

	plays = plays[0:len(pids)]
	for i, pid := range pids {
		plays[i].SongId = strconv.FormatInt(pid, 10)
	}
	return plays, nextCursor, nil
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
