package nup

import (
	"appengine"
	"appengine/datastore"
	"appengine/user"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"erat.org/cloud"
	"erat.org/nup"
)

const (
	configFile = "nup/config.json"
)

var cfg struct {
	AllowedUsers []string
}

func writeJsonResponse(w http.ResponseWriter, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
}

func checkUser(c appengine.Context, w http.ResponseWriter, r *http.Request) bool {
	loginUrl, _ := user.LoginURL(c, "/")
	u := user.Current(c)
	if u == nil {
		c.Debugf("Invalid access from unauthenticated user")
		http.Redirect(w, r, loginUrl, http.StatusFound)
		return false
	}

	allowed := false
	for _, au := range cfg.AllowedUsers {
		if u.Email == au {
			allowed = true
			break
		}
	}
	if !allowed {
		c.Debugf("Invalid access from %v", u.Email)
		http.Redirect(w, r, loginUrl, http.StatusFound)
		return false
	}
	return true
}

func checkOAuth(c appengine.Context, w http.ResponseWriter) bool {
	u, err := user.CurrentOAuth(c, "")
	if err != nil {
		c.Debugf("Missing OAuth Authorization header")
		http.Error(w, "OAuth Authorization header required", http.StatusUnauthorized)
		return false
	}
	if !appengine.IsDevAppServer() && !u.Admin {
		c.Debugf("Non-admin OAuth access from %v", u)
		http.Error(w, "Admin access only", http.StatusUnauthorized)
		return false
	}
	return true
}

func parseIntParam(c appengine.Context, w http.ResponseWriter, r *http.Request, name string, v *int64) bool {
	val, err := strconv.ParseInt(r.PostFormValue(name), 10, 64)
	if err != nil {
		c.Errorf("Unable to parse %v param %q", name, r.PostFormValue(name))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	*v = val
	return true
}

func parseFloatParam(c appengine.Context, w http.ResponseWriter, r *http.Request, name string, v *float64) bool {
	val, err := strconv.ParseFloat(r.PostFormValue(name), 64)
	if err != nil {
		c.Errorf("Unable to parse %v param %q", name, r.PostFormValue(name))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	*v = val
	return true
}

func init() {
	if err := cloud.ReadJson(configFile, &cfg); err != nil {
		panic(fmt.Sprintf("Unable to read %v: %v", configFile, err))
	}
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/contents", handleContents)
	http.HandleFunc("/list_tags", handleListTags)
	http.HandleFunc("/query", handleQuery)
	http.HandleFunc("/rate", handleRate)
	http.HandleFunc("/report_played", handleReportPlayed)
	http.HandleFunc("/songs", handleSongs)
	http.HandleFunc("/tag", handleTag)
	http.HandleFunc("/update_songs", handleUpdateSongs)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/static/index.html", http.StatusFound)
}

func handleContents(w http.ResponseWriter, r *http.Request) {
}

func handleListTags(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkUser(c, w, r) {
		return
	}

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
			c.Errorf("Unable to query tags: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	res := make([]string, len(tags))
	i := 0
	for t := range tags {
		res[i] = t
		i++
	}
	writeJsonResponse(w, res)
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	/*
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	*/
}

func handleRate(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkUser(c, w, r) {
		return
	}
	var id int64
	var rating float64
	if !parseIntParam(c, w, r, "songId", &id) || !parseFloatParam(c, w, r, "rating", &rating) {
		return
	}
	c.Debugf("Got request to set song %v's rating to %v", id, rating)
	if rating < 0.0 {
		rating = -1.0
	} else if rating > 1.0 {
		rating = 1.0
	}
	if err := updateExistingSong(c, id, func(c appengine.Context, s *nup.Song) error {
		s.Rating = rating
		return nil
	}); err != nil {
		c.Errorf("Got error while rating song: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

}

func handleReportPlayed(w http.ResponseWriter, r *http.Request) {
	// create key with song id from request
	// within transaction:
	//   check if Play already exists; if so, error
	//   insert new Play
	//   get existing Song
	//   update play times, play count, and update time
	//   put Song
}

func handleSongs(w http.ResponseWriter, r *http.Request) {
}

func handleTag(w http.ResponseWriter, r *http.Request) {
	// create key with song id from request
	// within transaction:
	//   get existing Song
	//   update tags and update time
	//   put Song
}

func handleUpdateSongs(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkOAuth(c, w) {
		return
	}

	updatedSongs := make([]nup.Song, 0, 0)
	if err := json.Unmarshal([]byte(r.PostFormValue("songs")), &updatedSongs); err != nil {
		c.Errorf("Unable to decode songs from update request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	replaceUserData := r.PostFormValue("replace") == "1"
	c.Debugf("Got %v song(s)", len(updatedSongs))

	for _, updatedSong := range updatedSongs {
		if err := updateOrInsertSong(c, &updatedSong, replaceUserData); err != nil {
			c.Errorf("Got error while updating song: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}
