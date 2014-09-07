package nup

import (
	"appengine"
	"appengine/user"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erat.org/cloud"
	"erat.org/nup"
)

const (
	// Config file path relative to base app directory.
	configFile = "nup/config.json"

	// Datastore kinds of various objects.
	playKind = "Play"
	songKind = "Song"
)

var cfg struct {
	// Email addresses of users allowed to use the server.
	AllowedUsers []string

	// Base URLs for song and cover files.
	// These should be something like "https://storage.cloud.google.com/my-bucket-name/".
	BaseSongUrl  string
	BaseCoverUrl string
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

func checkUserRequest(c appengine.Context, w http.ResponseWriter, r *http.Request, method string, redirectToLogin bool) bool {
	u := user.Current(c)
	allowed := false
	for _, au := range cfg.AllowedUsers {
		if u != nil && u.Email == au {
			allowed = true
			break
		}
	}

	if !allowed {
		if u == nil {
			c.Debugf("Unauthorized request for %v from unauthenticated user at %v", r.URL.String(), r.RemoteAddr)
		} else {
			c.Debugf("Unauthorized request for %v from %v at %v", r.URL.String(), u.Email, r.RemoteAddr)
		}
		if redirectToLogin {
			loginUrl, _ := user.LoginURL(c, "/")
			http.Redirect(w, r, loginUrl, http.StatusFound)
		} else {
			http.Error(w, "Request requires authorization", http.StatusUnauthorized)
		}
		return false
	}

	if r.Method != method {
		c.Debugf("Invalid %v request for %v (expected %v)", r.Method, r.URL.String(), method)
		w.Header().Set("Allow", method)
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return false
	}

	return true
}

func checkOAuthRequest(c appengine.Context, w http.ResponseWriter, r *http.Request) bool {
	u, err := user.CurrentOAuth(c, "")
	if err != nil {
		c.Debugf("Missing OAuth Authorization header in request for %v by %v", r.URL.String(), r.RemoteAddr)
		http.Error(w, "OAuth Authorization header required", http.StatusUnauthorized)
		return false
	}
	if !appengine.IsDevAppServer() && !u.Admin {
		c.Debugf("Non-admin OAuth request for %v from %v at %v", r.URL.String(), u, r.RemoteAddr)
		http.Error(w, "Admin access only", http.StatusUnauthorized)
		return false
	}
	return true
}

func parseIntParam(c appengine.Context, w http.ResponseWriter, r *http.Request, name string, v *int64) bool {
	val, err := strconv.ParseInt(r.FormValue(name), 10, 64)
	if err != nil {
		c.Errorf("Unable to parse %v param %q", name, r.FormValue(name))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	*v = val
	return true
}

func parseFloatParam(c appengine.Context, w http.ResponseWriter, r *http.Request, name string, v *float64) bool {
	val, err := strconv.ParseFloat(r.FormValue(name), 64)
	if err != nil {
		c.Errorf("Unable to parse %v param %q", name, r.FormValue(name))
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
	c := appengine.NewContext(r)
	if !checkUserRequest(c, w, r, "GET", true) {
		return
	}
	http.Redirect(w, r, "/static/index.html", http.StatusFound)
}

func handleContents(w http.ResponseWriter, r *http.Request) {
}

func handleListTags(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkUserRequest(c, w, r, "GET", false) {
		return
	}
	tags, err := getTags(c)
	if err != nil {
		c.Errorf("Unable to query tags: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJsonResponse(w, tags)
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkUserRequest(c, w, r, "GET", false) {
		return
	}

	q := &songQuery{}
	q.Artist = r.FormValue("artist")
	q.Title = r.FormValue("title")
	q.Album = r.FormValue("album")
	q.Keywords = strings.Fields(r.FormValue("keywords"))

	if r.FormValue("firstTrack") == "1" {
		q.Track = 1
		q.Disc = 1
	}

	if len(r.FormValue("minRating")) > 0 {
		if !parseFloatParam(c, w, r, "minRating", &q.MinRating) {
			return
		}
		q.HasMinRating = true
	} else if r.FormValue("unrated") == "1" {
		q.Unrated = true
	}

	if len(r.FormValue("maxPlays")) > 0 {
		if !parseIntParam(c, w, r, "maxPlays", &q.MaxPlays) {
			return
		}
		q.HasMaxPlays = true
	}

	if len(r.FormValue("firstPlayed")) > 0 {
		var s int64
		if !parseIntParam(c, w, r, "firstPlayed", &s) {
			return
		}
		q.MinFirstStartTime = time.Now().Add(time.Duration(-s) * time.Second)
	}
	if len(r.FormValue("lastPlayed")) > 0 {
		var s int64
		if !parseIntParam(c, w, r, "lastPlayed", &s) {
			return
		}
		q.MaxLastStartTime = time.Now().Add(time.Duration(-s) * time.Second)
	}

	for _, t := range strings.Fields(r.FormValue("tags")) {
		if t[0] == '-' {
			q.NotTags = append(q.NotTags, t[1:len(t)-1])
		} else {
			q.Tags = append(q.Tags, t)
		}
	}

	songs, err := doQuery(c, q, cfg.BaseSongUrl, cfg.BaseCoverUrl)
	if err != nil {
		c.Errorf("Unable to query songs: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJsonResponse(w, songs)
}

func handleRate(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkUserRequest(c, w, r, "POST", false) {
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
	if !checkOAuthRequest(c, w, r) {
		return
	}

	updatedSongs := make([]nup.Song, 0, 0)
	if err := json.Unmarshal([]byte(r.PostFormValue("songs")), &updatedSongs); err != nil {
		c.Errorf("Unable to decode songs from update request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	replaceUserData := r.FormValue("replace") == "1"
	c.Debugf("Got %v song(s) to update", len(updatedSongs))

	for _, updatedSong := range updatedSongs {
		if err := updateOrInsertSong(c, &updatedSong, replaceUserData); err != nil {
			c.Errorf("Got error while updating song: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
