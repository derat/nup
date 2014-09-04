package nup

import (
	"appengine"
	"appengine/datastore"
	"math"
	"sort"
	"strings"
	"time"

	"erat.org/nup"
)

const (
	MaxQueryResults = 100
)

type songQuery struct {
	Artist, Title, Album string

	MinRating    float64
	HasMinRating bool
	Unrated      bool

	MaxPlays    int64
	HasMaxPlays bool

	MinFirstStartTime time.Time
	MaxLastStartTime  time.Time

	Track, Disc int64

	Tags    []string
	NotTags []string

	Shuffle bool
}

// From https://groups.google.com/forum/#!topic/golang-nuts/tyDC4S62nPo.
type int64array []int64

func (a int64array) Len() int           { return len(a) }
func (a int64array) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a int64array) Less(i, j int) bool { return a[i] < a[j] }

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

func getQueryIds(c appengine.Context, qs []*datastore.Query) ([]*[]int64, error) {
	type idsAndError struct {
		ids *[]int64
		err error
	}
	ch := make(chan idsAndError)

	for _, q := range qs {
		go func(q *datastore.Query) {
			ids := make([]int64, 0)
			it := q.Run(c)
			for {
				if k, err := it.Next(nil); err == nil {
					ids = append(ids, k.IntID())
				} else if err == datastore.Done {
					break
				} else {
					ch <- idsAndError{nil, err}
					return
				}
			}
			sort.Sort(int64array(ids))
			ch <- idsAndError{&ids, nil}
		}(q)
	}

	res := make([]*[]int64, len(qs))
	for i := range qs {
		iae := <-ch
		if iae.err != nil {
			return nil, iae.err
		}
		res[i] = iae.ids
	}
	return res, nil
}

// intersectSortedIds returns the intersection of two sorted arrays that don't have duplicate values.
func intersectSortedIds(a, b *[]int64) *[]int64 {
	m := make([]int64, 0)
	var ai, bi int
	for ai < len(*a) && bi < len(*b) {
		av := (*a)[ai]
		bv := (*b)[bi]
		if av == bv {
			m = append(m, av)
			ai++
			bi++
		} else if av < bv {
			ai++
		} else {
			bi++
		}
	}
	return &m
}

func doQuery(c appengine.Context, query *songQuery) (*[]nup.Song, error) {
	// First, build a base query with all of the equality filters.
	bq := datastore.NewQuery(songKind).KeysOnly()
	if len(query.Artist) > 0 {
		bq = bq.Filter("ArtistLower =", strings.ToLower(query.Artist))
	}
	if len(query.Title) > 0 {
		bq = bq.Filter("TitleLower =", strings.ToLower(query.Title))
	}
	if len(query.Album) > 0 {
		bq = bq.Filter("AlbumLower =", strings.ToLower(query.Album))
	}
	if query.Unrated && !query.HasMinRating {
		bq = bq.Filter("Rating =", -1.0)
	}
	if query.Track > 0 {
		bq = bq.Filter("Track =", query.Track)
	}
	if query.Disc > 0 {
		bq = bq.Filter("Disc =", query.Disc)
	}
	if len(query.Tags) > 0 {
		for _, t := range query.Tags {
			bq = bq.Filter("Tag =", t)
		}
	}

	// Datastore doesn't allow multiple inequality filters on different properties.
	// Run a separate query in parallel for each filter and then merge the results.
	qs := make([]*datastore.Query, 0)
	if query.HasMinRating {
		qs = append(qs, bq.Filter("Rating >=", query.MinRating))
	}
	if query.HasMaxPlays {
		qs = append(qs, bq.Filter("NumPlays <=", query.MaxPlays))
	}
	if !query.MinFirstStartTime.IsZero() {
		qs = append(qs, bq.Filter("FirstStartTime >=", query.MinFirstStartTime))
	}
	if !query.MaxLastStartTime.IsZero() {
		qs = append(qs, bq.Filter("LastStartTime >=", query.MaxLastStartTime))
	}
	if len(qs) == 0 {
		qs = []*datastore.Query{bq}
	}

	// FIXME: Tags, NotTags.

	unmergedIds, err := getQueryIds(c, qs)
	if err != nil {
		return nil, err
	}
	var mergedIds *[]int64
	for i, a := range unmergedIds {
		if i == 0 {
			mergedIds = a
		} else {
			mergedIds = intersectSortedIds(mergedIds, a)
		}
	}

	// FIXME: Shuffle

	// There has to be a better way to do this.
	idsToReturn := (*mergedIds)[:int(math.Min(float64(len(*mergedIds)), float64(MaxQueryResults)))]
	keys := make([]*datastore.Key, len(idsToReturn))
	for i, id := range idsToReturn {
		keys[i] = datastore.NewKey(c, songKind, "", id, nil)
	}
	songs := make([]nup.Song, len(idsToReturn))
	if err = datastore.GetMulti(c, keys, songs); err != nil {
		return nil, err
	}
	return &songs, nil
}
