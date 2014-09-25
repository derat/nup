package appengine

import (
	"appengine"
	"appengine/datastore"
	"fmt"
	"sort"
	"strconv"
	"time"

	"erat.org/nup"
)

const (
	keyProperty = "__key__"

	maxPlaysForSongDump = 256
)

func dumpEntities(c appengine.Context, q *datastore.Query, cursor string, entities []interface{}) (ids, parentIds []int64, nextCursor string, err error) {
	q = q.KeysOnly()
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

	ids, _, nextCursor, err := dumpEntities(c, datastore.NewQuery(songKind).Order(keyProperty), cursor, songPtrs)
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

	_, pids, nextCursor, err := dumpEntities(c, datastore.NewQuery(playKind).Order(keyProperty), cursor, playPtrs)
	if err != nil {
		return nil, "", err
	}

	plays = plays[0:len(pids)]
	for i, pid := range pids {
		plays[i].SongId = strconv.FormatInt(pid, 10)
	}
	return plays, nextCursor, nil
}

func dumpSongsForAndroid(c appengine.Context, minLastModified time.Time, max int64, cursor, baseSongUrl, baseCoverUrl string) (songs []nup.Song, nextCursor string, err error) {
	if len(baseSongUrl) == 0 || len(baseCoverUrl) == 0 {
		return nil, "", fmt.Errorf("Invalid base song (%s) or cover (%s) URL for Android", baseSongUrl, baseCoverUrl)
	}

	songs = make([]nup.Song, max)
	songPtrs := make([]interface{}, max)
	for i := range songs {
		songPtrs[i] = &songs[i]
	}

	ids, _, nextCursor, err := dumpEntities(c, datastore.NewQuery(songKind).Filter("LastModifiedTime >= ", minLastModified), cursor, songPtrs)
	if err != nil {
		return nil, "", err
	}

	songs = songs[0:len(ids)]
	for i, id := range ids {
		prepareSongForSearchResult(&songs[i], id, baseSongUrl, baseCoverUrl)
	}
	return songs, nextCursor, nil
}

func dumpSingleSong(c appengine.Context, id int64) (*nup.Song, error) {
	sk := datastore.NewKey(c, songKind, "", id, nil)
	s := &nup.Song{}
	if err := datastore.Get(c, sk, s); err != nil {
		return nil, err
	}
	s.SongId = strconv.FormatInt(id, 10)

	plays := make([]nup.PlayDump, maxPlaysForSongDump)
	playPtrs := make([]interface{}, maxPlaysForSongDump)
	for i := range plays {
		playPtrs[i] = &plays[i].Play
	}
	pids, _, _, err := dumpEntities(c, datastore.NewQuery(playKind).Ancestor(sk), "", playPtrs)
	if err != nil {
		return nil, err
	}
	for i := range pids {
		s.Plays = append(s.Plays, plays[i].Play)
	}
	sort.Sort(nup.PlayArray(s.Plays))

	return s, nil
}
