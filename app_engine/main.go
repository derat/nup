package appengine

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"appengine"
	"appengine/user"

	"erat.org/nup"
)

const (
	// Path to the index file.
	indexPath = "index.html"

	// Default and maximum size of a batch of dumped entities.
	defaultDumpBatchSize = 100
	maxDumpBatchSize     = 5000
)

// TODO: This is swiped from https://code.google.com/p/go/source/detail?r=5e03333d2dcf.
// Switch to the version in net/http once it makes its way into App Engine.

// basicAuth returns the username and password provided in the request's
// Authorization header, if the request uses HTTP Basic Authentication.
// See RFC 2617, Section 2.
func basicAuth(r *http.Request) (username, password string, ok bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return
	}

	// "Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ==" returns ("Aladdin", "open sesame", true).
	if !strings.HasPrefix(auth, "Basic ") {
		return
	}
	c, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:], true
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

func writeTextResponse(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(s))
}

func hasAllowedGoogleAuth(c appengine.Context, r *http.Request, cfg *nup.ServerConfig) (email string, allowed bool) {
	u := user.Current(c)
	if u == nil {
		return "", false
	}

	for _, e := range cfg.GoogleUsers {
		if u.Email == e {
			return u.Email, true
		}
	}
	return u.Email, false
}

func hasAllowedBasicAuth(r *http.Request, cfg *nup.ServerConfig) (username string, allowed bool) {
	username, password, ok := basicAuth(r)
	if !ok {
		return "", false
	}

	for _, u := range cfg.BasicAuthUsers {
		if username == u.Username && password == u.Password {
			return username, true
		}
	}
	return username, false
}

func hasWebdriverCookie(r *http.Request) bool {
	if _, err := r.Cookie("webdriver"); err != nil {
		return false
	}
	return true
}

