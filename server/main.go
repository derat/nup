package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/derat/nup/types"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/user"
)

const (
	indexPath = "web/index.html" // path to index relative to base dir

	defaultDumpBatchSize = 100  // default size of batch of dumped entities
	maxDumpBatchSize     = 5000 // max size of batch of dumped entities

	maxCoverSize     = 800 // max size permitted in /cover requests
	coverJPEGQuality = 90  // quality to use when encoding /cover replies
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

// writeJSONResponse serializes v to JSON and writes it to w.
func writeJSONResponse(w http.ResponseWriter, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}
}

// writeTextResponse writes s to w as a text response.
func writeTextResponse(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(s))
}

// hasAllowedGoogleAuth checks whether ctx contains credentials for a Google
// user registered in cfg.
func hasAllowedGoogleAuth(ctx context.Context, cfg *types.ServerConfig) (email string, allowed bool) {
	u := user.Current(ctx)
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

// hasAllowedBasicAuth checks whether r is authorized via HTTP basic
// authentication with a user registered in cfg. If basic auth was used, the
// username return value is set regardless of the user is allowed or not.
func hasAllowedBasicAuth(r *http.Request, cfg *types.ServerConfig) (username string, allowed bool) {
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

// hasWebDriverCookie returns true if r contains a special cookie set by browser
// tests that use WebDriver.
func hasWebDriverCookie(r *http.Request) bool {
	if _, err := r.Cookie("webdriver"); err != nil {
		return false
	}
	return true
}

// checkRequest verifies that r is an authorized request using method.
// If the request is unauthorized and redirectToLogin is true, the client
// is redirected to the login screen.
func checkRequest(ctx context.Context, w http.ResponseWriter, r *http.Request,
	method string, redirectToLogin bool) bool {
	cfg := getConfig(ctx)
	username, allowed := hasAllowedGoogleAuth(ctx, cfg)
	if !allowed && len(username) == 0 {
		username, allowed = hasAllowedBasicAuth(r, cfg)
	}
	// Ugly hack since WebDriver doesn't support basic auth.
	if !allowed && appengine.IsDevAppServer() && hasWebDriverCookie(r) {
		allowed = true
	}
	if !allowed {
		if len(username) == 0 && redirectToLogin {
			loginURL, _ := user.LoginURL(ctx, "/")
			log.Debugf(ctx, "Unauthenticated request for %v from %v; redirecting to login", r.URL.String(), r.RemoteAddr)
			http.Redirect(w, r, loginURL, http.StatusFound)
		} else {
			log.Debugf(ctx, "Unauthorized request for %v from %q at %v", r.URL.String(), username, r.RemoteAddr)
			http.Error(w, "Request requires authorization", http.StatusUnauthorized)
		}
		return false
	}

	if r.Method != method {
		log.Debugf(ctx, "Invalid %v request for %v (expected %v)", r.Method, r.URL.String(), method)
		w.Header().Set("Allow", method)
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return false
	}

	return true
}

// parseIntParam parses and returns the named int64 form parameter from r.
// If the parameter is missing or unparseable, a bad request error is written
// to w, an error is logged, and the ok return value is false.
func parseIntParam(ctx context.Context, w http.ResponseWriter, r *http.Request,
	name string) (v int64, ok bool) {
	s := r.FormValue(name)
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Errorf(ctx, "Unable to parse %v param %q", name, s)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return v, false
	}
	return v, true
}

// parseFloatParam parses and returns the float64 form parameter from r.
// If the parameter is missing or unparseable, a bad request error is written
// to w, an error is logged, and the ok return value is false.
func parseFloatParam(ctx context.Context, w http.ResponseWriter, r *http.Request,
	name string) (v float64, ok bool) {
	s := r.FormValue(name)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Errorf(ctx, "Unable to parse %v param %q", name, s)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return v, false
	}
	return v, true
}

// secondsToTime converts fractional seconds since the Unix epoch to a time.Time.
func secondsToTime(s float64) time.Time {
	return time.Unix(0, int64(s*float64(time.Second/time.Nanosecond)))
}

