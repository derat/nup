package main

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

func getImageSize(path string) (imageSize, error) {
	f, err := os.Open(path)
	if err != nil {
		return imageSize{}, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return imageSize{}, err
	}

	return imageSize{img.Bounds().Max.X - img.Bounds().Min.X, img.Bounds().Max.Y - img.Bounds().Min.Y}, nil
}
