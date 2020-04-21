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
	"sync"

	"cloud.google.com/go/storage"

	"github.com/derat/nup/server/cache"
	"github.com/derat/nup/server/common"

	"golang.org/x/image/draw"

	"google.golang.org/api/option"
	"google.golang.org/appengine/log"
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

// Scale reads the cover image at fn (corresponding to Song.CoverFilename),
// scales and crops it to be a square image with the supplied width and height
// size, and writes it in JPEG format to w.
func Scale(ctx context.Context, fn string, size, quality int, w io.Writer) error {
	var data []byte
	var err error

	log.Debugf(ctx, "Checking cache for scaled cover")
	if data, err = cache.GetCover(ctx, fn, size); len(data) > 0 {
		log.Debugf(ctx, "Writing %d-byte cached scaled cover", len(data))
		_, err = w.Write(data)
		return err
	}
	log.Debugf(ctx, "Checking cache for original cover")
	if data, err = cache.GetCover(ctx, fn, 0); len(data) > 0 {
		log.Debugf(ctx, "Got %d-byte cached original cover", len(data))
	} else if err != nil {
		log.Errorf(ctx, "Cache lookup failed: %v", err) // swallow error
	}

	if len(data) == 0 {
		log.Debugf(ctx, "Loading original cover")
		if data, err = load(ctx, fn); err != nil {
			return fmt.Errorf("failed to read cover: %v", err)
		}
		log.Debugf(ctx, "Caching %v-byte original cover", len(data))
		if err = cache.SetCover(ctx, fn, 0, data); err != nil {
			log.Errorf(ctx, "Cache write failed: %v", err) // swallow error
		}
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
		sr.Max.X = sr.Dy()
	} else if sr.Dy() > sr.Dx() {
		sr.Min.Y += (sr.Dy() - sr.Dx()) / 2
		sr.Max.Y = sr.Dx()
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
	if err := cache.SetCover(ctx, fn, size, b.Bytes()); err != nil {
		log.Errorf(ctx, "Cache write failed: %v", err) // swallow error
	}
	return nil
}

// load loads and returns the cover image with the supplied original
// filename (see Song.CoverFilename).
func load(ctx context.Context, fn string) ([]byte, error) {
	var r io.ReadCloser
	if cfg := common.Config(ctx); cfg.CoverBucket != "" {
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
		log.Debugf(ctx, "Opening object %q from bucket %q", fn, cfg.CoverBucket)
		if r, err = client.Bucket(cfg.CoverBucket).Object(fn).NewReader(ctx); err != nil {
			return nil, err
		}
	} else if cfg.CoverBaseURL != "" {
		url := cfg.CoverBaseURL + fn
		log.Debugf(ctx, "Opening %v", url)
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		r = resp.Body
	} else {
		return nil, errors.New("neither CoverBucket nor CoverBaseURL is set")
	}
	defer r.Close()

	log.Debugf(ctx, "Reading cover data")
	return ioutil.ReadAll(r)
}