func main() {
	loadBaseConfig()
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/clear", handleClear)
	http.HandleFunc("/config", handleConfig)
	http.HandleFunc("/cover", handleCover)
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

	// TODO: Update to use http.ListenAndServe():
	// https://cloud.google.com/appengine/docs/standard/go111/go-differences#writing_a_main_package
	appengine.Main()
}

func handleClear(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "POST", false) {
		return
	}
	if !appengine.IsDevAppServer() {
		http.Error(w, "Only works on dev server", http.StatusBadRequest)
		return
	}
	if err := clearData(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTextResponse(w, "ok")
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "POST", false) {
		return
	}
	if !appengine.IsDevAppServer() {
		http.Error(w, "Only works on dev server", http.StatusBadRequest)
		return
	}

	cfg := types.ServerConfig{}
	if err := json.NewDecoder(r.Body).Decode(&cfg); err == nil {
		addTestUserToConfig(&cfg)
		saveTestConfig(ctx, &cfg)
	} else if err == io.EOF {
		clearTestConfig(ctx)
	} else {
		log.Errorf(ctx, "Failed to decode config: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeTextResponse(w, "ok")
}

func handleCover(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "GET", false) {
		return
	}

	fn := r.FormValue("filename")
	if fn == "" {
		log.Errorf(ctx, "Missing filename in cover request")
		http.Error(w, "Missing filename", http.StatusBadRequest)
		return
	}
	var size int64 = 140
	if r.FormValue("size") != "" {
		var ok bool
		if size, ok = parseIntParam(ctx, w, r, "size"); !ok {
			return
		} else if size <= 0 || size > maxCoverSize {
			log.Errorf(ctx, "Invalid cover size %v", size)
			http.Error(w, "Invalid size", http.StatusBadRequest)
			return
		}
	}

	// This handler is expensive, so try to minimize duplicate requests:
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control
	//
	// Note that Cache-Control is "helpfully" rewritten to "no-cache,
	// must-revalidate" in response to requests from admin users:
	// https://cloud.google.com/appengine/docs/standard/go111/reference/request-response-headers#headers_added_or_replaced
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Expires", time.Now().UTC().Add(24*time.Hour).Format(time.RFC1123))

	w.Header().Set("Content-Type", "image/jpeg")
	if err := scaleCover(ctx, fn, int(size), coverJPEGQuality, w); err != nil {
		log.Errorf(ctx, "Failed to scale cover: %v", err)
		http.Error(w, "Scaling failed", http.StatusInternalServerError)
		return
	}
}

