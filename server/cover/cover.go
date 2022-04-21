// Copyright 2020 Daniel Erat.
// All rights reserved.

// Package cover loads and resizes album art cover images.
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
	cacheKeyPrefix  = "cover"        // memcache key prefix
	cacheExpiration = 24 * time.Hour // memcache expiration
)

// cacheKey returns the memcache key that should be used for caching a
// cover image with the supplied filename and size (i.e. width/height).
func cacheKey(fn string, size int) string {
	// TODO: Hash the filename?
	// https://godoc.org/google.golang.org/appengine/memcache#Get says that the
	// key can be at most 250 bytes.
	return fmt.Sprintf("%s-%d-%s", cacheKeyPrefix, size, fn)
}

// Scale reads the cover image at fn (corresponding to Song.CoverFilename),
// scales and crops it to be a square image with the supplied width and height
// size, and writes it in JPEG format to w. If size is zero or negative, the
// original (possibly non-square) cover data is written.
// The bucket and baseURL args correspond to CoverBucket and CoverBaseURL in ServerConfig.
// os.ErrNotExist is replied if the specified file does not exist.
func Scale(ctx context.Context, bucket, baseURL, fn string,
	size, quality int, w io.Writer) error {
	var data []byte
	var err error

	log.Debugf(ctx, "Checking cache for scaled cover")
	if data, err = getCachedCover(ctx, fn, size); len(data) > 0 {
		log.Debugf(ctx, "Writing %d-byte cached scaled cover", len(data))
		_, err = w.Write(data)
		return err
	}
	log.Debugf(ctx, "Checking cache for original cover")
	if data, err = getCachedCover(ctx, fn, 0); len(data) > 0 {
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
		if err = setCachedCover(ctx, fn, 0, data); err != nil {
			log.Errorf(ctx, "Cache write failed: %v", err) // swallow error
		}
	}

	if size <= 0 {
		log.Debugf(ctx, "Writing %d-byte original cover", len(data))
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

	log.Debugf(ctx, "Scaling from %vx%v to %vx%v",
		sr.Dx(), sr.Dy(), size, size)
	dr := image.Rect(0, 0, size, size)
	dst := image.NewRGBA(dr)
	// draw.CatmullRom seems to be very slow. I've seen a Scale call from
	// 1200x1200 to 512x512 take 908 ms on App Engine.
	draw.ApproxBiLinear.Scale(dst, dr, src, sr, draw.Src, nil)

	log.Debugf(ctx, "JPEG-encoding scaled image")
	var b bytes.Buffer
	w = io.MultiWriter(w, &b)
	if err := jpeg.Encode(w, dst, &jpeg.Options{Quality: quality}); err != nil {
		return err
	}
	log.Debugf(ctx, "Caching %v-byte scaled cover", b.Len())
	if err := setCachedCover(ctx, fn, size, b.Bytes()); err != nil {
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

// setCachedCover caches a cover image with the supplied filename, requested size, and
// raw data. size should be 0 when caching the original image.
func setCachedCover(ctx context.Context, fn string, size int, data []byte) error {
	return memcache.Set(ctx, &memcache.Item{
		Key:        cacheKey(fn, size),
		Value:      data,
		Expiration: cacheExpiration,
	})
}

// getCachedCover attempts to look up raw data for the cover image with the supplied
// filename and size. If the image isn't present, both the returned byte slice
// and the error are nil.
func getCachedCover(ctx context.Context, fn string, size int) ([]byte, error) {
	item, err := memcache.Get(ctx, cacheKey(fn, size))
	if err == memcache.ErrCacheMiss {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return item.Value, nil
}
