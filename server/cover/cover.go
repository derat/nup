// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package cover loads and resizes album art cover images.
//
// More than you ever wanted to know about image sizes:
//
// Web notification icons should be 192x192 per
// https://developers.google.com/web/fundamentals/push-notifications/display-a-notification: "Sadly
// there aren't any solid guidelines for what size image to use for an icon. Android seems to want a
// 64dp image (which is 64px multiples by the device pixel ratio). If we assume the highest pixel
// ratio for a device will be 3, an icon size of 192px or more is a safe bet." On Chrome OS, icons
// look like they're just 58x58 on a device with a DPR of 1.6, suggesting that they're around 36x36
// dp, or 72x72 on a device with a DPR of 2.
//
// mediaSession on Chrome for Android uses 512x512 per https://web.dev/media-session/, although
// Chrome OS media notifications display album art at a substantially smaller size (128x128 at 1.6
// DPR, for 80x80 dp or 160x160 with a DPR of 2. The code seems to specify that 72x72 (dp?) is the
// desired size, though:
// https://github.com/chromium/chromium/blob/3abe39d/components/media_message_center/media_notification_view_modern_impl.cc#L50
//
// In the web interface, <play-view> uses 70x70 CSS pixels for the current song's cover and
// <fullscreen-overlay> uses 80x80 CSS pixels for the next song's cover. The song info dialog uses
// 192x192 CSS pixels. Favicons allegedly take a wide variety of sizes:
// https://stackoverflow.com/a/26807004
//
// In the Android client, NupActivity displays cover images at 100dp. Per
// https://developer.android.com/training/multiscreen/screendensities, the highest screen density is
// xxxhdpi, which looks like it's 4x (i.e. 4 pixels per dp), for 400x400. The Pixel 4a appears to
// just have a device pixel ratio of 2.75, i.e. xxhdpi. It sounds like xxxhdpi resources are maybe
// only used for launcher icons; 3x is realistically probably the most that needs to be handled:
// https://stackoverflow.com/questions/21452353/android-xxx-hdpi-real-devices
//
// For Android media-session-related stuff, the framework docs are not very helpful!
// https://developer.android.com/reference/kotlin/android/support/v4/media/MediaMetadataCompat#METADATA_KEY_ALBUM_ART:kotlin.String:
// "The artwork should be relatively small and may be scaled down by the system if it is too large."
// Thanks, I will try to make the images relatively small and not too large.
//
// My Android Auto head unit is only 800x480 (and probably only uses around a third of the vertical
// height for album art, with a blurry, scaled-up version in the background). The most expensive
// aftermarket AA units that I see right now have 1280x720 displays.
//
// So I think that the Android client, which downloads and caches images in a single size,
// realistically just needs something like 384x384. 512x512 is probably safer to handle future
// Android Auto UI changes.
//
// For the web interface, I don't think that there's much in the way of non-mobile devices that have
// a DPR above 2 (the 2021 MacBook Pro reports 2.0 for window.devicePixelRatio, for instance).
// 160x160 is probably enough for everything except the song info dialog (which can use the same
// 512x512 images as Android), but I'm going to go with 256x256 for a bit of future-proofing (and
// because it typically seems to be only a few KB larger than 160x160 in WebP).
package cover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"

	"golang.org/x/image/draw"

	"google.golang.org/api/option"
	"google.golang.org/appengine/v2/log"
	"google.golang.org/appengine/v2/memcache"
)

// A single storage.Client is initialized in response to the first load() call
// that needs to read from Cloud Storage and then reused. I was initially seeing
// very slow NewClient() and Object() calls in load(), sometimes taking close to
// a second in total. When reusing a single client, I frequently see 90-160 ms,
// but the numbers are noisy enough that I'm still not completely convinced
// that this helps.
var client *storage.Client
var clientOnce sync.Once

// More superstition: https://github.com/googleapis/google-cloud-go/issues/530
const grpcPoolSize = 4

const (
	cacheKeyPrefix  = "cover"   // memcache key prefix
	cacheExpiration = time.Hour // memcache expiration
)

// cacheKey returns the memcache key that should be used for caching a
// cover image with the supplied filename, size (i.e. width/height), and format.
func cacheKey(fn string, size int, it imageType) string {
	// TODO: Hash the filename?
	// https://godoc.org/google.golang.org/appengine/memcache#Get says that the
	// key can be at most 250 bytes.
	key := fmt.Sprintf("%s-%d-", cacheKeyPrefix, size)
	if it == webpType {
		key += "webp-"
	}
	return key + fn
}

