// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package main implements nup's App Engine server.
package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/derat/nup/server/cache"
	"github.com/derat/nup/server/config"
	"github.com/derat/nup/server/cover"
	"github.com/derat/nup/server/db"
	"github.com/derat/nup/server/dump"
	"github.com/derat/nup/server/query"
	"github.com/derat/nup/server/ratelimit"
	"github.com/derat/nup/server/stats"
	"github.com/derat/nup/server/update"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/log"
)

const (
	defaultDumpBatchSize = 100  // default size of batch of dumped entities
	maxDumpBatchSize     = 5000 // max size of batch of dumped entities

	maxCoverSize     = 800 // max size permitted in /cover scale requests
	coverJPEGQuality = 90  // quality to use when encoding /cover replies
)

// forceUpdateFailures can be set by tests via /config to indicate that failures should be reported
// for all user data updates (ratings, tags, plays).
// TODO: This will only affect the instance that receives the /config request. For now,
// test/dev_server.go passes --max_module_instances=1 to ensure that there's a single instance.
var forceUpdateFailures = false

// staticFileETags maps from a relative request path (e.g. "index.html") to a string containing
// a quoted ETag header value for the file. We do this here instead of in getStaticFile() since
// we don't need to hash the files that go into the bundle, just the bundle itself.
var staticFileETags sync.Map

func main() {
	rand.Seed(time.Now().UnixNano())

	// Get masks for various types of users.
	norm := config.NormalUser
	admin := config.AdminUser
	guest := config.GuestUser
	cron := config.CronUser

	// Use a wrapper instead of calling http.HandleFunc directly to reduce the risk
	// that a handler neglects checking that requests are authorized.
	addHandler("/", http.MethodGet, norm|admin|guest, redirectUnauth, handleStatic)
	addHandler("/manifest.json", http.MethodGet, norm|admin|guest, allowUnauth, handleStatic)

	addHandler("/cover", http.MethodGet, norm|admin|guest, rejectUnauth, handleCover)
	addHandler("/delete_song", http.MethodPost, admin, rejectUnauth, handleDeleteSong)
	addHandler("/dump_song", http.MethodGet, norm|admin|guest, rejectUnauth, handleDumpSong)
	addHandler("/export", http.MethodGet, norm|admin|guest, rejectUnauth, handleExport)
	addHandler("/import", http.MethodPost, admin, rejectUnauth, handleImport)
	addHandler("/now", http.MethodGet, norm|admin|guest, rejectUnauth, handleNow)
	addHandler("/played", http.MethodPost, norm|admin|guest, rejectUnauth, handlePlayed)
	addHandler("/presets", http.MethodGet, norm|admin|guest, rejectUnauth, handlePresets)
	addHandler("/query", http.MethodGet, norm|admin|guest, rejectUnauth, handleQuery)
	addHandler("/rate_and_tag", http.MethodPost, norm|admin, rejectUnauth, handleRateAndTag)
	addHandler("/reindex", http.MethodPost, admin, rejectUnauth, handleReindex)
	addHandler("/song", http.MethodGet, norm|admin|guest, rejectUnauth, handleSong)
	addHandler("/stats", http.MethodGet, norm|admin|guest|cron, rejectUnauth, handleStats)
	addHandler("/tags", http.MethodGet, norm|admin|guest, rejectUnauth, handleTags)

	if appengine.IsDevAppServer() {
		addHandler("/clear", http.MethodPost, admin, rejectUnauth, handleClear)
		addHandler("/config", http.MethodPost, admin, rejectUnauth, handleConfig)
		addHandler("/flush_cache", http.MethodPost, admin, rejectUnauth, handleFlushCache)
	}

	// Generate the index file and JS bundle so we're ready to serve them.
	// We can't check whether minification is enabled at this point (since we don't
	// have a context to load the config from datastore), so just optimistically
	// assume that it is.
	getStaticFile(indexFile, true)
	getStaticFile(bundleFile, true)

	// The google.golang.org/appengine packages are (were?) deprecated, and the official way forward
	// is (was?) to use the non-App-Engine-specific cloud.google.com/go packages and call
	// http.ListenAndServe(): https://cloud.google.com/appengine/docs/standard/go/go-differences
	//
	// However, this approach seems strictly worse in terms of usability, functionality, and cost:
	//
	// Log messages written via the log package in the Go standard library don't have a severity
	// associated with them and don't get grouped with requests. It looks like the
	// cloud.google.com/go/logging package can be used to write structured entries, but associating
	// them with requests seems to require parsing X-Cloud-Trace-Context headers from incoming
	// requests: https://cloud.google.com/appengine/docs/standard/go/writing-application-logs
	// There are apparently third-party packages that can make this easier.
	//
	// Memcache support is completely dropped. The suggestion is to use Memorystore for Redis
	// instead, but there's no free tier or shared instance:
	// https://cloud.google.com/appengine/docs/standard/go/using-memorystore
	// As of April 2020, the minimum cost (for a 1 GB Basic tier M1 instance) seems to be
	// $0.049/hour, for about $35/month. I'm assuming that you can't get billed for a partial GB.
	//
	// Datastore seems to be pretty much the same, but it sounds like you need to run the datastore
	// emulator now instead of using dev_appserver.py:
	// https://cloud.google.com/datastore/docs/tools/datastore-emulator
	// The emulator is still in beta, of course. You also need to explicitly initialize a client,
	// which is a bit painful when you're dealing with individual requests and making datastore
	// calls from different packages.
	//
	// The App Engine Mail and Blobstore APIs are apparently also getting killed off, but this app
	// fortunately doesn't use them.
	//
	// Support for the appengine packages was initially dropped in the go112 runtime, but as of
	// November 2021, it seems like this policy was maybe silently changed.
	// https://cloud.google.com/appengine/docs/standard/go/go-differences now links to
	// https://cloud.google.com/appengine/docs/standard/go/services/access, which explains how to
	// continue using App Engine bundled services in Go 1.12+ (currently in a preview state).
	//
	// appengine.Main() needs to be called here so that appengine.NewContext() will work in the
	// handlers.
	appengine.Main()
}

