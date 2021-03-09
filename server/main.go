// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/derat/nup/internal/pkg/types"
	"github.com/derat/nup/server/cache"
	"github.com/derat/nup/server/common"
	"github.com/derat/nup/server/cover"
	"github.com/derat/nup/server/dump"
	"github.com/derat/nup/server/query"
	"github.com/derat/nup/server/storage"
	"github.com/derat/nup/server/update"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/user"
)

const (
	indexPath = "web/index.html" // path to index relative to base dir

	// Maximum response size permitted by App Engine:
	// https://cloud.google.com/appengine/docs/standard/go111/how-requests-are-handled
	maxResponseSize = 32 * 1024 * 1024

	defaultDumpBatchSize = 100  // default size of batch of dumped entities
	maxDumpBatchSize     = 5000 // max size of batch of dumped entities

	maxCoverSize     = 800 // max size permitted in /cover scale requests
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
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
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
	cfg := common.Config(ctx)
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
	if err := common.LoadConfig(); err != nil {
		panic(fmt.Sprintf("Loading config failed: %v", err))
	}
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/cover", handleCover)
	http.HandleFunc("/delete_song", handleDeleteSong)
	http.HandleFunc("/dump_song", handleDumpSong)
	http.HandleFunc("/export", handleExport)
	http.HandleFunc("/import", handleImport)
	http.HandleFunc("/list_tags", handleListTags)
	http.HandleFunc("/now_nsec", handleNowNsec)
	http.HandleFunc("/query", handleQuery)
	http.HandleFunc("/rate_and_tag", handleRateAndTag)
	http.HandleFunc("/report_played", handleReportPlayed)
	http.HandleFunc("/song_data", handleSongData)
	http.HandleFunc("/songs", handleSongs)

	if appengine.IsDevAppServer() {
		http.HandleFunc("/clear", handleClear)
		http.HandleFunc("/config", handleConfig)
		http.HandleFunc("/flush_cache", handleFlushCache)
	}

	// The google.golang.org/appengine packages are deprecated, and the official
	// way forward is to use cloud.google.com/go and call http.ListenAndServe():
	// https://cloud.google.com/appengine/docs/standard/go/go-differences
	//
	// However, the new approach seems strictly worse in terms of usability,
	// functionality, and cost:
	//
	// Log messages written via the log package in the Go standard library don't
	// have a severity associated with them and don't get grouped with requests.
	// It looks like the cloud.google.com/go/logging package can be used to
	// write structured entries, but associating them with requests seems to
	// require parsing X-Cloud-Trace-Context headers from incoming requests:
	// https://cloud.google.com/appengine/docs/standard/go/writing-application-logs
	// There are apparently third-party packages that can make this easier.
	//
	// Memcache support is completely dropped. The suggestion is to use
	// Memorystore for Redis instead, but there's no free tier or shared instance:
	// https://cloud.google.com/appengine/docs/standard/go/using-memorystore
	// As of April 2020, the minimum cost (for a 1 GB Basic tier M1
	// instance) seems to be $0.049/hour, for about $35/month. I'm assuming that
	// you can't get billed for a partial GB.
	//
	// Datastore seems to be pretty much the same, but it sounds like you need to
	// run the datastore emulator now instead of using dev_appserver.py:
	// https://cloud.google.com/datastore/docs/tools/datastore-emulator
	// The emulator is still in beta, of course. You also need to explicitly
	// initialize a client, which is a bit painful when you're dealing with
	// individual requests and making datastore calls from different packages.
	//
	// The App Engine Mail and Blobstore APIs are apparently also getting killed
	// off, but this app fortunately doesn't use them.
	//
	// Support for the appengine packages is dropped in the go112 runtime and
	// later, so I guess I'll need to move over at some point. In the maintime,
	// appengine.Main() needs to be called here so that appengine.NewContext()
	// will work in the handlers.
	appengine.Main()
}