// OrigExt is the extension for original (non-WebP) cover images.
const OrigExt = ".jpg"

// WebPSizes contains the sizes for which WebP versions of images can be requested.
// See the package comment for the origin of these numbers.
var WebPSizes = []int{256, 512}

// WebPFilename returns the filename that should be used for the WebP version of JPEG
// file fn scaled to the specified size. fn can be a full path.
// Given fn "foo/bar.jpg" and size 256, returns "foo/bar.256.webp".
func WebPFilename(fn string, size int) string {
	if strings.HasSuffix(fn, OrigExt) {
		fn = fn[:len(fn)-4]
	}
	return fmt.Sprintf("%s.%d.webp", fn, size)
}

var webpRegexp = regexp.MustCompile(`(.+)\.\d+\.webp$`)

// OrigFilename attempts to return the original JPEG filename for the supplied WebP cover image
// (generated by WebPFilename). Given "foo/bar.256.webp", returns "foo/bar.jpeg".
// fn is returned unchanged if it doesn't appear to be a generated image.
func OrigFilename(fn string) string {
	ms := webpRegexp.FindStringSubmatch(fn)
	if ms == nil {
		return fn
	}
	return ms[1] + OrigExt
}

// Scale reads the cover image at fn (corresponding to Song.CoverFilename),
// scales and crops it to be a square image with the supplied width and height
// size, and writes it in JPEG format to w.
//
// If size is zero or negative, the original (possibly non-square) cover data is written.
// If webp is true, a prescaled WebP version of the image will be returned if available.
// The bucket and baseURL args correspond to CoverBucket and CoverBaseURL in ServerConfig.
// If w is an http.ResponseWriter, its Content-Type header will be set.
// os.ErrNotExist is replied if the specified file does not exist.
func Scale(ctx context.Context, bucket, baseURL, fn string,
	size, quality int, webp bool, w io.Writer) error {
	// If WebP was requested, try to load it first before falling back to JPEG.
	// There's sadly still no native Go library for encoding to WebP (only decoding),
	// so we rely on files generated by the "nup covers" command.
	if webp {
		log.Debugf(ctx, "Checking cache for WebP cover")
		if data, _ := getCachedCover(ctx, fn, size, webpType); len(data) > 0 {
			log.Debugf(ctx, "Writing %d-byte cached WebP cover", len(data))
			setContentType(w, webpType)
			_, err := w.Write(data)
			return err
		}
		log.Debugf(ctx, "Loading WebP cover")
		wfn := WebPFilename(fn, size)
		if data, err := load(ctx, bucket, baseURL, wfn); err != nil {
			log.Debugf(ctx, "Failed loading WebP cover: %v", err)
		} else {
			setContentType(w, webpType)
			_, werr := w.Write(data)
			log.Debugf(ctx, "Caching %v-byte WebP cover", len(data))
			if err := setCachedCover(ctx, fn, size, webpType, data); err != nil {
				log.Errorf(ctx, "Cache write failed: %v", err) // swallow error
			}
			return werr
		}
	}

	log.Debugf(ctx, "Checking cache for scaled cover")
	if data, _ := getCachedCover(ctx, fn, size, jpegType); len(data) > 0 {
		log.Debugf(ctx, "Writing %d-byte cached scaled cover", len(data))
		setContentType(w, jpegType)
		_, err := w.Write(data)
		return err
	}

	var data []byte
	var err error
	log.Debugf(ctx, "Checking cache for original cover")
	if data, err = getCachedCover(ctx, fn, 0, jpegType); len(data) > 0 {
		log.Debugf(ctx, "Got %d-byte cached original cover", len(data))
	} else if err != nil {
		log.Errorf(ctx, "Cache lookup failed: %v", err) // swallow error
	}

	if len(data) == 0 {
		log.Debugf(ctx, "Loading original cover")
		if data, err = load(ctx, bucket, baseURL, fn); err != nil {
			return fmt.Errorf("failed to read cover: %v", err)
		}
		log.Debugf(ctx, "Caching %v-byte original cover", len(data))
		if err = setCachedCover(ctx, fn, 0, jpegType, data); err != nil {
			log.Errorf(ctx, "Cache write failed: %v", err) // swallow error
		}
	}

	if size <= 0 {
		log.Debugf(ctx, "Writing %d-byte original cover", len(data))
		setContentType(w, jpegType)
		_, err = w.Write(data)
		return err
	}

	log.Debugf(ctx, "Decoding %v bytes", len(data))
	src, _, err := image.Decode(bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	// Crop the source image rect if it isn't square.
	sr := src.Bounds()
	if sr.Dx() > sr.Dy() {
		sr.Min.X += (sr.Dx() - sr.Dy()) / 2
		sr.Max.X = sr.Min.X + sr.Dy()
	} else if sr.Dy() > sr.Dx() {
		sr.Min.Y += (sr.Dy() - sr.Dx()) / 2
		sr.Max.Y = sr.Min.Y + sr.Dx()
	}

	// TODO: Would it be better to never upscale?

	log.Debugf(ctx, "Scaling from %vx%v to %vx%v", sr.Dx(), sr.Dy(), size, size)
	dr := image.Rect(0, 0, size, size)
	dst := image.NewRGBA(dr)
	// draw.CatmullRom seems to be very slow. I've seen a Scale call from
	// 1200x1200 to 512x512 take 908 ms on App Engine.
	draw.ApproxBiLinear.Scale(dst, dr, src, sr, draw.Src, nil)

	log.Debugf(ctx, "JPEG-encoding scaled image")
	setContentType(w, jpegType)
	var b bytes.Buffer
	w = io.MultiWriter(w, &b)
	if err := jpeg.Encode(w, dst, &jpeg.Options{Quality: quality}); err != nil {
		return err
	}
	log.Debugf(ctx, "Caching %v-byte scaled cover", b.Len())
	if err := setCachedCover(ctx, fn, size, jpegType, b.Bytes()); err != nil {
		log.Errorf(ctx, "Cache write failed: %v", err) // swallow error
	}
	return nil
}

// load loads and returns the cover image with the supplied original filename (see Song.CoverFilename).
func load(ctx context.Context, bucket, baseURL, fn string) ([]byte, error) {
	var r io.ReadCloser
	if bucket != "" {
		// It would seem more reasonable to call NewClient from an init()
		// function instead, but that produces an error like the following:
		//
		//   dialing: google: could not find default credentials. See
		//   https://developers.google.com/accounts/docs/application-default-credentials for more information.
		//
		// This happens regardless of whether I pass context.Background() or
		// appengine.BackgroundContext(). It feels wrong to use the credentials
		// from the first request for all later requests, but it seems to work.
		// Requests are only accepted from a specific list of users and are all
		// satisfied using the same GCS bucket, so hopefully there are no
		// security implications from doing this.
		var err error
		clientOnce.Do(func() {
			log.Debugf(ctx, "Initializing storage client")
			client, err = storage.NewClient(ctx, option.WithGRPCConnectionPool(grpcPoolSize))
		})
		if err != nil {
			return nil, err
		}
		log.Debugf(ctx, "Opening object %q from bucket %q", fn, bucket)
		if r, err = client.Bucket(bucket).Object(fn).NewReader(ctx); err == storage.ErrObjectNotExist {
			return nil, os.ErrNotExist
		} else if err != nil {
			return nil, err
		}
	} else if baseURL != "" {
		url := baseURL + fn
		log.Debugf(ctx, "Opening %v", url)
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		} else if resp.StatusCode >= 300 {
			resp.Body.Close()
			if resp.StatusCode == 404 {
				return nil, os.ErrNotExist
			}
			return nil, fmt.Errorf("server replied with %q", resp.Status)
		}
		r = resp.Body
	} else {
		return nil, errors.New("neither CoverBucket nor CoverBaseURL is set")
	}
	defer r.Close()

	log.Debugf(ctx, "Reading cover data")
	return ioutil.ReadAll(r)
}

// setCachedCover caches a cover image with the supplied filename, requested size,
// format, and raw data. size should be 0 when caching the original image.
func setCachedCover(ctx context.Context, fn string, size int, it imageType, data []byte) error {
	return memcache.Set(ctx, &memcache.Item{
		Key:        cacheKey(fn, size, it),
		Value:      data,
		Expiration: cacheExpiration,
	})
}

// getCachedCover attempts to look up raw data for the cover image with the supplied
// filename, size, and format. If the image isn't present, both the returned byte slice
// and the error are nil.
func getCachedCover(ctx context.Context, fn string, size int, it imageType) ([]byte, error) {
	item, err := memcache.Get(ctx, cacheKey(fn, size, it))
	if err == memcache.ErrCacheMiss {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return item.Value, nil
}

type imageType string

const (
	jpegType imageType = "image/jpeg"
	webpType imageType = "image/webp"
)

// setContentType sets w's Content-Type to it if w is an http.ResponseWriter.
func setContentType(w io.Writer, it imageType) {
	if rw, ok := w.(http.ResponseWriter); ok {
		rw.Header().Set("Content-Type", string(it))
	}
}
