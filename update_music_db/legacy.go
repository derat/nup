package main

import (
	"database/sql"
	"fmt"

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

func importFromLegacyDb(path string) (*[]*nup.SongData, *[]*nup.ExtraSongData, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	songs := make([]*nup.SongData, 0, 0)
	extra := make([]*nup.ExtraSongData, 0, 0)

	if err = doQuery(db,
		`SELECT SongId, Sha1, Filename, Artist, Title, Album, DiscNumber, TrackNumber, Length, Rating, Deleted, LastModifiedUsec
		 FROM Songs
		 ORDER BY SongId ASC`,
		func(rows *sql.Rows) error {
			var s nup.SongData
			var e nup.ExtraSongData
			e.Tags = make([]nup.TagData, 0, 0)
			e.Plays = make([]nup.PlayData, 0, 0)

			var lengthSec int
			if err := rows.Scan(&s.SongId, &s.Sha1, &s.Filename, &s.Artist, &s.Title, &s.Album, &s.Disc, &s.Track,
				&lengthSec, &e.Rating, &e.Deleted, &e.LastModifiedUsec); err != nil {
				return err
			}
			s.LengthMs = int64(lengthSec) * 1000
			e.SongId = s.SongId

			songs = append(songs, &s)
			extra = append(extra, &e)
			return nil
		}); err != nil {
		return nil, nil, err
	}

	i := 0
	if err = doQuery(db, "SELECT SongId, StartTime, IpAddress FROM PlayHistory ORDER BY SongId ASC, StartTime ASC", func(rows *sql.Rows) error {
		var songId, startTime int
		var ip string
		if err := rows.Scan(&songId, &startTime, &ip); err != nil {
			return err
		}
		for i < len(extra) && extra[i].SongId < songId {
			i++
		}
		if i >= len(extra) || extra[i].SongId != songId {
			return fmt.Errorf("Don't have extra song data for song ID %v (dangling PlayHistory row)", songId)
		}
		extra[i].Plays = append(extra[i].Plays, nup.PlayData{startTime, ip})
		return nil
	}); err != nil {
		return nil, nil, err
	}

	i = 0
	if err = doQuery(db,
		`SELECT st.SongId, st.CreationTime, t.Name FROM SongTags st
		 INNER JOIN Tags t ON(st.TagId = t.TagId)
		 ORDER BY st.SongId ASC, t.Name ASC`,
		func(rows *sql.Rows) error {
			var songId, creationTime int
			var tagName string
			if err := rows.Scan(&songId, &creationTime, &tagName); err != nil {
				return err
			}
			for i < len(extra) && extra[i].SongId < songId {
				i++
			}
			if i >= len(extra) || extra[i].SongId != songId {
				return fmt.Errorf("Don't have extra song data for song ID %v (dangling SongTags row)", songId)
			}
			extra[i].Tags = append(extra[i].Tags, nup.TagData{tagName, creationTime})
			return nil
		}); err != nil {
		return nil, nil, err
	}

	return &songs, &extra, nil
}
