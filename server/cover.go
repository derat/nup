package main

import (
	"context"
	"errors"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"

	"cloud.google.com/go/storage"

	"golang.org/x/image/draw"

	"google.golang.org/appengine/log"
)

// scaleCover reads the cover image at fn (corresponding to Song.CoverFilename),
// scales and crops it to be a square image with the supplied width and height
// size, and writes it in JPEG format to w.
//
// TODO: The source and/or dest images should probably be cached.
func scaleCover(ctx context.Context, fn string, size, quality int, w io.Writer) error {
	log.Debugf(ctx, "Opening cover file %q", fn)
	var r io.ReadCloser
	if cfg := getConfig(ctx); cfg.CoverBucket != "" {
		client, err := storage.NewClient(ctx)
		if err != nil {
			return err
		}
		defer client.Close()
		if r, err = client.Bucket(cfg.CoverBucket).Object(fn).NewReader(ctx); err != nil {
			return err
		}
	} else if cfg.CoverBaseURL != "" {
		resp, err := http.Get(cfg.CoverBaseURL + fn)
		if err != nil {
			return err
		}
		r = resp.Body
	} else {
		return errors.New("neither CoverBucket nor CoverBaseURL is set")
	}
	defer r.Close()

	log.Debugf(ctx, "Decoding cover image")
	src, _, err := image.Decode(r)
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

	// TODO: Would it be best to never upscale?

	log.Debugf(ctx, "Scaling cover image from %vx%v to %vx%v",
		sr.Dx(), sr.Dy(), size, size)
	dr := image.Rect(0, 0, size, size)
	dst := image.NewRGBA(dr)
	// draw.CatmullRom seems to be very slow. I've seen a Scale call from
	// 1200x1200 to 512x512 take 908 ms on AppEngine.
	draw.ApproxBiLinear.Scale(dst, dr, src, sr, draw.Src, nil)

	log.Debugf(ctx, "Encoding cover image to JPEG")
	return jpeg.Encode(w, dst, &jpeg.Options{Quality: quality})
}
