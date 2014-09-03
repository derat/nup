package nup

import (
	"appengine"
	"appengine/user"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"erat.org/nup"
)

// Email addresses of users allowed to use the app.
var allowedUsers map[string]bool

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
	if ok, _ := allowedUsers[u.Email]; !ok {
		c.Debugf("Invalid access from %v", u.ID)
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

func init() {
	allowedUsers = make(map[string]bool)
	for _, u := range strings.Split(os.Getenv("ALLOWED_USERS"), ",") {
		allowedUsers[u] = true
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
	tags := make([]string, 0, 0)
	/*
		if err := useDb(func(db *sql.DB) error {
			return iterateOverRows(db, "SELECT Name FROM Tags", func(r *sql.Rows) error {
				var tag string
				if err := r.Scan(&tag); err != nil {
					return err
				}
				tags = append(tags, tag)
				return nil
			})
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	*/
	writeJsonResponse(w, tags)
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	/*
		w.Header().Set("Content-Type", "application/json")
		w.Write(jsonData)
	*/
}

func handleRate(w http.ResponseWriter, r *http.Request) {
}

func handleReportPlayed(w http.ResponseWriter, r *http.Request) {
}

func handleSongs(w http.ResponseWriter, r *http.Request) {
}

func handleTag(w http.ResponseWriter, r *http.Request) {
}

func handleUpdateSongs(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkOAuth(c, w) {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	songs := make([]nup.Song, 0, 0)
	if err := json.Unmarshal([]byte(r.Form.Get("songs")), &songs); err != nil {
		c.Errorf("Unable to decode songs from update request: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	//replace := r.Form.Get("replace") == "1"
	c.Debugf("Got %v song(s)", len(songs))
}
