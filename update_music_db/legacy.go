package main

import (
	"database/sql"

	"erat.org/nup"
	_ "github.com/mattn/go-sqlite3"
)

func importFromLegacyDb(path string) (*[]*nup.SongData, *[]*nup.ExtraSongData, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, nil, err
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT SongId, Sha1, Filename, Artist, Title, Album, DiscNumber, TrackNumber, Length, Rating, Deleted, LastModifiedUsec
		 FROM Songs
		 ORDER BY SongId ASC`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	songs := make([]*nup.SongData, 0, 0)
	extra := make([]*nup.ExtraSongData, 0, 0)
	for rows.Next() {
		var s nup.SongData
		var e nup.ExtraSongData
		var lengthSec int
		if err = rows.Scan(&s.SongId, &s.Sha1, &s.Filename, &s.Artist, &s.Title, &s.Album, &s.Disc, &s.Track,
			&lengthSec, &e.Rating, &e.Deleted, &e.LastModifiedUsec); err != nil {
			return nil, nil, err
		}
		s.LengthMs = int64(lengthSec) * 1000
		e.SongId = s.SongId

		songs = append(songs, &s)
		extra = append(extra, &e)
	}

	return &songs, &extra, nil
}
