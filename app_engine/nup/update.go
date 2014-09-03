package nup

import (
	"appengine"
	"appengine/datastore"
	"fmt"
	"strings"
	"time"
	"unicode"

	"erat.org/nup"
)

const (
	playKind = "Play"
	songKind = "Song"
)

func copySongFileFields(dest, src *nup.Song) {
	dest.Sha1 = src.Sha1
	dest.Filename = src.Filename
	dest.Artist = src.Artist
	dest.Title = src.Title
	dest.Album = src.Album
	dest.Track = src.Track
	dest.Disc = src.Disc
	dest.LengthMs = src.LengthMs

	dest.ArtistLower = strings.ToLower(src.Artist)
	dest.TitleLower = strings.ToLower(src.Title)
	dest.AlbumLower = strings.ToLower(src.Album)

	keywords := make(map[string]bool)
	for _, s := range []string{dest.ArtistLower, dest.TitleLower, dest.AlbumLower} {
		for _, w := range strings.FieldsFunc(s, func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsNumber(c) }) {
			keywords[w] = true
		}
	}
	dest.Keywords = make([]string, len(keywords))
	i := 0
	for w := range keywords {
		dest.Keywords[i] = w
		i++
	}
}

func copySongUserFields(dest, src *nup.Song) {
	dest.Rating = src.Rating
	dest.FirstStartTime = src.FirstStartTime
	dest.LastStartTime = src.LastStartTime
	dest.NumPlays = src.NumPlays
	dest.Tags = src.Tags
}

func replacePlays(c appengine.Context, songKey *datastore.Key, plays *[]nup.Play) error {
	playKeys, err := datastore.NewQuery(playKind).Ancestor(songKey).KeysOnly().GetAll(c, nil)
	if err != nil {
		return err
	}
	if err = datastore.DeleteMulti(c, playKeys); err != nil {
		return err
	}

	playKeys = make([]*datastore.Key, len(*plays))
	playPtrs := make([]*nup.Play, len(*plays))
	for i, p := range *plays {
		playKeys[i] = datastore.NewIncompleteKey(c, playKind, songKey)
		playPtrs[i] = &p
	}
	if _, err = datastore.PutMulti(c, playKeys, playPtrs); err != nil {
		return err
	}
	return nil
}

func updateSong(c appengine.Context, updatedSong *nup.Song, replaceUserData bool) error {
	sha1 := updatedSong.Sha1
	queryKeys, err := datastore.NewQuery(songKind).KeysOnly().Filter("Sha1 =", sha1).GetAll(c, nil)
	if err != nil {
		return fmt.Errorf("Querying for SHA1 %v failed: %v", sha1, err)
	}
	if len(queryKeys) > 1 {
		return fmt.Errorf("Found %v songs with SHA1 %v; expected 0 or 1", sha1)
	}

	return datastore.RunInTransaction(c, func(c appengine.Context) error {
		var key *datastore.Key
		song := nup.Song{}
		if len(queryKeys) == 1 {
			c.Debugf("Updating %v with SHA1 %v", updatedSong.Filename, sha1)
			key = queryKeys[0]
			if !replaceUserData {
				if err := datastore.Get(c, key, &song); err != nil {
					return fmt.Errorf("Getting %v failed: %v", sha1, key.IntID(), err)
				}
			}
		} else {
			c.Debugf("Inserting %v with SHA1 %v", updatedSong.Filename, sha1)
			key = datastore.NewIncompleteKey(c, songKind, nil)
		}

		copySongFileFields(&song, updatedSong)
		if replaceUserData {
			copySongUserFields(&song, updatedSong)
		}
		song.LastModifiedTime = time.Now()
		key, err = datastore.Put(c, key, &song)
		if err != nil {
			return fmt.Errorf("Putting %v failed: %v", key.IntID(), err)
		}
		c.Debugf("Put song with key %v", key.IntID())

		if replaceUserData {
			if err = replacePlays(c, key, &updatedSong.Plays); err != nil {
				return err
			}
		}
		return nil
	}, nil)
}
