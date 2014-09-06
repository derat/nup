package main

import (
	"database/sql"
	"sort"
	"time"

	"erat.org/nup"
	_ "github.com/mattn/go-sqlite3"
)

func doQuery(db *sql.DB, q string, f func(*sql.Rows) error) error {
	rows, err := db.Query(q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := f(rows); err != nil {
			return err
		}
	}
	return nil
}

func getSongsFromLegacyDb(path string, updateChan chan SongAndError) (numUpdates int, err error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// TODO: If I cared about this path, it'd be better to iterate through all three queries in parallel
	// in a goroutine and stream the songs out via the channel instead of building up a map.
	songs := make(map[int]*nup.Song)

	if err = doQuery(db, "SELECT SongId, Sha1, Filename, Artist, Title, Album, DiscNumber, TrackNumber, Length, Rating FROM Songs WHERE Deleted = 0",
		func(rows *sql.Rows) error {
			var s nup.Song
			var songId int
			if err := rows.Scan(&songId, &s.Sha1, &s.Filename, &s.Artist, &s.Title, &s.Album, &s.Disc, &s.Track, &s.Length, &s.Rating); err != nil {
				return err
			}
			s.Plays = make([]*nup.Play, 0)
			s.Tags = make([]string, 0)
			songs[songId] = &s
			return nil
		}); err != nil {
		return 0, err
	}

	if err = doQuery(db, "SELECT SongId, StartTime, IpAddress FROM PlayHistory", func(rows *sql.Rows) error {
		var songId, startTimeSec int
		var ip string
		if err := rows.Scan(&songId, &startTimeSec, &ip); err != nil {
			return err
		}
		s, ok := songs[songId]
		// If not present, it's probably deleted.
		if !ok {
			return nil
		}

		startTime := time.Unix(int64(startTimeSec), 0)
		s.Plays = append(s.Plays, &nup.Play{startTime, ip})
		return nil
	}); err != nil {
		return 0, err
	}

	if err = doQuery(db, "SELECT st.SongId, t.Name FROM SongTags st INNER JOIN Tags t ON(st.TagId = t.TagId)", func(rows *sql.Rows) error {
		var songId int
		var tag string
		if err := rows.Scan(&songId, &tag); err != nil {
			return err
		}
		s, ok := songs[songId]
		// If not present, it's probably deleted.
		if !ok {
			return nil
		}
		s.Tags = append(s.Tags, tag)
		return nil
	}); err != nil {
		return 0, err
	}

	go func() {
		keys := make([]int, len(songs), len(songs))
		i := 0
		for id := range songs {
			keys[i] = id
			i++
		}
		sort.Ints(keys)
		for _, id := range keys {
			updateChan <- SongAndError{songs[id], nil}
		}
	}()
	return len(songs), nil
}