func handleClear(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	if err := update.ClearData(ctx); err != nil {
		log.Errorf(ctx, "Clearing songs and plays failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := stats.Clear(ctx); err != nil {
		log.Errorf(ctx, "Clearing stats failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := ratelimit.Clear(ctx); err != nil {
		log.Errorf(ctx, "Clearing rate-limiting info failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTextResponse(w, "ok")
}

func handleConfig(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	forceUpdateFailures = r.FormValue("forceUpdateFailures") == "1"
	writeTextResponse(w, "ok")
}

func handleCover(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
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
	webp := r.FormValue("webp") == "1"

	// cover.Scale will set the Content-Type header.
	addLongCacheHeaders(w)
	if err := cover.Scale(ctx, cfg.CoverBucket, cfg.CoverBaseURL, fn, int(size),
		coverJPEGQuality, webp, w); err != nil {
		log.Errorf(ctx, "Scaling cover %q failed: %v", fn, err)
		if os.IsNotExist(err) {
			http.Error(w, "Not found", http.StatusNotFound)
		} else {
			http.Error(w, "Scaling failed", http.StatusInternalServerError)
		}
		return
	}
}

func handleDeleteSong(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	id, ok := parseIntParam(ctx, w, r, "songId")
	if !ok {
		return
	}
	if err := update.DeleteSong(ctx, id); err != nil {
		log.Errorf(ctx, "Deleting song %v failed: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	writeTextResponse(w, "ok")
}

func handleDumpSong(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
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

func handleExport(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
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

		var songs []db.Song
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
			objectPtrs[i] = s
		}
	case "play":
		var plays []db.PlayDump
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

func handleFlushCache(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	if err := query.FlushCache(ctx, cache.Memcache); err != nil {
		log.Errorf(ctx, "Flushing query cache from memcache failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.FormValue("onlyMemcache") != "1" {
		if err := query.FlushCache(ctx, cache.Datastore); err != nil {
			log.Errorf(ctx, "Flushing query cache from datastore failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeTextResponse(w, "ok")
}

func handleImport(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	dataPolicy := update.PreserveUserData
	if r.FormValue("replaceUserData") == "1" {
		dataPolicy = update.ReplaceUserData
	}

	keyType := update.UpdateBySHA1
	if r.FormValue("useFilenames") == "1" {
		keyType = update.UpdateByFilename
	}

	var delay time.Duration
	if len(r.FormValue("updateDelayNsec")) > 0 {
		if ns, ok := parseIntParam(ctx, w, r, "updateDelayNsec"); !ok {
			return
		} else {
			delay = time.Nanosecond * time.Duration(ns)
		}
	}

	numSongs := 0
	d := json.NewDecoder(r.Body)
	for {
		s := &db.Song{}
		if err := d.Decode(s); err == io.EOF {
			break
		} else if err != nil {
			log.Errorf(ctx, "Decode song failed: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := update.UpdateOrInsertSong(ctx, s, dataPolicy, keyType, delay); err != nil {
			log.Errorf(ctx, "Update song with SHA1 %v failed: %v", s.SHA1, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		numSongs++
	}
	if err := query.FlushCacheForUpdate(ctx, query.MetadataUpdate); err != nil {
		log.Errorf(ctx, "Flushing query cache for update failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	log.Debugf(ctx, "Updated %v song(s)", numSongs)
	writeTextResponse(w, "ok")
}

func handleNow(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	writeTextResponse(w, strconv.FormatInt(time.Now().UnixNano(), 10))
}

func handlePlayed(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	id, ok := parseIntParam(ctx, w, r, "songId")
	if !ok {
		return
	}
	startTime, ok := parseDateParam(ctx, w, r, "startTime")
	if !ok {
		return
	}

	if forceUpdateFailures && appengine.IsDevAppServer() {
		http.Error(w, "Returning an error, as requested", http.StatusInternalServerError)
		return
	}

	// SplitHostPort removes brackets for us.
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// Drop the trailing colon and port number. We can't just split on ':' and
		// take the first item since we may get an IPv6 address like "[::1]:12345".
		ip = regexp.MustCompile(":\\d+$").ReplaceAllString(r.RemoteAddr, "")
	}

	if err := update.AddPlay(ctx, id, startTime, ip); err != nil {
		log.Errorf(ctx, "Recording play of %v at %v failed: %v", id, startTime, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	writeTextResponse(w, "ok")
}

func handlePresets(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	writeJSONResponse(w, cfg.Presets)
}

func handleQuery(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	var flags query.SongsFlags
	if r.FormValue("cacheOnly") == "1" {
		flags |= query.CacheOnly
	}
	if v := r.FormValue("fallback"); v == "force" {
		flags |= query.ForceFallback
	} else if v == "never" {
		flags |= query.NoFallback
	}

	q := query.SongQuery{
		Artist:               r.FormValue("artist"),
		Title:                r.FormValue("title"),
		Album:                r.FormValue("album"),
		AlbumID:              r.FormValue("albumId"),
		Filename:             r.FormValue("filename"),
		Keywords:             strings.Fields(r.FormValue("keywords")),
		MaxPlays:             -1,
		Shuffle:              r.FormValue("shuffle") == "1",
		OrderByLastStartTime: r.FormValue("orderByLastPlayed") == "1",
	}

	if r.FormValue("firstTrack") == "1" {
		q.Track = 1
		q.Disc = 1
	}

	if len(r.FormValue("minRating")) > 0 {
		if v, ok := parseIntParam(ctx, w, r, "minRating"); !ok {
			return
		} else {
			q.MinRating = int(v)
		}
	} else if r.FormValue("unrated") == "1" {
		q.Unrated = true
	}

	if len(r.FormValue("maxPlays")) > 0 {
		var ok bool
		if q.MaxPlays, ok = parseIntParam(ctx, w, r, "maxPlays"); !ok {
			return
		}
	}

	for name, dst := range map[string]*time.Time{
		"minDate":        &q.MinDate,
		"maxDate":        &q.MaxDate,
		"minFirstPlayed": &q.MinFirstStartTime,
		"maxLastPlayed":  &q.MaxLastStartTime,
	} {
		if len(r.FormValue(name)) > 0 {
			var ok bool
			if *dst, ok = parseDateParam(ctx, w, r, name); !ok {
				return
			}
		}
	}

	for _, t := range strings.Fields(r.FormValue("tags")) {
		if t[0] == '-' {
			q.NotTags = append(q.NotTags, t[1:len(t)])
		} else {
			q.Tags = append(q.Tags, t)
		}
	}

	songs, err := query.Songs(ctx, &q, flags)
	if err != nil {
		log.Errorf(ctx, "Unable to query songs: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, songs)
}

func handleRateAndTag(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	id, ok := parseIntParam(ctx, w, r, "songId")
	if !ok {
		return
	}

	var delay time.Duration
	if len(r.FormValue("updateDelayNsec")) > 0 {
		if ns, ok := parseIntParam(ctx, w, r, "updateDelayNsec"); !ok {
			return
		} else {
			delay = time.Nanosecond * time.Duration(ns)
		}
	}

	var hasRating bool
	var rating int
	var tags []string
	if _, ok := r.Form["rating"]; ok {
		if v, ok := parseIntParam(ctx, w, r, "rating"); !ok {
			return
		} else {
			rating = int(v)
		}
		hasRating = true
		if rating < 0 {
			rating = 0
		} else if rating > 5 {
			rating = 5
		}
	}
	if _, ok := r.Form["tags"]; ok {
		tags = strings.Fields(r.FormValue("tags"))
	}
	if !hasRating && tags == nil {
		http.Error(w, "No rating or tags supplied", http.StatusBadRequest)
		return
	}

	if forceUpdateFailures && appengine.IsDevAppServer() {
		http.Error(w, "Returning an error, as requested", http.StatusInternalServerError)
		return
	}

	if err := update.SetRatingAndTags(ctx, id, hasRating, rating, tags, delay); err != nil {
		log.Errorf(ctx, "Rating/tagging song %d failed: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeTextResponse(w, "ok")
}

func handleReindex(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	cursor, scanned, updated, err := update.ReindexSongs(ctx, r.FormValue("cursor"))
	if err != nil {
		log.Errorf(ctx, "Reindexing songs failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, struct {
		Scanned int    `json:"scanned"`
		Updated int    `json:"updated"`
		Cursor  string `json:"cursor"`
	}{
		scanned, updated, cursor,
	})
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
func handleSong(ctx context.Context, cfg *config.Config, w http.ResponseWriter, req *http.Request) {
	if max := cfg.MaxGuestSongRequestsPerHour; max > 0 {
		if user, utype := cfg.GetUser(req); utype == config.GuestUser {
			// TODO: This should probably handle range requests differently.
			// Maybe we should just count requests that ask for the first byte?
			if err := ratelimit.Attempt(ctx, user, time.Now(), max, time.Hour); err != nil {
				log.Errorf(ctx, "Song request from %q rejected: %v", user, err)
				http.Error(w, "Guest rate limit exceeded", http.StatusTooManyRequests)
				return
			}
		}
	}

	fn := req.FormValue("filename")
	if fn == "" {
		log.Errorf(ctx, "Missing filename in song data request")
		http.Error(w, "Missing filename", http.StatusBadRequest)
		return
	}

	r, err := openSong(ctx, cfg, fn)
	if err != nil {
		log.Errorf(ctx, "Opening song %q failed: %v", fn, err)
		if os.IsNotExist(err) {
			http.Error(w, "Not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("Failed opening song: %v", err), http.StatusInternalServerError)
		}
		return
	}
	defer r.Close()

	addLongCacheHeaders(w)

	if sr, ok := r.(songReader); ok {
		if err := sendSong(ctx, req, w, sr); err != nil {
			log.Errorf(ctx, "Sending song %q failed: %v", fn, err)
		}
	} else {
		// Just send a 200 with the whole file if we're getting it over HTTP rather than from GCS.
		// This is only used by tests.
		w.Header().Set("Content-Type", "audio/mpeg")
		if _, err := io.Copy(w, r); err != nil {
			// Too late to report an HTTP error.
			log.Errorf(ctx, "Sending song %q failed: %v", fn, err)
		}
	}
}

func handleStatic(ctx context.Context, cfg *config.Config, w http.ResponseWriter, req *http.Request) {
	p := filepath.Clean(req.URL.Path)
	if p == "/" {
		p = "index.html"
	} else if strings.HasPrefix(p, "/") {
		p = p[1:]
	}

	minify := cfg.Minify == nil || *cfg.Minify
	if strings.HasSuffix(p, ".ts") {
		// Serving TypeScript files doesn't make sense.
		http.Error(w, "Not found", http.StatusNotFound)
	} else if b, err := getStaticFile(p, minify); os.IsNotExist(err) {
		http.Error(w, "Not found", http.StatusNotFound)
	} else if err != nil {
		log.Errorf(ctx, "Reading %q failed: %v", p, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		var etag string
		if v, ok := staticFileETags.Load(p); ok {
			etag = v.(string)
		} else {
			sum := sha1.Sum(b)
			etag = fmt.Sprintf(`"%s"`, hex.EncodeToString(sum[:]))
			staticFileETags.Store(p, etag)
		}
		w.Header().Set("ETag", etag)

		// App Engine seems to always report static file mtimes as 1980:
		//  https://issuetracker.google.com/issues/168399701
		//  https://stackoverflow.com/questions/63813692
		//  https://github.com/GoogleChrome/web.dev/issues/3913
		http.ServeContent(w, req, filepath.Base(p), time.Time{}, bytes.NewReader(b))
	}
}

func handleStats(ctx context.Context, cfg *config.Config, w http.ResponseWriter, req *http.Request) {
	// Updates would be better suited to POST than to GET, but App Engine cron uses GET per
	// https://cloud.google.com/appengine/docs/standard/go/scheduling-jobs-with-cron-yaml.
	if req.FormValue("update") == "1" {
		// Don't let guest users update stats.
		if user, utype := cfg.GetUser(req); utype == config.GuestUser {
			log.Errorf(ctx, "Rejecting stats update from guest user %q", user)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		if err := stats.Update(ctx); err != nil {
			log.Errorf(ctx, "Updating stats failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeTextResponse(w, "ok")
		return
	}

	stats, err := stats.Get(ctx)
	if err != nil {
		log.Errorf(ctx, "Getting stats failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, stats)
}

func handleTags(ctx context.Context, cfg *config.Config, w http.ResponseWriter, r *http.Request) {
	tags, err := query.Tags(ctx, r.FormValue("requireCache") == "1")
	if err != nil {
		log.Errorf(ctx, "Querying tags failed: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, tags)
}
