// Copyright 2020 Daniel Erat.
// All rights reserved.

package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/derat/nup/internal/pkg/mp3gain"
	"github.com/derat/nup/internal/pkg/types"
	"github.com/derat/taglib-go/taglib"
)

const (
	albumIDTag          = "MusicBrainz Album Id"
	recordingIDOwner    = "http://musicbrainz.org"
	logProgressInterval = 100
	mp3Extension        = ".mp3"
)

func readID3Footer(f *os.File, fi os.FileInfo) (length int64, artist, title, album string, err error) {
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
	paths, err := filepath.Glob(filepath.Join(dir, "*"+mp3Extension))
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
func readSong(path, relPath string, fi os.FileInfo, gain *mp3gain.Info) (*types.Song, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	s := types.Song{Filename: relPath}
	var footerLength int64
	footerLength, s.Artist, s.Title, s.Album, err = readID3Footer(f, fi)
	if err != nil {
		return nil, err
	}

	var headerLength int64
	tag, err := taglib.Decode(f, fi.Size())
	if err != nil {
		// Tolerate missing ID3v2 tags if we got an artist and title from ID3v1.
		if len(s.Artist) == 0 && len(s.Title) == 0 {
			return nil, err
		}
	} else {
		s.Artist = tag.Artist()
		s.Title = tag.Title()
		s.Album = tag.Album()
		s.AlbumID = tag.CustomFrames()[albumIDTag]
		s.RecordingID = tag.UniqueFileIdentifiers()[recordingIDOwner]
		s.Track = int(tag.Track())
		s.Disc = int(tag.Disc())
		headerLength = int64(tag.TagSize())
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
func readSongList(listPath, musicDir string, ch chan types.SongOrErr, computeGain bool) (numSongs int, err error) {
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

		if computeGain {
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

	// Now read the files asynchronously.
	for _, rel := range paths {
		go func(rel string) {
			full := filepath.Join(musicDir, rel)
			if fi, err := os.Stat(full); err != nil {
				ch <- types.NewSongOrErr(nil, err)
			} else {
				var gain *mp3gain.Info
				if gi, ok := gains[full]; ok {
					gain = &gi
				}
				s, err := readSong(full, rel, fi, gain)
				ch <- types.NewSongOrErr(s, err)
			}
		}(rel)
	}

	return len(paths), nil
}

// scanOptions contains options for scanForUpdatedSongs.
type scanOptions struct {
	computeGain bool   // use mp3gain to compute gain adjustments
	forceGlob   string // glob matching files to update even if unchanged
	logProgress bool   // periodically log progress while scanning
}

// scanForUpdatedSongs looks for songs under musicDir updated more recently than
// lastUpdateTime and asynchronously sends the resulting Song structs to ch.
// The number of songs that will be sent to the channel is returned.
func scanForUpdatedSongs(musicDir string, lastUpdateTime time.Time, ch chan types.SongOrErr,
	opts *scanOptions) (numUpdates int, err error) {
	var numMP3s int                   // total number of songs under musicDir
	var gains map[string]mp3gain.Info // keys are full paths
	var gainsDir string               // directory for gains

	err = filepath.Walk(musicDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeType != 0 || strings.ToLower(filepath.Ext(path)) != mp3Extension {
			return nil
		}
		relPath, err := filepath.Rel(musicDir, path)
		if err != nil {
			return fmt.Errorf("%v isn't subpath of %v: %v", path, musicDir, err)
		}

		numMP3s++
		if opts.logProgress && numMP3s%logProgressInterval == 0 {
			log.Printf("Progress: scanned %v files", numMP3s)
		}

		if opts.forceGlob != "" {
			if matched, err := filepath.Match(opts.forceGlob, relPath); err != nil {
				return fmt.Errorf("invalid glob %q: %v", opts.forceGlob, err)
			} else if !matched {
				return nil
			}
		} else {
			stat := fi.Sys().(*syscall.Stat_t)
			ctime := time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
			if fi.ModTime().Before(lastUpdateTime) && ctime.Before(lastUpdateTime) {
				return nil
			}
		}

		var gain *mp3gain.Info
		if opts.computeGain {
			if dir := filepath.Dir(path); gainsDir != dir {
				log.Printf("Computing gain adjustments for %v", dir)
				if gains, err = computeDirGains(dir); err != nil {
					return fmt.Errorf("failed computing gains for %v: %v", dir, err)
				}
				gainsDir = dir
			}
			if gi, ok := gains[path]; ok {
				gain = &gi
			}
		}

		go func() {
			s, err := readSong(path, relPath, fi, gain)
			ch <- types.NewSongOrErr(s, err)
		}()
		numUpdates++
		return nil
	})
	if err != nil {
		return 0, err
	}

	if opts.logProgress {
		log.Printf("Found %v update(s) among %v files.", numUpdates, numMP3s)
	}
	return numUpdates, nil
}
