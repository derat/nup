package main

import (
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

	"erat.org/nup"
	"github.com/hjfreyer/taglib-go/taglib"
)

const (
	albumIdTag          = "MusicBrainz Album Id"
	logProgressInterval = 100
	mp3Extension        = ".mp3"
)

func readId3Footer(f *os.File, fi os.FileInfo) (length int64, artist, title, album string, err error) {
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

// computeAudioSha1 returns a SHA1 hash of the audio (i.e. non-metadata) portion of f.
func computeAudioSha1(f *os.File, fi os.FileInfo, headerLength, footerLength int64) (string, error) {
	if _, err := f.Seek(headerLength, 0); err != nil {
		return "", err
	}
	hasher := sha1.New()
	if _, err := io.CopyN(hasher, f, fi.Size()-headerLength-footerLength); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// computeAudioDurationMs reads Xing data from the first frame in f to return the audio length in milliseconds.
// Only supports MPEG Audio 1, Layer 3.
// Format details at http://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header.
func computeAudioDurationMs(f *os.File, fi os.FileInfo, headerLength, footerLength int64) (int64, error) {
	if _, err := f.Seek(headerLength, 0); err != nil {
		return 0, fmt.Errorf("Unable to seek to 0x%x: %v", headerLength, err)
	}
	var header uint32
	if err := binary.Read(f, binary.BigEndian, &header); err != nil {
		return 0, fmt.Errorf("Unable to read frame header at 0x%x: %v", headerLength, err)
	}
	getBits := func(startBit, numBits uint) uint32 {
		return (header << startBit) >> (32 - numBits)
	}
	if getBits(0, 11) != 0x7ff {
		return 0, fmt.Errorf("Missing sync at 0x%x (got 0x%x instead of 0x7ff)", headerLength, getBits(0, 11))
	}
	if getBits(11, 2) != 0x3 {
		return 0, fmt.Errorf("Unsupported MPEG Audio version at 0x%x (got 0x%x instead of 0x3)", headerLength, getBits(11, 2))
	}
	if getBits(13, 2) != 0x1 {
		return 0, fmt.Errorf("Unsupported layer at 0%x (got 0x%x instead of 0x1)", headerLength, getBits(13, 2))
	}

	// This table is specific to MPEG 1, Layer 3.
	var kbitRates = [...]int64{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	kbitRate := kbitRates[getBits(16, 4)]
	if kbitRate == 0 {
		return 0, fmt.Errorf("Unsupported bitrate at 0x%x (got index %d)", headerLength, getBits(16, 4))
	}

	// This table is specific to MPEG 1.
	var sampleRates = [...]int64{44100, 48000, 32000, 0}
	sampleRate := sampleRates[getBits(20, 2)]
	if sampleRate == 0 {
		return 0, fmt.Errorf("Unsupported sample rate at 0x%x (got index %d)", headerLength, getBits(20, 2))
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
		return 0, fmt.Errorf("Unable to read Xing header at 0x%x: %v", xingHeaderStart, err)
	}
	xingHeaderName := string(b[0:4])
	if xingHeaderName == "Xing" || xingHeaderName == "Info" {
		r := bytes.NewReader(b[4:])
		var xingFlags uint32
		binary.Read(r, binary.BigEndian, &xingFlags)
		if xingFlags&0x1 == 0x0 {
			return 0, fmt.Errorf("Xing header at 0x%x lacks number of frames", xingHeaderStart)
		}
		var numFrames uint32
		binary.Read(r, binary.BigEndian, &numFrames)

		// This constant is specific to MPEG 1, Layer 3.
		const samplesPerFrame = 1152
		return int64(samplesPerFrame) * int64(numFrames) * 1000 / sampleRate, nil
	}

	// Okay, no Xing VBR header. Assume that the file has a fixed bitrate.
	// (The other alternative is to read the whole file to count the number of frames.)
	return (fi.Size() - headerLength - footerLength) / kbitRate * 8, nil
}

func readFileDetails(path, relPath string, fi os.FileInfo, updateChan chan nup.SongOrErr) {
	s := &nup.Song{Filename: relPath}
	var err error
	defer func() { updateChan <- nup.SongOrErr{s, err} }()

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	var footerLength int64
	footerLength, s.Artist, s.Title, s.Album, err = readId3Footer(f, fi)
	if err != nil {
		return
	}

	var headerLength int64
	tag, err := taglib.Decode(f, fi.Size())
	if err != nil {
		// Tolerate missing ID3v2 tags if we got an artist and title from ID3v1.
		if len(s.Artist) == 0 && len(s.Title) == 0 {
			return
		}
		err = nil
	} else {
		s.Artist = tag.Artist()
		s.Title = tag.Title()
		s.Album = tag.Album()
		s.AlbumId = tag.CustomFrames()[albumIdTag]
		s.Track = int(tag.Track())
		s.Disc = int(tag.Disc())
		headerLength = int64(tag.TagSize())
	}

	s.Sha1, err = computeAudioSha1(f, fi, headerLength, footerLength)
	if err != nil {
		return
	}
	lengthMs, err := computeAudioDurationMs(f, fi, headerLength, footerLength)
	if err != nil {
		return
	}
	s.Length = float64(lengthMs) / 1000
}

func getSongByPath(musicDir, relPath string, updateChan chan nup.SongOrErr) {
	p := filepath.Join(musicDir, relPath)
	fi, err := os.Stat(p)
	if err != nil {
		updateChan <- nup.SongOrErr{nil, err}
		return
	}
	readFileDetails(p, relPath, fi, updateChan)
}

func scanForUpdatedSongs(musicDir, forceGlob string, lastUpdateTime time.Time, updateChan chan nup.SongOrErr, logProgress bool) (numUpdates int, err error) {
	numMp3s := 0
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

		numMp3s++
		if logProgress && numMp3s%logProgressInterval == 0 {
			log.Printf("Progress: scanned %v files\n", numMp3s)
		}

		if len(forceGlob) > 0 {
			if matched, err := filepath.Match(forceGlob, relPath); err != nil {
				return fmt.Errorf("Invalid glob %v: %v", forceGlob, err)
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

		numUpdates++
		go readFileDetails(path, relPath, fi, updateChan)
		return nil
	})
	if err != nil {
		return 0, err
	}

	if logProgress {
		log.Printf("Found %v update(s) among %v files.\n", numUpdates, numMp3s)
	}
	return numUpdates, nil
}
