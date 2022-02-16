// Copyright 2020 Daniel Erat.
// All rights reserved.

package update

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/cmd/nup/mp3gain"
	"github.com/derat/nup/server/db"
	"github.com/derat/taglib-go/taglib"
	"github.com/derat/taglib-go/taglib/id3"
)

const (
	albumIDTag          = "MusicBrainz Album Id" // usually used as cover ID
	coverIDTag          = "nup Cover Id"         // can be set for non-MusicBrainz tracks
	recordingIDOwner    = "http://musicbrainz.org"
	albumArtistTag      = "TPE2"
	logProgressInterval = 100
)

// readID3v1Footer reads a 128-byte ID3v1 footer from the end of f.
// ID3v1 is a terrible format.
func readID3v1Footer(f *os.File, fi os.FileInfo) (length int64, artist, title, album string, err error) {
	const (
		footerLength = 128
		footerMagic  = "TAG"
		titleLength  = 30
		artistLength = 30
		albumLength  = 30
		// TODO: Add year (4 bytes), comment (30), and genre (1) if I ever care about them.
	)

	// Check for an ID3v1 footer.
	buf := make([]byte, footerLength)
	if _, err := f.ReadAt(buf, fi.Size()-int64(len(buf))); err != nil {
		return 0, "", "", "", err
	}

	b := bytes.NewBuffer(buf)
	if string(b.Next(len(footerMagic))) != footerMagic {
		return 0, "", "", "", nil
	}

	clean := func(b []byte) string { return string(bytes.TrimSpace(bytes.TrimRight(b, "\x00"))) }
	title = clean(b.Next(titleLength))
	artist = clean(b.Next(artistLength))
	album = clean(b.Next(albumLength))
	return footerLength, artist, title, album, nil
}

// getID3v2TextFrame returns the first ID3v2 text frame with the supplied ID from gen.
// If the frame isn't present, an empty string and nil error are returned.
//
// The taglib library has built-in support for some frames ("TPE1", "TIT2", "TALB", etc.)
// and provides generic support for custom "TXXX" frames, but it doesn't seem to provide
// an easy way to read other well-known frames like "TPE2".
func getID3v2TextFrame(gen taglib.GenericTag, id string) (string, error) {
	switch tag := gen.(type) {
	case *id3.Id3v23Tag:
		if frames := tag.Frames[id]; len(frames) == 0 {
			return "", nil
		} else if fields, err := id3.GetId3v23TextIdentificationFrame(frames[0]); err != nil {
			return "", err
		} else {
			return fields[0], nil
		}
	case *id3.Id3v24Tag:
		if frames := tag.Frames[id]; len(frames) == 0 {
			return "", nil
		} else if fields, err := id3.GetId3v24TextIdentificationFrame(frames[0]); err != nil {
			return "", err
		} else {
			return fields[0], nil
		}
	default:
		return "", errors.New("unsupported ID3 version")
	}
}

