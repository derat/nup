package main

import (
	"time"

	"erat.org/nup"
)

type databaseUpdater struct {
}

func newDatabaseUpdater() (*databaseUpdater, error) {
	u := &databaseUpdater{}
	return u, nil
}

func (u *databaseUpdater) GetLastUpdateTime() (time.Time, error) {
	return time.Now(), nil
}

func (u *databaseUpdater) SetLastUpdateTime(t time.Time) error {
	return nil
}

func (u *databaseUpdater) UpdateSongs(songs []*nup.SongData) error {
	return nil
}
