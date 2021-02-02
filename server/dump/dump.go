// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package dump loads data from datastore so it can be dumped to clients.
package dump

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/derat/nup/cloudutil"
	"github.com/derat/nup/server/common"
	"github.com/derat/nup/types"

	"google.golang.org/appengine/datastore"
)

const (
	keyProperty         = "__key__"
	maxPlaysForSongDump = 256
)

// Songs returns songs from datastore.
// max contains the maximum number of songs to return in this call.
// If cursor is non-empty, it is used to resume an already-started query.
// If includeCovers is true, the Song.CoverFilename field is preserved.
func Songs(ctx context.Context, max int64, cursor string, includeCovers bool) (
	songs []types.Song, nextCursor string, err error) {
	songs = make([]types.Song, max)
	songPtrs := make([]interface{}, max)
	for i := range songs {
		songPtrs[i] = &songs[i]
	}

	ids, _, nextCursor, err := getEntities(
		ctx, datastore.NewQuery(common.SongKind).Order(keyProperty), cursor, songPtrs)
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

// Plays returns plays from datastore.
// max contains the maximum number of plays to return in this call.
// If cursor is non-empty, it is used to resume an already-started query.
func Plays(ctx context.Context, max int64, cursor string) (
	plays []types.PlayDump, nextCursor string, err error) {
	plays = make([]types.PlayDump, max)
	playPtrs := make([]interface{}, max)
	for i := range plays {
		playPtrs[i] = &plays[i].Play
	}

	_, pids, nextCursor, err := getEntities(
		ctx, datastore.NewQuery(common.PlayKind).Order(keyProperty), cursor, playPtrs)
	if err != nil {
		return nil, "", err
	}

	plays = plays[0:len(pids)]
	for i, pid := range pids {
		plays[i].SongID = strconv.FormatInt(pid, 10)
	}
	return plays, nextCursor, nil
}

// SongsAndroid returns newly-updated songs from datastore for the Android client.
// If deleted is true, deleted songs will be returned instead.
func SongsAndroid(ctx context.Context, minLastModified time.Time, deleted bool, max int64, cursor string) (
	songs []types.Song, nextCursor string, err error) {
	songs = make([]types.Song, max)
	songPtrs := make([]interface{}, max)
	for i := range songs {
		songPtrs[i] = &songs[i]
	}

	kind := common.SongKind
	if deleted {
		kind = common.DeletedSongKind
	}

	ids, _, nextCursor, err := getEntities(ctx, datastore.NewQuery(kind).Filter(
		"LastModifiedTime >= ", minLastModified), cursor, songPtrs)
	if err != nil {
		return nil, "", err
	}

	cfg := common.Config(ctx)
	songs = songs[0:len(ids)]
	for i, id := range ids {
		common.PrepareSongForClient(&songs[i], id, cfg, cloudutil.AndroidClient)
	}
	return songs, nextCursor, nil
}

// SingleSong returns the song identified by id.
func SingleSong(ctx context.Context, id int64) (*types.Song, error) {
	sk := datastore.NewKey(ctx, common.SongKind, "", id, nil)
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
	pids, _, _, err := getEntities(ctx, datastore.NewQuery(common.PlayKind).Ancestor(sk), "", playPtrs)
	if err != nil {
		return nil, err
	}
	for i := range pids {
		s.Plays = append(s.Plays, plays[i].Play)
	}
	sort.Sort(types.PlayArray(s.Plays))

	return s, nil
}

func getEntities(ctx context.Context, q *datastore.Query, cursor string, entities []interface{}) (
	ids, parentIDs []int64, nextCursor string, err error) {
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
	parentIDs = make([]int64, 0, len(entities))

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
		parentIDs = append(parentIDs, pid)

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
	return ids, parentIDs, nextCursor, nil
}
