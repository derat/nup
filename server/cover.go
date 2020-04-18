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
)

const coverJPEGQuality = 95

// scaleCover reads the cover image at fn (corresponding to Song.CoverFilename),
// scales and crops it to be a square image with the supplied width and height
// size, and writes it in JPEG format to w.
//
// TODO: The source and/or dest images should probably be cached.
func scaleCover(ctx context.Context, fn string, size int, w io.Writer) error {
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

	dr := image.Rect(0, 0, size, size)
	dst := image.NewRGBA(dr)
	draw.CatmullRom.Scale(dst, dr, src, sr, draw.Src, nil)
	return jpeg.Encode(w, dst, &jpeg.Options{Quality: coverJPEGQuality})
}
