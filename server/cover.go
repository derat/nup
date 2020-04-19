package main

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

	"cloud.google.com/go/storage"
	"github.com/derat/nup/types"

	"golang.org/x/image/draw"

	"google.golang.org/appengine/log"
)

// scaleCover reads the cover image at fn (corresponding to Song.CoverFilename),
// scales and crops it to be a square image with the supplied width and height
// size, and writes it in JPEG format to w.
func scaleCover(ctx context.Context, fn string, size, quality int, w io.Writer) error {
	var data []byte
	var err error

	useCache := getConfig(ctx).CacheCovers == types.MemcacheCaching
	if useCache {
		log.Debugf(ctx, "Checking cache for scaled cover")
		if data, err = getCoverFromMemcache(ctx, fn, size); len(data) > 0 {
			log.Debugf(ctx, "Writing %d-byte cached scaled cover", len(data))
			_, err = w.Write(data)
			return err
		}
		log.Debugf(ctx, "Checking cache for original cover")
		if data, err = getCoverFromMemcache(ctx, fn, 0); len(data) > 0 {
			log.Debugf(ctx, "Got %d-byte cached original cover", len(data))
		} else if err != nil {
			log.Errorf(ctx, "Cache lookup failed: %v", err) // swallow error
		}
	}

	if len(data) == 0 {
		log.Debugf(ctx, "Loading original cover")
		if data, err = loadCover(ctx, fn); err != nil {
			return fmt.Errorf("failed to read cover: %v", err)
		}
		if useCache {
			log.Debugf(ctx, "Caching %v-byte original cover", len(data))
			if err = writeCoverToMemcache(ctx, fn, 0, data); err != nil {
				log.Errorf(ctx, "Cache write failed: %v", err) // swallow error
			}
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
	// 1200x1200 to 512x512 take 908 ms on AppEngine.
	draw.ApproxBiLinear.Scale(dst, dr, src, sr, draw.Src, nil)

	var b bytes.Buffer
	if useCache {
		w = io.MultiWriter(w, &b)
	}
	log.Debugf(ctx, "JPEG-encoding scaled image")
	if err := jpeg.Encode(w, dst, &jpeg.Options{Quality: quality}); err != nil {
		return err
	}
	if useCache {
		log.Debugf(ctx, "Caching %v-byte scaled cover", b.Len())
		if err = writeCoverToMemcache(ctx, fn, size, b.Bytes()); err != nil {
			log.Errorf(ctx, "Cache write failed: %v", err) // swallow error
		}
	}
	return nil
}

// loadCover loads and returns the cover image with the supplied original
// filename (see Song.CoverFilename).
func loadCover(ctx context.Context, fn string) ([]byte, error) {
	var r io.ReadCloser
	if cfg := getConfig(ctx); cfg.CoverBucket != "" {
		log.Debugf(ctx, "Opening object %q from bucket %q", fn, cfg.CoverBucket)
		client, err := storage.NewClient(ctx)
		if err != nil {
			return nil, err
		}
		defer client.Close()
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