func handleClear(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	if !checkRequest(ctx, w, r, "POST", false) {
		return
	}
	if err := update.ClearData(ctx); err != nil {
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

	cfg := types.ServerConfig{}
	if err := json.NewDecoder(r.Body).Decode(&cfg); err == nil {
		common.AddTestUserToConfig(&cfg)
		common.SaveTestConfig(ctx, &cfg)
	} else if err == io.EOF {
		common.ClearTestConfig(ctx)
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
	var size int64
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
	if err := cover.Scale(ctx, fn, int(size), coverJPEGQuality, w); err != nil {
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
	if err := update.DeleteSong(ctx, id); err != nil {
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

	id, ok := parseIntParam(ctx, w, r, "songId")
	if !ok {
		return
	}

	s, err := dump.SingleSong(ctx, id)
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
	writeTextResponse(w, out.String())
}

func handleExport(w http.ResponseWriter, r *http.Request) {
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

	w.Header().Set("Content-Type", "text/plain")
	e := json.NewEncoder(w)

	var objectPtrs []interface{}
	var nextCursor string
	var err error

	switch r.FormValue("type") {
	case "song":
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

		var songs []types.Song
		songs, nextCursor, err = dump.Songs(ctx, max, r.FormValue("cursor"),
			r.FormValue("deleted") == "1", minLastModified)
		if err != nil {
			log.Errorf(ctx, "Dumping songs failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		omit := make(map[string]bool)
		for _, s := range strings.Split(r.FormValue("omit"), ",") {
			omit[s] = true
		}
		objectPtrs = make([]interface{}, len(songs))
		for i := range songs {
			s := &songs[i]
			if omit["coverFilename"] {
				s.CoverFilename = ""
			}
			if omit["plays"] {
				s.Plays = nil
			}
			if omit["sha1"] {
				s.SHA1 = ""
			}
			if omit["tags"] {
				s.Tags = nil
			}
			objectPtrs[i] = s
		}
	case "play":
		var plays []types.PlayDump
		plays, nextCursor, err = dump.Plays(ctx, max, r.FormValue("cursor"))
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
	if err := cache.Flush(ctx, cache.Memcache); err != nil {
		log.Errorf(ctx, "Flushing memcache failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.FormValue("onlyMemcache") != "1" {
		if err := cache.Flush(ctx, cache.Datastore); err != nil {
			log.Errorf(ctx, "Flushing datastore failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
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
		if err := update.UpdateOrInsertSong(ctx, s, replaceUserData, updateDelay); err != nil {
			log.Errorf(ctx, "Failed to update song with SHA1 %v: %v", s.SHA1, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		numSongs++
	}
	if err := cache.FlushForUpdate(ctx, common.MetadataUpdate); err != nil {
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
	tags, err := query.Tags(ctx, r.FormValue("requireCache") == "1")
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

	q := common.SongQuery{
		Artist:   r.FormValue("artist"),
		Title:    r.FormValue("title"),
		Album:    r.FormValue("album"),
		AlbumID:  r.FormValue("albumId"),
		Keywords: strings.Fields(r.FormValue("keywords")),
		Shuffle:  r.FormValue("shuffle") == "1",
	}

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

	songs, err := query.Songs(ctx, &q, cacheOnly)
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

	cfg := common.Config(ctx)
	if cfg.ForceUpdateFailures && appengine.IsDevAppServer() {
		http.Error(w, "Returning an error, as requested", http.StatusInternalServerError)
		return
	}

	if err := update.SetRatingAndTags(ctx, id, hasRating, rating, tags, updateDelay); err != nil {
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

	cfg := common.Config(ctx)
	if cfg.ForceUpdateFailures && appengine.IsDevAppServer() {
		http.Error(w, "Returning an error, as requested", http.StatusInternalServerError)
		return
	}

	// Drop the trailing colon and port number. We can't just split on ':' and
	// take the first item since we may get an IPv6 address like "[::1]:12345".
	ip := regexp.MustCompile(":\\d+$").ReplaceAllString(r.RemoteAddr, "")
	if err := update.AddPlay(ctx, id, startTime, ip); err != nil {
		log.Errorf(ctx, "Got error while recording play: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	writeTextResponse(w, "ok")
}

// The existence of this endpoint makes me extremely unhappy, but it seems necessary due to
// bad interactions between Google Cloud Storage, the Web Audio API, and CORS:
//
//  - The <audio> element doesn't allow its volume to be set above 1.0, so the web client needs to
//    use GainNode from the Web Audio API to amplify quiet tracks.
//  - <audio> seems to support playing cross-origin data as long as you don't look at it, but the
//    Web Audio API replaces cross-origin data with zeros:
//    https://www.w3.org/TR/webaudio/#MediaElementAudioSourceOptions-security
//  - You can use CORS to get around that, but the GCS authenticated browser endpoint
//    (storage.cloud.google.com) doesn't allow CORS requests:
//    https://cloud.google.com/storage/docs/cross-origin
//
// So, I'm copying songs through App Engine instead of letting GCS serve them so they won't be
// cross-origin.
//
// The Web Audio part of this is particularly frustrating, as the JS doesn't actually need to look
// at the audio data; it just need to amplify it.
func handleSongData(w http.ResponseWriter, req *http.Request) {
	ctx := appengine.NewContext(req)
	if !checkRequest(ctx, w, req, "GET", false) {
		return
	}

	fn := req.FormValue("filename")
	if fn == "" {
		log.Errorf(ctx, "Missing filename in song data request")
		http.Error(w, "Missing filename", http.StatusBadRequest)
		return
	}

	r, err := openSong(ctx, fn)
	if err != nil {
		log.Errorf(ctx, "Failed opening song %q: %v", fn, err)
		// TODO: It'd be better to report 404 when appropriate.
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer r.Close()

	if or, ok := r.(*storage.ObjectReader); ok {
		if err := sendObject(ctx, req, w, or); err != nil {
			log.Errorf(ctx, "Failed copying song %q: %v", fn, err)
		}
	} else {
		// Just send a 200 with the whole file if we're getting it over HTTP rather than from GCS.
		// This is only used by tests.
		w.Header().Set("Content-Type", "audio/mpeg")
		if _, err := io.Copy(w, r); err != nil {
			// Too late to report an HTTP error.
			log.Errorf(ctx, "Failed copying song %q: %v", fn, err)
		}
	}
}

// TODO: Delete this after updating the Android client to use /export.
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

	songs, cursor, err := dump.SongsAndroid(ctx, minLastModified, deleted != 0, max, r.FormValue("cursor"))
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