// computeAudioSHA1 returns a SHA1 hash of the audio (i.e. non-metadata) portion of f.
func computeAudioSHA1(f *os.File, fi os.FileInfo, headerLength, footerLength int64) (string, error) {
	if _, err := f.Seek(headerLength, 0); err != nil {
		return "", err
	}
	hasher := sha1.New()
	if _, err := io.CopyN(hasher, f, fi.Size()-headerLength-footerLength); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// computeAudioDuration reads Xing data from the first frame in f to return the audio length.
// Only supports MPEG Audio 1, Layer 3.
// Format details at http://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header.
func computeAudioDuration(f *os.File, fi os.FileInfo, headerLength, footerLength int64) (time.Duration, error) {
	if _, err := f.Seek(headerLength, 0); err != nil {
		return 0, fmt.Errorf("unable to seek to %#x: %v", headerLength, err)
	}
	var header uint32
	if err := binary.Read(f, binary.BigEndian, &header); err != nil {
		return 0, fmt.Errorf("unable to read frame header at %#x: %v", headerLength, err)
	}
	getBits := func(startBit, numBits uint) uint32 {
		return (header << startBit) >> (32 - numBits)
	}
	if getBits(0, 11) != 0x7ff {
		return 0, fmt.Errorf("missing sync at %#x (got %#x instead of 0x7ff)", headerLength, getBits(0, 11))
	}
	if getBits(11, 2) != 0x3 {
		return 0, fmt.Errorf("unsupported MPEG Audio version at %#x (got %#x instead of 0x3)", headerLength, getBits(11, 2))
	}
	if getBits(13, 2) != 0x1 {
		return 0, fmt.Errorf("unsupported layer at %#x (got %#x instead of 0x1)", headerLength, getBits(13, 2))
	}

	// This table is specific to MPEG 1, Layer 3.
	var kbitRates = [...]int64{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	kbitRate := kbitRates[getBits(16, 4)]
	if kbitRate == 0 {
		return 0, fmt.Errorf("unsupported bitrate at %#x (got index %d)", headerLength, getBits(16, 4))
	}

	// This table is specific to MPEG 1.
	var sampleRates = [...]int64{44100, 48000, 32000, 0}
	sampleRate := sampleRates[getBits(20, 2)]
	if sampleRate == 0 {
		return 0, fmt.Errorf("unsupported sample rate at %#x (got index %d)", headerLength, getBits(20, 2))
	}

	xingHeaderStart := headerLength + 4
	// Skip "side information".
	if getBits(24, 2) == 0x3 { // Channel mode; 0x3 is mono.
		xingHeaderStart += 17
	} else {
		xingHeaderStart += 32
	}
	// Skip 16-bit CRC if present.
	if getBits(15, 1) == 0x0 { // 0x0 means "has protection".
		xingHeaderStart += 2
	}

	b := make([]byte, 12, 12)
	if _, err := f.ReadAt(b, xingHeaderStart); err != nil {
		return 0, fmt.Errorf("unable to read Xing header at %#x: %v", xingHeaderStart, err)
	}
	xingHeaderName := string(b[0:4])
	if xingHeaderName == "Xing" || xingHeaderName == "Info" {
		r := bytes.NewReader(b[4:])
		var xingFlags uint32
		binary.Read(r, binary.BigEndian, &xingFlags)
		if xingFlags&0x1 == 0x0 {
			return 0, fmt.Errorf("Xing header at %#x lacks number of frames", xingHeaderStart)
		}
		var numFrames uint32
		binary.Read(r, binary.BigEndian, &numFrames)

		// This constant is specific to MPEG 1, Layer 3.
		const samplesPerFrame = 1152
		ms := int64(samplesPerFrame) * int64(numFrames) * 1000 / sampleRate
		return time.Duration(ms) * time.Millisecond, nil
	}

	// Okay, no Xing VBR header. Assume that the file has a fixed bitrate.
	// (The other alternative is to read the whole file to count the number of frames.)
	ms := (fi.Size() - headerLength - footerLength) / kbitRate * 8
	return time.Duration(ms) * time.Millisecond, nil
}

// computeDirGains computes gain adjustments for all MP3 files in dir.
func computeDirGains(dir string) (map[string]mp3gain.Info, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.[mM][pP]3")) // case-insensitive :-/
	if err != nil {
		return nil, err
	}

	// Group files by album.
	albums := make(map[string][]string) // paths grouped by album ID
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			return nil, err
		}
		if !fi.Mode().IsRegular() {
			continue
		}

		// TODO: Consider caching tags so we don't need to decode again in readSong.
		// In practice, computing gains is so incredibly slow (at least on my computer)
		// that caching probably doesn't matter in the big scheme of things.
		var album string
		if tag, err := taglib.Decode(f, fi.Size()); err == nil {
			album = tag.CustomFrames()[albumIDTag]
		}
		// If we didn't get an album ID, just use the path so the file will be in its own group.
		if album == "" {
			album = p
		}
		albums[album] = append(albums[album], p)
	}

	// Compute gains for each album.
	gains := make(map[string]mp3gain.Info, len(paths))
	for _, paths := range albums {
		infos, err := mp3gain.ComputeAlbum(paths)
		if err != nil {
			return nil, err
		}
		for p, info := range infos {
			gains[p] = info
		}
	}
	return gains, nil
}

