// Copyright 2022 Daniel Erat.
// All rights reserved.

package update

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/derat/taglib-go/taglib"
	"github.com/derat/taglib-go/taglib/id3"
)

// readID3v1Footer reads a 128-byte ID3v1 footer from the end of f.
// ID3v1 is a terrible format.
func readID3v1Footer(f *os.File, fi os.FileInfo) (length int64, artist, title, album string, err error) {
	const (
		footerLen   = 128
		footerMagic = "TAG"
		titleLen    = 30
		artistLen   = 30
		albumLen    = 30
		// TODO: Add year (4 bytes), comment (30), and genre (1) if I ever care about them.
	)

	// Check for an ID3v1 footer.
	buf := make([]byte, footerLen)
	if _, err := f.ReadAt(buf, fi.Size()-int64(len(buf))); err != nil {
		return 0, "", "", "", err
	}

	b := bytes.NewBuffer(buf)
	if string(b.Next(len(footerMagic))) != footerMagic {
		return 0, "", "", "", nil
	}

	clean := func(b []byte) string { return string(bytes.TrimSpace(bytes.TrimRight(b, "\x00"))) }
	title = clean(b.Next(titleLen))
	artist = clean(b.Next(artistLen))
	album = clean(b.Next(albumLen))
	return footerLen, artist, title, album, nil
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

// frameInfo contains information about an MP3 audio frame header.
type frameInfo struct {
	kbitRate    int64 // in 1000 bits per second (not 1024)
	sampleRate  int64 // in hertz
	channelMode uint8 // 0x0 stereo, 0x1 joint stereo, 0x2 dual channel, 0x3 single channel
	hasCRC      bool  // 16-bit CRC follows header
	hasPadding  bool  // frame is padded with one extra bit
}

func (fi *frameInfo) size() int64 {
	// See https://www.opennet.ru/docs/formats/mpeghdr.html. Calculation may be more complicated per
	// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header, but if we're off we'll
	// probably see a problem when reading the next frame.
	s := (samplesPerFrame / 8) * (fi.kbitRate * 1000) / fi.sampleRate
	if fi.hasPadding {
		s++
	}
	return s
}

func (fi *frameInfo) duration() time.Duration {
	return (time.Second / time.Duration(fi.sampleRate)) * samplesPerFrame
}

// This constant is specific to MPEG 1, Layer 3.
const samplesPerFrame = 1152

// This table is specific to MPEG 1, Layer 3.
var kbitRates = [...]int64{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}

// This table is specific to MPEG 1.
var sampleRates = [...]int64{44100, 48000, 32000, 0}

// readFrameInfo reads an MPEG audio frame header at start in f.
// Format details at http://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header.
func readFrameInfo(f *os.File, start int64) (*frameInfo, error) {
	if _, err := f.Seek(start, 0); err != nil {
		return nil, err
	}
	var header uint32
	if err := binary.Read(f, binary.BigEndian, &header); err != nil {
		return nil, err
	}
	getBits := func(startBit, numBits uint) uint32 {
		return (header << startBit) >> (32 - numBits)
	}
	if v := getBits(0, 11); v != 0x7ff {
		return nil, fmt.Errorf("missing sync (got %#x instead of 0x7ff)", v)
	}
	if v := getBits(11, 2); v != 0x3 {
		return nil, fmt.Errorf("unsupported MPEG Audio version (got %#x instead of 0x3)", v)
	}
	if v := getBits(13, 2); v != 0x1 {
		return nil, fmt.Errorf("unsupported layer (got %#x instead of 0x1)", v)
	}

	return &frameInfo{
		kbitRate:    kbitRates[getBits(16, 4)],
		sampleRate:  sampleRates[getBits(20, 2)],
		channelMode: uint8(getBits(24, 2)),
		hasCRC:      getBits(15, 1) == 0x0,
		hasPadding:  getBits(22, 1) == 0x1,
	}, nil
}

// durationInfo contains extra information read by computeAudioDuration.
type durationInfo struct {
	kbitRate   int64
	sampleRate int64
	xingFlags  uint32
	numFrames  uint32 // from xing header
	numBytes   uint32 // from xing header
}

// computeAudioDuration reads Xing data from the frame at headerLen in f to return the audio length.
// Only supports MPEG Audio 1, Layer 3.
func computeAudioDuration(f *os.File, fi os.FileInfo, headerLen, footerLen int64) (
	time.Duration, *durationInfo, error) {
	finfo, err := readFrameInfo(f, headerLen)
	if err != nil {
		return 0, nil, fmt.Errorf("failed reading header at %#x: %v", headerLen, err)
	}

	info := durationInfo{
		kbitRate:   finfo.kbitRate,
		sampleRate: finfo.sampleRate,
	}

	xingStart := headerLen + 4
	if finfo.channelMode == 0x3 { // mono
		xingStart += 17
	} else {
		xingStart += 32
	}
	if finfo.hasCRC {
		xingStart += 2
	}

	b := make([]byte, 16)
	if _, err := f.ReadAt(b, xingStart); err != nil {
		return 0, &info, fmt.Errorf("unable to read Xing header at %#x: %v", xingStart, err)
	}
	xingName := string(b[0:4])
	if xingName == "Xing" || xingName == "Info" {
		r := bytes.NewReader(b[4:])
		if err := binary.Read(r, binary.BigEndian, &info.xingFlags); err != nil {
			return 0, &info, err
		}
		if info.xingFlags&0x1 == 0 {
			return 0, &info, fmt.Errorf("Xing header at %#x lacks number of frames", xingName)
		}
		if err := binary.Read(r, binary.BigEndian, &info.numFrames); err != nil {
			return 0, &info, err
		}
		if info.xingFlags&0x2 != 0 {
			if err := binary.Read(r, binary.BigEndian, &info.numBytes); err != nil {
				return 0, &info, err
			}
		}

		ms := samplesPerFrame * int64(info.numFrames) * 1000 / info.sampleRate
		return time.Duration(ms) * time.Millisecond, &info, nil
	}

	// Okay, no Xing VBR header. Assume that the file has a fixed bitrate.
	// (The other alternative is to read the whole file to count the number of frames.)
	ms := (fi.Size() - headerLen - footerLen) / info.kbitRate * 8
	return time.Duration(ms) * time.Millisecond, &info, nil
}
