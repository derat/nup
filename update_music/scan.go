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
	mp3Extension        = ".mp3"
	logProgressInterval = 100
)

func getId3FooterLength(f *os.File, fi os.FileInfo) (int64, error) {
	const (
		length = 128
		magic  = "TAG"
	)

	// Check for an ID3v1 footer.
	buf := make([]byte, len(magic), len(magic))
	if _, err := f.ReadAt(buf, fi.Size()-length); err != nil {
		return 0, err
	}
	if string(buf) == magic {
		return length, nil
	}
	return 0, nil
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
		return 0, fmt.Errorf("Unable to seek to %v: %v", headerLength, err)
	}
	var header uint32
	if err := binary.Read(f, binary.BigEndian, &header); err != nil {
		return 0, fmt.Errorf("Unable to read frame header at %v: %v", headerLength, err)
	}
	getBits := func(startBit, numBits uint) uint32 {
		return (header << startBit) >> (32 - numBits)
	}
	if getBits(0, 11) != 0x7ff {
		return 0, fmt.Errorf("Missing sync at %v", headerLength)
	}
	if getBits(11, 2) != 0x3 {
		return 0, fmt.Errorf("Unsupported MPEG Audio version at %v", headerLength)
	}
	if getBits(13, 2) != 0x1 {
		return 0, fmt.Errorf("Unsupported layer at %v", headerLength)
	}

	// This table is specific to MPEG 1, Layer 3.
	var kbitRates = [...]int64{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	kbitRate := kbitRates[getBits(16, 4)]
	if kbitRate == 0 {
		return 0, fmt.Errorf("Unsupported bitrate at %v", headerLength)
	}

	// This table is specific to MPEG 1.
	var sampleRates = [...]int64{44100, 48000, 32000, 0}
	sampleRate := sampleRates[getBits(20, 2)]
	if sampleRate == 0 {
		return 0, fmt.Errorf("Unsupported sample rate at %v", headerLength)
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
		return 0, fmt.Errorf("Unable to read Xing header at %v: %v", xingHeaderStart, err)
	}
	xingHeaderName := string(b[0:4])
	if xingHeaderName == "Xing" || xingHeaderName == "Info" {
		r := bytes.NewReader(b[4:])
		var xingFlags uint32
		binary.Read(r, binary.BigEndian, &xingFlags)
		if xingFlags&0x1 == 0x0 {
			return 0, fmt.Errorf("Xing header at %v lacks number of frames", xingHeaderStart)
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

func readFileDetails(p, musicDir string, fi os.FileInfo, updateChan chan SongAndError) {
	s := &nup.Song{}
	var err error
	defer func() { updateChan <- SongAndError{s, err} }()

	s.Filename, err = filepath.Rel(musicDir, p)
	if err != nil {
		return
	}

	var f *os.File
	f, err = os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()

	var tag taglib.GenericTag
	tag, err = taglib.Decode(f, fi.Size())
	if err != nil {
		return
	}
	s.Artist = tag.Artist()
	s.Title = tag.Title()
	s.Album = tag.Album()
	s.Track = int(tag.Track())
	s.Disc = int(tag.Disc())

	var footerLength int64
	footerLength, err = getId3FooterLength(f, fi)
	if err != nil {
		return
	}
	s.Sha1, err = computeAudioSha1(f, fi, int64(tag.TagSize()), footerLength)
	if err != nil {
		return
	}
	lengthMs, err := computeAudioDurationMs(f, fi, int64(tag.TagSize()), footerLength)
	if err != nil {
		return
	}
	s.Length = float64(lengthMs) / 1000
}

func scanForUpdatedSongs(musicDir string, lastUpdateTime time.Time, updateChan chan SongAndError) (numUpdates int, err error) {
	numMp3s := 0
	err = filepath.Walk(musicDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeType != 0 || strings.ToLower(filepath.Ext(p)) != mp3Extension {
			return nil
		}

		numMp3s++
		if numMp3s%logProgressInterval == 0 {
			log.Printf("Progress: scanned %v files\n", numMp3s)
		}

		stat := fi.Sys().(*syscall.Stat_t)
		ctime := time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
		if fi.ModTime().After(lastUpdateTime) || ctime.After(lastUpdateTime) {
			numUpdates++
			go readFileDetails(p, musicDir, fi, updateChan)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	log.Printf("Found %v update(s) among %v files.\n", numUpdates, numMp3s)
	return numUpdates, nil
}