// readSong creates a Song for the file at the supplied path.
func readSong(path, relPath string, fi os.FileInfo, gain *mp3gain.Info,
	artistRewrites map[string]string) (*db.Song, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := db.Song{Filename: relPath}
	var footerLength int64
	footerLength, s.Artist, s.Title, s.Album, err = readID3v1Footer(f, fi)
	if err != nil {
		return nil, err
	}

	var headerLength int64
	if tag, err := taglib.Decode(f, fi.Size()); err != nil {
		// Tolerate missing ID3v2 tags if we got an artist and title from ID3v1.
		if len(s.Artist) == 0 && len(s.Title) == 0 {
			return nil, err
		}
	} else {
		s.Artist = tag.Artist()
		s.Title = tag.Title()
		s.Album = tag.Album()
		s.AlbumID = tag.CustomFrames()[albumIDTag]
		s.CoverID = tag.CustomFrames()[coverIDTag]
		s.RecordingID = tag.UniqueFileIdentifiers()[recordingIDOwner]
		s.Track = int(tag.Track())
		s.Disc = int(tag.Disc())
		headerLength = int64(tag.TagSize())

		// Only save the album artist if it's different from the track artist.
		if aa, err := getID3v2TextFrame(tag, albumArtistTag); err != nil {
			return nil, err
		} else if aa != s.Artist {
			s.AlbumArtist = aa
		}
	}

	if repl, ok := artistRewrites[s.Artist]; ok {
		s.Artist = repl
	}

	s.SHA1, err = computeAudioSHA1(f, fi, headerLength, footerLength)
	if err != nil {
		return nil, err
	}
	dur, err := computeAudioDuration(f, fi, headerLength, footerLength)
	if err != nil {
		return nil, err
	}
	s.Length = dur.Seconds()

	if gain != nil {
		s.TrackGain = gain.TrackGain
		s.AlbumGain = gain.AlbumGain
		s.PeakAmp = gain.PeakAmp
	}
	return &s, nil
}

// readSongList reads a list of relative (to musicDir) paths from listPath
// and asynchronously sends the resulting Song structs to ch.
// The number of songs that will be sent to the channel is returned.
func readSongList(listPath, musicDir string, ch chan songOrErr,
	opts *scanOptions) (numSongs int, err error) {
	f, err := os.Open(listPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var paths []string                     // relative paths
	gains := make(map[string]mp3gain.Info) // keyed by full path

	// Read the list synchronously first so we can compute all of the gain adjustments if needed.
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		rel := sc.Text()
		paths = append(paths, rel)

		if opts.computeGain {
			if opts.dumpedGains != nil {
				gi, ok := opts.dumpedGains[rel]
				if !ok {
					return 0, fmt.Errorf("no dumped gain info for %q", rel)
				}
				gains[filepath.Join(musicDir, rel)] = gi
			} else {
				// Scan the file's directory if we don't already have its gain info.
				full := filepath.Join(musicDir, rel)
				if _, ok := gains[full]; !ok {
					dir := filepath.Dir(full)
					log.Printf("Computing gain adjustments for %v", dir)
					m, err := computeDirGains(dir)
					if err != nil {
						return 0, err
					}
					for p, gi := range m {
						gains[p] = gi
					}
				}
			}
		}
	}

	// Now read the files asynchronously.
	for _, rel := range paths {
		go func(rel string) {
			full := filepath.Join(musicDir, rel)
			if fi, err := os.Stat(full); err != nil {
				ch <- songOrErr{nil, err}
			} else {
				var gain *mp3gain.Info
				if gi, ok := gains[full]; ok {
					gain = &gi
				}
				s, err := readSong(full, rel, fi, gain, opts.artistRewrites)
				ch <- songOrErr{s, err}
			}
		}(rel)
	}

	return len(paths), nil
}