func checkRequest(c appengine.Context, w http.ResponseWriter, r *http.Request, method string, redirectToLogin bool) bool {
	cfg := getConfig(c)
	username, allowed := hasAllowedGoogleAuth(c, r, cfg)
	if !allowed && len(username) == 0 {
		username, allowed = hasAllowedBasicAuth(r, cfg)
	}
	// Ugly hack since Webdriver doesn't support basic auth.
	if !allowed && appengine.IsDevAppServer() && hasWebdriverCookie(r) {
		allowed = true
	}
	if !allowed {
		if len(username) == 0 && redirectToLogin {
			loginUrl, _ := user.LoginURL(c, "/")
			c.Debugf("Unauthenticated request for %v from %v; redirecting to login", r.URL.String(), r.RemoteAddr)
			http.Redirect(w, r, loginUrl, http.StatusFound)
		} else {
			c.Debugf("Unauthorized request for %v from %q at %v", r.URL.String(), username, r.RemoteAddr)
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
	loadBaseConfig()
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/clear", handleClear)
	http.HandleFunc("/config", handleConfig)
	http.HandleFunc("/delete_song", handleDeleteSong)
	http.HandleFunc("/dump_song", handleDumpSong)
	http.HandleFunc("/export", handleExport)
	http.HandleFunc("/flush_cache", handleFlushCache)
	http.HandleFunc("/import", handleImport)
	http.HandleFunc("/list_tags", handleListTags)
	http.HandleFunc("/now_nsec", handleNowNsec)
	http.HandleFunc("/query", handleQuery)
	http.HandleFunc("/rate_and_tag", handleRateAndTag)
	http.HandleFunc("/report_played", handleReportPlayed)
	http.HandleFunc("/songs", handleSongs)
}

func handleClear(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "POST", false) {
		return
	}
	if !appengine.IsDevAppServer() {
		http.Error(w, "Only works on dev server", http.StatusBadRequest)
		return
	}
	if err := clearData(c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTextResponse(w, "ok")
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "POST", false) {
		return
	}
	if !appengine.IsDevAppServer() {
		http.Error(w, "Only works on dev server", http.StatusBadRequest)
		return
	}

	cfg := nup.ServerConfig{}
	if err := json.NewDecoder(r.Body).Decode(&cfg); err == nil {
		addTestUserToConfig(&cfg)
		saveTestConfig(c, &cfg)
	} else if err == io.EOF {
		clearTestConfig(c)
	} else {
		c.Errorf("Failed to decode config: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeTextResponse(w, "ok")
}

func handleDeleteSong(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	c.Debugf("Got request: %v", r.URL.String())
	if !checkRequest(c, w, r, "POST", false) {
		return
	}

	var id int64
	if !parseIntParam(c, w, r, "songId", &id) {
		return
	}
	if err := deleteSong(c, id); err != nil {
		c.Errorf("Got error while deleting song: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	writeTextResponse(w, "ok")
}

func handleDumpSong(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "GET", false) {
		return
	}

	var id int64
	if !parseIntParam(c, w, r, "id", &id) {
		return
	}

	var s *nup.Song
	var err error
	if r.FormValue("cache") == "1" {
		var songs map[int64]nup.Song
		if songs, err = getSongsFromCache(c, []int64{id}); err == nil {
			if song, ok := songs[id]; ok {
				s = &song
				s.SongId = strconv.FormatInt(id, 10)
			} else {
				err = fmt.Errorf("Song %v not cached", id)
			}
		}
	} else {
		s, err = dumpSingleSong(c, id)
	}

	if err != nil {
		c.Errorf("Dumping song %v failed: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b, err := json.Marshal(s)
	if err != nil {
		c.Errorf("Marshaling song %v failed: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var out bytes.Buffer
	json.Indent(&out, b, "", "  ")
	w.Header().Set("Content-Type", "text/plain")
	out.WriteTo(w)
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "GET", false) {
		return
	}

	includeCovers := false
	if r.FormValue("covers") == "1" {
		includeCovers = true
	}

	var max int64 = defaultDumpBatchSize
	if len(r.FormValue("max")) > 0 && !parseIntParam(c, w, r, "max", &max) {
		return
	}
	if max > maxDumpBatchSize {
		max = maxDumpBatchSize
	}

	w.Header().Set("Content-Type", "text/plain")
	e := json.NewEncoder(w)

	var objectPtrs []interface{}
	var nextCursor string
	var err error

	switch r.FormValue("type") {
	case "song":
		var songs []nup.Song
		songs, nextCursor, err = dumpSongs(c, max, r.FormValue("cursor"), includeCovers)
		if err != nil {
			c.Errorf("Dumping songs failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		objectPtrs = make([]interface{}, len(songs))
		for i := range songs {
			objectPtrs[i] = &songs[i]
		}
	case "play":
		var plays []nup.PlayDump
		plays, nextCursor, err = dumpPlays(c, max, r.FormValue("cursor"))
		if err != nil {
			c.Errorf("Dumping plays failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		objectPtrs = make([]interface{}, len(plays))
		for i := range plays {
			objectPtrs[i] = &plays[i]
		}
	default:
		http.Error(w, "Invalid type", http.StatusBadRequest)
		return
	}

	for i := 0; i < len(objectPtrs); i++ {
		if err = e.Encode(objectPtrs[i]); err != nil {
			c.Errorf("Encoding object failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if len(nextCursor) > 0 {
		if err = e.Encode(nextCursor); err != nil {
			c.Errorf("Encoding cursor failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func handleFlushCache(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "POST", false) {
		return
	}
	if !appengine.IsDevAppServer() {
		http.Error(w, "Only works on dev server", http.StatusBadRequest)
		return
	}
	if err := flushCache(c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTextResponse(w, "ok")
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "GET", true) {
		return
	}

	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	f, err := os.Open(indexPath)
	if err != nil {
		c.Errorf("Failed to open %v: %v", indexPath, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "text/html")
	if _, err = io.Copy(w, f); err != nil {
		c.Errorf("Failed to copy %v to response: %v", indexPath, err)
	}
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "POST", false) {
		return
	}

	var updateDelayNsec int64 = 0
	if len(r.FormValue("updateDelayNsec")) > 0 && !parseIntParam(c, w, r, "updateDelayNsec", &updateDelayNsec) {
		return
	}
	updateDelay := time.Nanosecond * time.Duration(updateDelayNsec)

	numSongs := 0
	replaceUserData := r.FormValue("replaceUserData") == "1"
	d := json.NewDecoder(r.Body)
	for true {
		s := &nup.Song{}
		if err := d.Decode(s); err == io.EOF {
			break
		} else if err != nil {
			c.Errorf("Failed to decode song: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := updateOrInsertSong(c, s, replaceUserData, updateDelay); err != nil {
			c.Errorf("Failed to update song with SHA1 %v: %v", s.Sha1, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		numSongs++
	}
	if err := flushDataFromCacheForUpdate(c, metadataUpdate); err != nil {
		c.Errorf("Failed to flush cached queries: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	c.Debugf("Updated %v song(s)", numSongs)
	writeTextResponse(w, "ok")
}

func handleListTags(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "GET", false) {
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

func handleNowNsec(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "GET", false) {
		return
	}
	writeTextResponse(w, strconv.FormatInt(time.Now().UnixNano(), 10))
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "GET", false) {
		return
	}

	cacheOnly := r.FormValue("cacheOnly") == "1"

	q := &songQuery{}
	q.Artist = r.FormValue("artist")
	q.Title = r.FormValue("title")
	q.Album = r.FormValue("album")
	q.Keywords = strings.Fields(r.FormValue("keywords"))
	q.Shuffle = r.FormValue("shuffle") == "1"

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

	if len(r.FormValue("minFirstPlayed")) > 0 {
		var s float64
		if !parseFloatParam(c, w, r, "minFirstPlayed", &s) {
			return
		}
		q.MinFirstStartTime = nup.SecondsToTime(s)
	}
	if len(r.FormValue("maxLastPlayed")) > 0 {
		var s float64
		if !parseFloatParam(c, w, r, "maxLastPlayed", &s) {
			return
		}
		q.MaxLastStartTime = nup.SecondsToTime(s)
	}

	q.Tags = make([]string, 0)
	q.NotTags = make([]string, 0)
	for _, t := range strings.Fields(r.FormValue("tags")) {
		if t[0] == '-' {
			q.NotTags = append(q.NotTags, t[1:len(t)])
		} else {
			q.Tags = append(q.Tags, t)
		}
	}

	songs, err := getSongsForQuery(c, q, cacheOnly)
	if err != nil {
		c.Errorf("Unable to query songs: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJsonResponse(w, songs)
}

func handleRateAndTag(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "POST", false) {
		return
	}
	var id int64
	if !parseIntParam(c, w, r, "songId", &id) {
		return
	}

	var updateDelayNsec int64 = 0
	if len(r.FormValue("updateDelayNsec")) > 0 && !parseIntParam(c, w, r, "updateDelayNsec", &updateDelayNsec) {
		return
	}
	updateDelay := time.Nanosecond * time.Duration(updateDelayNsec)

	hasRating := false
	var rating float64
	var tags []string

	if _, ok := r.Form["rating"]; ok {
		if !parseFloatParam(c, w, r, "rating", &rating) {
			return
		}
		hasRating = true
		if rating < 0.0 {
			rating = -1.0
		} else if rating > 1.0 {
			rating = 1.0
		}
	}
	if _, ok := r.Form["tags"]; ok {
		tags = strings.Fields(r.FormValue("tags"))
	}
	if !hasRating && tags == nil {
		http.Error(w, "No rating or tags supplied", http.StatusBadRequest)
		return
	}

	cfg := getConfig(c)
	if cfg.ForceUpdateFailures && appengine.IsDevAppServer() {
		http.Error(w, "Returning an error, as requested", http.StatusInternalServerError)
		return
	}

	if err := updateRatingAndTags(c, id, hasRating, rating, tags, updateDelay); err != nil {
		c.Errorf("Got error while rating/tagging song: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTextResponse(w, "ok")
}

func handleReportPlayed(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	c.Debugf("Got request: %v", r.URL.String())
	if !checkRequest(c, w, r, "POST", false) {
		return
	}

	var id int64
	var startTimeFloat float64
	if !parseIntParam(c, w, r, "songId", &id) || !parseFloatParam(c, w, r, "startTime", &startTimeFloat) {
		return
	}
	startTime := nup.SecondsToTime(startTimeFloat)

	cfg := getConfig(c)
	if cfg.ForceUpdateFailures && appengine.IsDevAppServer() {
		http.Error(w, "Returning an error, as requested", http.StatusInternalServerError)
		return
	}

	if err := addPlay(c, id, startTime, r.RemoteAddr); err != nil {
		c.Errorf("Got error while recording play: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	writeTextResponse(w, "ok")
}

func handleSongs(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !checkRequest(c, w, r, "GET", false) {
		return
	}

	var max int64 = defaultDumpBatchSize
	if len(r.FormValue("max")) > 0 && !parseIntParam(c, w, r, "max", &max) {
		return
	}
	if max > maxDumpBatchSize {
		max = maxDumpBatchSize
	}

	var minLastModified time.Time
	if len(r.FormValue("minLastModifiedNsec")) > 0 {
		var ns int64
		if !parseIntParam(c, w, r, "minLastModifiedNsec", &ns) {
			return
		}
		if ns > 0 {
			minLastModified = time.Unix(0, ns)
		}
	}

	var deleted int64
	if len(r.FormValue("deleted")) > 0 && !parseIntParam(c, w, r, "deleted", &deleted) {
		return
	}

	songs, cursor, err := dumpSongsForAndroid(c, minLastModified, deleted != 0, max, r.FormValue("cursor"))
	if err != nil {
		c.Errorf("Unable to get songs: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows := make([]interface{}, 0)
	for _, s := range songs {
		rows = append(rows, s)
	}
	if len(cursor) > 0 {
		rows = append(rows, cursor)
	}
	writeJsonResponse(w, rows)
}