func handleDeleteSong(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	log.Debugf(ctx, "Got request: %v", r.URL.String())
	if !checkRequest(ctx, w, r, "POST", false) {
		return
	}

	id, ok := parseIntParam(ctx, w, r, "songId")
	if !ok {
		return
	}
	if err := deleteSong(ctx, id); err != nil {
		log.Errorf(ctx, "Got error while deleting song: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	writeTextResponse(w, "ok")
}

func handleDumpSong(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "GET", false) {
		return
	}

	id, ok := parseIntParam(ctx, w, r, "id")
	if !ok {
		return
	}

	var s *types.Song
	var err error
	if r.FormValue("cache") == "1" {
		var songs map[int64]types.Song
		if getConfig(ctx).CacheSongs != types.MemcacheCaching {
			err = errors.New("song caching is disabled")
		} else {
			if songs, err = getSongsFromMemcache(ctx, []int64{id}); err == nil {
				if song, ok := songs[id]; ok {
					s = &song
					s.SongID = strconv.FormatInt(id, 10)
				} else {
					err = fmt.Errorf("song %v not cached", id)
				}
			}
		}
	} else {
		s, err = dumpSingleSong(ctx, id)
	}

	if err != nil {
		log.Errorf(ctx, "Dumping song %v failed: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	b, err := json.Marshal(s)
	if err != nil {
		log.Errorf(ctx, "Marshaling song %v failed: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var out bytes.Buffer
	json.Indent(&out, b, "", "  ")
	w.Header().Set("Content-Type", "text/plain")
	out.WriteTo(w)
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "GET", false) {
		return
	}

	includeCovers := false
	if r.FormValue("covers") == "1" {
		includeCovers = true
	}

	var max int64 = defaultDumpBatchSize
	if len(r.FormValue("max")) > 0 {
		var ok bool
		if max, ok = parseIntParam(ctx, w, r, "max"); !ok {
			return
		}
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
		var songs []types.Song
		songs, nextCursor, err = dumpSongs(ctx, max, r.FormValue("cursor"), includeCovers)
		if err != nil {
			log.Errorf(ctx, "Dumping songs failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		objectPtrs = make([]interface{}, len(songs))
		for i := range songs {
			objectPtrs[i] = &songs[i]
		}
	case "play":
		var plays []types.PlayDump
		plays, nextCursor, err = dumpPlays(ctx, max, r.FormValue("cursor"))
		if err != nil {
			log.Errorf(ctx, "Dumping plays failed: %v", err)
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
			log.Errorf(ctx, "Encoding object failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if len(nextCursor) > 0 {
		if err = e.Encode(nextCursor); err != nil {
			log.Errorf(ctx, "Encoding cursor failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

func handleFlushCache(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "POST", false) {
		return
	}
	if !appengine.IsDevAppServer() {
		http.Error(w, "Only works on dev server", http.StatusBadRequest)
		return
	}
	if err := flushCache(ctx); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTextResponse(w, "ok")
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "GET", true) {
		return
	}

	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	f, err := os.Open(indexPath)
	if err != nil {
		log.Errorf(ctx, "Failed to open %v: %v", indexPath, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "text/html")
	if _, err = io.Copy(w, f); err != nil {
		log.Errorf(ctx, "Failed to copy %v to response: %v", indexPath, err)
	}
}

func handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "POST", false) {
		return
	}

	var updateDelayNsec int64
	if len(r.FormValue("updateDelayNsec")) > 0 {
		var ok bool
		if updateDelayNsec, ok = parseIntParam(ctx, w, r, "updateDelayNsec"); !ok {
			return
		}
	}
	updateDelay := time.Nanosecond * time.Duration(updateDelayNsec)

	numSongs := 0
	replaceUserData := r.FormValue("replaceUserData") == "1"
	d := json.NewDecoder(r.Body)
	for true {
		s := &types.Song{}
		if err := d.Decode(s); err == io.EOF {
			break
		} else if err != nil {
			log.Errorf(ctx, "Failed to decode song: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := updateOrInsertSong(ctx, s, replaceUserData, updateDelay); err != nil {
			log.Errorf(ctx, "Failed to update song with SHA1 %v: %v", s.SHA1, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		numSongs++
	}
	if err := flushCacheForUpdate(ctx, metadataUpdate); err != nil {
		log.Errorf(ctx, "Failed to flush cached queries: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Debugf(ctx, "Updated %v song(s)", numSongs)
	writeTextResponse(w, "ok")
}

func handleListTags(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "GET", false) {
		return
	}
	tags, err := getTags(ctx)
	if err != nil {
		log.Errorf(ctx, "Unable to query tags: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, tags)
}

func handleNowNsec(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "GET", false) {
		return
	}
	writeTextResponse(w, strconv.FormatInt(time.Now().UnixNano(), 10))
}

func handleQuery(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "GET", false) {
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
		var ok bool
		if q.MinRating, ok = parseFloatParam(ctx, w, r, "minRating"); !ok {
			return
		}
		q.HasMinRating = true
	} else if r.FormValue("unrated") == "1" {
		q.Unrated = true
	}

	if len(r.FormValue("maxPlays")) > 0 {
		var ok bool
		if q.MaxPlays, ok = parseIntParam(ctx, w, r, "maxPlays"); !ok {
			return
		}
		q.HasMaxPlays = true
	}

	if len(r.FormValue("minFirstPlayed")) > 0 {
		if s, ok := parseFloatParam(ctx, w, r, "minFirstPlayed"); !ok {
			return
		} else {
			q.MinFirstStartTime = secondsToTime(s)
		}
	}
	if len(r.FormValue("maxLastPlayed")) > 0 {
		if s, ok := parseFloatParam(ctx, w, r, "maxLastPlayed"); !ok {
			return
		} else {
			q.MaxLastStartTime = secondsToTime(s)
		}
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

	songs, err := getSongsForQuery(ctx, q, cacheOnly)
	if err != nil {
		log.Errorf(ctx, "Unable to query songs: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, songs)
}

func handleRateAndTag(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "POST", false) {
		return
	}
	id, ok := parseIntParam(ctx, w, r, "songId")
	if !ok {
		return
	}

	var updateDelayNsec int64
	if len(r.FormValue("updateDelayNsec")) > 0 {
		if updateDelayNsec, ok = parseIntParam(ctx, w, r, "updateDelayNsec"); !ok {
			return
		}
	}
	updateDelay := time.Nanosecond * time.Duration(updateDelayNsec)

	hasRating := false
	var rating float64
	var tags []string
	if _, ok := r.Form["rating"]; ok {
		if rating, ok = parseFloatParam(ctx, w, r, "rating"); !ok {
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

	cfg := getConfig(ctx)
	if cfg.ForceUpdateFailures && appengine.IsDevAppServer() {
		http.Error(w, "Returning an error, as requested", http.StatusInternalServerError)
		return
	}

	if err := updateRatingAndTags(ctx, id, hasRating, rating, tags, updateDelay); err != nil {
		log.Errorf(ctx, "Got error while rating/tagging song: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTextResponse(w, "ok")
}

func handleReportPlayed(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	log.Debugf(ctx, "Got request: %v", r.URL.String())
	if !checkRequest(ctx, w, r, "POST", false) {
		return
	}

	id, ok := parseIntParam(ctx, w, r, "songId")
	if !ok {
		return
	}

	startTimeFloat, ok := parseFloatParam(ctx, w, r, "startTime")
	if !ok {
		return
	}
	startTime := secondsToTime(startTimeFloat)

	cfg := getConfig(ctx)
	if cfg.ForceUpdateFailures && appengine.IsDevAppServer() {
		http.Error(w, "Returning an error, as requested", http.StatusInternalServerError)
		return
	}

	// Drop the trailing colon and port number. We can't just split on ':' and
	// take the first item since we may get an IPv6 address like "[::1]:12345".
	ip := regexp.MustCompile(":\\d+$").ReplaceAllString(r.RemoteAddr, "")
	if err := addPlay(ctx, id, startTime, ip); err != nil {
		log.Errorf(ctx, "Got error while recording play: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	writeTextResponse(w, "ok")
}

func handleSongs(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "GET", false) {
		return
	}

	var max int64 = defaultDumpBatchSize
	if len(r.FormValue("max")) > 0 {
		var ok bool
		if max, ok = parseIntParam(ctx, w, r, "max"); !ok {
			return
		}
	}
	if max > maxDumpBatchSize {
		max = maxDumpBatchSize
	}

	var minLastModified time.Time
	if len(r.FormValue("minLastModifiedNsec")) > 0 {
		ns, ok := parseIntParam(ctx, w, r, "minLastModifiedNsec")
		if !ok {
			return
		}
		if ns > 0 {
			minLastModified = time.Unix(0, ns)
		}
	}

	var deleted int64
	if len(r.FormValue("deleted")) > 0 {
		var ok bool
		if deleted, ok = parseIntParam(ctx, w, r, "deleted"); !ok {
			return
		}
	}

	songs, cursor, err := dumpSongsForAndroid(ctx, minLastModified, deleted != 0, max, r.FormValue("cursor"))
	if err != nil {
		log.Errorf(ctx, "Unable to get songs: %v", err)
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
	writeJSONResponse(w, rows)
}