// scanOptions contains options for scanForUpdatedSongs and readSongList.
// Some of the options aren't used by readSongList.
type scanOptions struct {
	computeGain    bool                    // use mp3gain to compute gain adjustments
	forceGlob      string                  // glob matching files to update even if unchanged
	logProgress    bool                    // periodically log progress while scanning
	artistRewrites map[string]string       // artist names from tags to rewrite
	dumpedGains    map[string]mp3gain.Info // precomputed gains keyed by Song.Filename
}

// scanForUpdatedSongs looks for songs under musicDir updated more recently than lastUpdateTime or
// in directories not listed in lastUpdateDirs and asynchronously sends the resulting Song structs
// to ch. The number of songs that will be sent to the channel and seen directories (relative to
// musicDir) are returned.
func scanForUpdatedSongs(musicDir string, lastUpdateTime time.Time, lastUpdateDirs []string,
	ch chan songOrErr, opts *scanOptions) (numUpdates int, seenDirs []string, err error) {
	var numMP3s int                   // total number of songs under musicDir
	var gains map[string]mp3gain.Info // keys are full paths
	var gainsDir string               // directory for gains

	oldDirs := make(map[string]struct{}, len(lastUpdateDirs))
	for _, d := range lastUpdateDirs {
		oldDirs[d] = struct{}{}
	}
	newDirs := make(map[string]struct{})

	err = filepath.Walk(musicDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() || !client.IsMusicPath(path) {
			return nil
		}
		relPath, err := filepath.Rel(musicDir, path)
		if err != nil {
			return fmt.Errorf("%q isn't subpath of %q: %v", path, musicDir, err)
		}

		numMP3s++
		if opts.logProgress && numMP3s%logProgressInterval == 0 {
			log.Printf("Scanned %v files", numMP3s)
		}

		relDir := filepath.Dir(relPath)
		newDirs[relDir] = struct{}{}

		if opts.forceGlob != "" {
			if matched, err := filepath.Match(opts.forceGlob, relPath); err != nil {
				return fmt.Errorf("invalid glob %q: %v", opts.forceGlob, err)
			} else if !matched {
				return nil
			}
		} else {
			// Bail out if the file isn't new and we saw its directory in the last update.
			// We need to check for new directories to handle the situation described at
			// https://github.com/derat/nup/issues/22 where a directory containing files
			// with old timestamps is moved into the tree.
			stat := fi.Sys().(*syscall.Stat_t)
			ctime := time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
			oldFile := fi.ModTime().Before(lastUpdateTime) && ctime.Before(lastUpdateTime)
			_, oldDir := oldDirs[relDir]

			// Handle old configs that don't include previously-seen directories.
			if len(oldDirs) == 0 {
				oldDir = true
			}

			if oldFile && oldDir {
				return nil
			}
		}

		var gain *mp3gain.Info
		if opts.computeGain {
			if opts.dumpedGains != nil {
				gi, ok := opts.dumpedGains[relPath]
				if !ok {
					return fmt.Errorf("no dumped gain info for %q", relPath)
				}
				gain = &gi
			} else {
				// Compute the gains for this directory if we didn't already do it.
				if dir := filepath.Dir(path); gainsDir != dir {
					log.Printf("Computing gain adjustments for %v", dir)
					if gains, err = computeDirGains(dir); err != nil {
						return fmt.Errorf("failed computing gains for %q: %v", dir, err)
					}
					gainsDir = dir
				}
				gi, ok := gains[path]
				if !ok {
					return fmt.Errorf("missing gain info for %q after computing", relPath)
				}
				gain = &gi
			}
		}

		go func() {
			s, err := readSong(path, relPath, fi, gain, opts.artistRewrites)
			ch <- songOrErr{s, err}
		}()
		numUpdates++
		return nil
	})
	if err != nil {
		return 0, nil, err
	}

	if opts.logProgress {
		log.Printf("Found %v update(s) among %v files", numUpdates, numMP3s)
	}
	for d := range newDirs {
		seenDirs = append(seenDirs, d)
	}
	sort.Strings(seenDirs)
	return numUpdates, seenDirs, nil
}
