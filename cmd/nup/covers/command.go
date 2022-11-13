// Copyright 2020 Daniel Erat.
// All rights reserved.

package covers

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/derat/nup/cmd/nup/client"
	srvcover "github.com/derat/nup/server/cover"
	"github.com/derat/nup/server/db"
	"github.com/derat/taglib-go/taglib"
	"github.com/google/subcommands"
)

const (
	logInterval = 100
	albumIDTag  = "MusicBrainz Album Id"
	coverExt    = ".jpg"
)

type Command struct {
	Cfg *client.Config

	coverDir     string // directory containing cover images
	download     bool   // download image covers to coverDir
	generateWebP bool   // generate WebP versions of covers in coverDir
	maxSongs     int    // songs to inspect
	maxRequests  int    // parallel HTTP requests
	size         int    // image size to download (250, 500, 1200)
}

func (*Command) Name() string     { return "covers" }
func (*Command) Synopsis() string { return "manage album art" }
func (*Command) Usage() string {
	return `covers [flags]:
	Works with album art images in a directory.
	With -download, downloads album art from coverartarchive.org.
	With -generate-webp, generates WebP versions of existing JPEG images.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.coverDir, "cover-dir", "", "Directory containing cover images")
	f.BoolVar(&cmd.download, "download", false,
		"Download covers for dumped songs read from stdin or positional song files to -cover-dir")
	f.IntVar(&cmd.size, "download-size", 1200, "Image size to download (250, 500, or 1200)")
	f.BoolVar(&cmd.generateWebP, "generate-webp", false, "Generate WebP versions of covers in -cover-dir")
	f.IntVar(&cmd.maxSongs, "max-downloads", -1, "Maximum number of songs to inspect for -download")
	f.IntVar(&cmd.maxRequests, "max-requests", 2, "Maximum number of parallel HTTP requests for -download")
}

func (cmd *Command) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if cmd.coverDir == "" {
		fmt.Fprintln(os.Stderr, "-cover-dir must be supplied")
		return subcommands.ExitUsageError
	}

	switch {
	case cmd.download:
		if err := cmd.doDownload(fs.Args()); err != nil {
			fmt.Fprintln(os.Stderr, "Failed downloading covers:", err)
			return subcommands.ExitFailure
		}
		return subcommands.ExitSuccess
	case cmd.generateWebP:
		if err := cmd.doGenerateWebP(); err != nil {
			fmt.Fprintln(os.Stderr, "Failed generating WebP images:", err)
			return subcommands.ExitFailure
		}
		return subcommands.ExitSuccess
	default:
		fmt.Fprintln(os.Stderr, "Must supply one of -download and -generate-webp")
		return subcommands.ExitUsageError
	}
}

func (cmd *Command) doDownload(paths []string) error {
	albumIDs := make([]string, 0)
	if len(paths) > 0 {
		ids := make(map[string]struct{})
		for _, p := range paths {
			if id, err := readSong(p); err != nil {
				return err
			} else if len(id) > 0 {
				log.Printf("%v has album ID %v", p, id)
				ids[id] = struct{}{}
			}
		}
		for id, _ := range ids {
			albumIDs = append(albumIDs, id)
		}
	} else {
		log.Print("Reading songs from stdin")
		var err error
		if albumIDs, err = readDumpedSongs(os.Stdin, cmd.coverDir, cmd.maxSongs); err != nil {
			return err
		}
	}

	log.Printf("Downloading cover(s) for %v album(s)", len(albumIDs))
	downloadCovers(albumIDs, cmd.coverDir, cmd.size, cmd.maxRequests)
	return nil
}

func (cmd *Command) doGenerateWebP() error {
	return filepath.Walk(cmd.coverDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() || !strings.HasSuffix(p, coverExt) {
			return nil
		}

		var width, height int
		for _, size := range srvcover.WebPSizes {
			// Skip this size if we already have it and it's up-to-date.
			gp := srvcover.WebPFilename(p, size)
			if gfi, err := os.Stat(gp); err == nil && !fi.ModTime().After(gfi.ModTime()) {
				continue
			}
			// Read the source image's dimensions if we haven't already.
			if width == 0 && height == 0 {
				if width, height, err = getDimensions(p); err != nil {
					return fmt.Errorf("failed getting %q dimensions: %v", p, err)
				}
			}
			if err := writeWebP(p, gp, width, height, size); err != nil {
				return fmt.Errorf("failed converting %q to %q: %v", p, gp, err)
			}
		}
		return nil
	})
}

// getDimensions returns the dimensions of the JPEG image at p.
func getDimensions(p string) (width, height int, err error) {
	f, err := os.Open(p)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	return cfg.Width, cfg.Height, err
}

// writeWebP writes the JPEG image at srcPath of the supplied dimensions
// to destPath in WebP format and with the supplied (square) size.
// The image is cropped and scaled as per the Scale function in server/cover/cover.go.
func writeWebP(srcPath string, destPath string, srcWidth, srcHeight int, destSize int) error {
	args := []string{
		"-mt", // multithreaded
		"-resize", strconv.Itoa(destSize), strconv.Itoa(destSize),
	}

	// Crop the source image rect if it isn't square.
	if srcWidth > srcHeight {
		args = append(args, "-crop", strconv.Itoa((srcWidth-srcHeight)/2), "0",
			strconv.Itoa(srcHeight), strconv.Itoa(srcHeight))
	} else if srcHeight > srcWidth {
		args = append(args, "-crop", "0", strconv.Itoa((srcHeight-srcWidth)/2),
			strconv.Itoa(srcWidth), strconv.Itoa(srcWidth))
	}

	// TODO: cwebp dies with "Unsupported color conversion request" when given
	// a JPEG with a CMYK (rather than RGB) color space:
	// https://groups.google.com/a/webmproject.org/g/webp-discuss/c/MH8q_d6M1vM
	// This can be fixed with e.g. "convert -colorspace RGB old.jpg new.jpg", but
	// CMYK images seem to be rare enough that I haven't bothered automating this.
	args = append(args, "-o", destPath, srcPath)
	err := exec.Command("cwebp", args...).Run()
	// TODO: It'd probably be safer to write to a temp file and then rename, since it'd
	// still be possible for us to die before we can unlink the dest file. If a partial
	// file is written, it probably won't be replaced in future runs due to its timestamp.
	if err != nil {
		os.Remove(destPath)
	}
	return err
}

// TODO: This code... isn't great. Make it share more with the update subcommand?
func readSong(path string) (albumID string, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	tag, err := taglib.Decode(f, fi.Size())
	if err != nil {
		return "", err
	}
	return tag.CustomFrames()[albumIDTag], nil
}

func readDumpedSongs(r io.Reader, coverDir string, maxSongs int) (albumIDs []string, err error) {
	missingAlbumIDs := make(map[string]struct{})
	d := json.NewDecoder(r)
	numSongs := 0
	for {
		if maxSongs >= 0 && numSongs >= maxSongs {
			break
		}

		s := db.Song{}
		if err = d.Decode(&s); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		numSongs++

		if numSongs%logInterval == 0 {
			log.Printf("Scanned %v songs", numSongs)
		}

		// Can't do anything if the song doesn't have an album ID.
		if len(s.AlbumID) == 0 {
			continue
		}

		// Check if we already have the cover.
		if _, err := os.Stat(filepath.Join(coverDir, s.AlbumID+coverExt)); err == nil {
			continue
		}

		missingAlbumIDs[s.AlbumID] = struct{}{}
	}
	if numSongs%logInterval != 0 {
		log.Printf("Scanned %v songs", numSongs)
	}

	ret := make([]string, len(missingAlbumIDs))
	i := 0
	for id := range missingAlbumIDs {
		ret[i] = id
		i++
	}
	return ret, nil
}

// downloadCover downloads cover art for albumID into dir.
// If the cover was not found, path is empty and err is nil.
func downloadCover(albumID, dir string, size int) (path string, err error) {
	url := fmt.Sprintf("https://coverartarchive.org/release/%s/front-%d", albumID, size)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("Fetching %v failed: %v", url, err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		if resp.StatusCode == 404 {
			return "", nil
		}
		return "", fmt.Errorf("Got %v when fetching %v", resp.StatusCode, url)
	}
	defer resp.Body.Close()

	path = filepath.Join(dir, albumID+coverExt)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err = io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("Failed to read from %v: %v", url, err)
	}
	return path, nil
}

func downloadCovers(albumIDs []string, dir string, size, maxRequests int) {
	cache := client.NewTaskCache(maxRequests)
	wg := sync.WaitGroup{}
	wg.Add(len(albumIDs))

	for _, id := range albumIDs {
		go func(id string) {
			if path, err := cache.Get(id, id, func() (map[string]interface{}, error) {
				if p, err := downloadCover(id, dir, size); err != nil {
					return nil, err
				} else {
					return map[string]interface{}{id: p}, nil
				}
			}); err != nil {
				log.Printf("Failed to get %v: %v", id, err)
			} else if len(path.(string)) == 0 {
				log.Printf("Didn't find %v", id)
			} else {
				log.Printf("Wrote %v", path.(string))
			}
			wg.Done()
		}(id)
	}
	wg.Wait()
}
