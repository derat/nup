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
	// Maximum batch size when dumping song data.
	dumpSongBatchSize = 100

	// Maximum batch size when returning songs to Android.
	androidSongBatchSize = 100
)

func createCursor(it *datastore.Iterator) (string, error) {
	c, err := it.Cursor()
	if err != nil {
		return "", fmt.Errorf("Unable to get cursor: %v", err)
	}
	return c.String(), nil
}

func dumpSongs(c appengine.Context, songCursor, playCursor string) (songs []nup.Song, nextSongCursor, nextPlayCursor string, err error) {
	startQuery := func(kind, cursor string) (*datastore.Iterator, error) {
		q := datastore.NewQuery(kind).Order(keyProperty)
		if len(cursor) > 0 {
			c, err := datastore.DecodeCursor(cursor)
			if err != nil {
				return nil, fmt.Errorf("Unable to decode %v cursor %q: %v", kind, cursor, err)
			}
			q = q.Start(c)
		}
		return q.Run(c), nil
	}

	si, err := startQuery(songKind, songCursor)
	if err != nil {
		return
	}
	pi, err := startQuery(playKind, playCursor)
	if err != nil {
		return
	}

	nextPlayCursor = playCursor
	p := nup.Play{}
	pk, err := pi.Next(&p)
	if err != datastore.Done && err != nil {
		return
	}

	songs = make([]nup.Song, 0)

	for true {
		s := nup.Song{}
		sk, err := si.Next(&s)
		if err == datastore.Done {
			break
		} else if err != nil {
			return nil, "", "", err
		}
		s.SongId = strconv.FormatInt(sk.IntID(), 10)
		s.CoverFilename = ""
		s.Plays = make([]nup.Play, 0)

		for pk != nil && pk.Parent().IntID() == sk.IntID() {
			nextPlayCursor, err = createCursor(pi)
			if err != nil {
				return nil, "", "", err
			}
			s.Plays = append(s.Plays, p)
			if pk, err = pi.Next(&p); err == datastore.Done {
				break
			} else if err != nil {
				return nil, "", "", err
			}
		}

		songs = append(songs, s)

		if len(songs) == dumpSongBatchSize {
			nextSongCursor, err = createCursor(si)
			return songs, nextSongCursor, nextPlayCursor, nil
		}
	}

	if pk, err = pi.Next(&p); err != datastore.Done {
		err = fmt.Errorf("Have orphaned play %v for song %v", pk.IntID(), pk.Parent().IntID())
		return nil, "", "", err
	}
	return songs, "", "", nil
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
