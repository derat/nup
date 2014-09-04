package nup

import (
	"appengine"
	"appengine/datastore"

	"erat.org/nup"
)

func getTags(c appengine.Context) (*[]string, error) {
	tags := make(map[string]bool)
	it := datastore.NewQuery(songKind).Project("Tags").Distinct().Run(c)
	for {
		song := &nup.Song{}
		if _, err := it.Next(song); err == nil {
			for _, t := range song.Tags {
				tags[t] = true
			}
		} else if err == datastore.Done {
			break
		} else {
			return nil, err
		}
	}

	res := make([]string, len(tags))
	i := 0
	for t := range tags {
		res[i] = t
		i++
	}
	return &res, nil
}
