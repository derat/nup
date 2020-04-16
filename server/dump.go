package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/derat/nup/cloudutil"
	"github.com/derat/nup/types"

	"google.golang.org/appengine/datastore"
)

const (
	keyProperty = "__key__"

	maxPlaysForSongDump = 256
)

func dumpEntities(ctx context.Context, q *datastore.Query, cursor string, entities []interface{}) (ids, parentIds []int64, nextCursor string, err error) {
	q = q.KeysOnly()
	if len(cursor) > 0 {
		dc, err := datastore.DecodeCursor(cursor)
		if err != nil {
			return nil, nil, "", fmt.Errorf("unable to decode cursor %q: %v", cursor, err)
		}
		q = q.Start(dc)
	}
	it := q.Run(ctx)

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
				return nil, nil, "", fmt.Errorf("unable to get cursor: %v", err)
			}
			nextCursor = nc.String()
			break
		}
	}

	entities = entities[0:len(keys)]
	if len(keys) > 0 {
		if err := datastore.GetMulti(ctx, keys, entities); err != nil {
			return nil, nil, "", fmt.Errorf("failed to get %v entities: %v", len(keys), err)
		}
	}
	return ids, parentIds, nextCursor, nil
}

func dumpSongs(ctx context.Context, max int64, cursor string, includeCovers bool) (songs []types.Song, nextCursor string, err error) {
	songs = make([]types.Song, max)
	songPtrs := make([]interface{}, max)
	for i := range songs {
		songPtrs[i] = &songs[i]
	}

	ids, _, nextCursor, err := dumpEntities(ctx, datastore.NewQuery(songKind).Order(keyProperty), cursor, songPtrs)
	if err != nil {
		return nil, "", err
	}

	songs = songs[0:len(ids)]
	for i, id := range ids {
		s := &songs[i]
		s.SongID = strconv.FormatInt(id, 10)
		if !includeCovers {
			s.CoverFilename = ""
		}
	}
	return songs, nextCursor, nil
}

func dumpPlays(ctx context.Context, max int64, cursor string) (plays []types.PlayDump, nextCursor string, err error) {
	plays = make([]types.PlayDump, max)
	playPtrs := make([]interface{}, max)
	for i := range plays {
		playPtrs[i] = &plays[i].Play
	}

	_, pids, nextCursor, err := dumpEntities(ctx, datastore.NewQuery(playKind).Order(keyProperty), cursor, playPtrs)
	if err != nil {
		return nil, "", err
	}

	plays = plays[0:len(pids)]
	for i, pid := range pids {
		plays[i].SongID = strconv.FormatInt(pid, 10)
	}
	return plays, nextCursor, nil
}

func dumpSongsForAndroid(ctx context.Context, minLastModified time.Time, deleted bool, max int64, cursor string) (songs []types.Song, nextCursor string, err error) {
	songs = make([]types.Song, max)
	songPtrs := make([]interface{}, max)
	for i := range songs {
		songPtrs[i] = &songs[i]
	}

	kind := songKind
	if deleted {
		kind = deletedSongKind
	}

	ids, _, nextCursor, err := dumpEntities(ctx, datastore.NewQuery(kind).Filter("LastModifiedTime >= ", minLastModified), cursor, songPtrs)
	if err != nil {
		return nil, "", err
	}

	cfg := getConfig(ctx)
	songs = songs[0:len(ids)]
	for i, id := range ids {
		prepareSongForClient(&songs[i], id, cfg, cloudutil.AndroidClient)
	}
	return songs, nextCursor, nil
}

func dumpSingleSong(ctx context.Context, id int64) (*types.Song, error) {
	sk := datastore.NewKey(ctx, songKind, "", id, nil)
	s := &types.Song{}
	if err := datastore.Get(ctx, sk, s); err != nil {
		return nil, err
	}
	s.SongID = strconv.FormatInt(id, 10)

	plays := make([]types.PlayDump, maxPlaysForSongDump)
	playPtrs := make([]interface{}, maxPlaysForSongDump)
	for i := range plays {
		playPtrs[i] = &plays[i].Play
	}
	pids, _, _, err := dumpEntities(ctx, datastore.NewQuery(playKind).Ancestor(sk), "", playPtrs)
	if err != nil {
		return nil, err
	}
	for i := range pids {
		s.Plays = append(s.Plays, plays[i].Play)
	}
	sort.Sort(types.PlayArray(s.Plays))

	return s, nil
}
