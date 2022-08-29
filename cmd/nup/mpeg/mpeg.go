// Copyright 2022 Daniel Erat.
// All rights reserved.

package mpeg

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/derat/taglib-go/taglib"
	"github.com/derat/taglib-go/taglib/id3"
)

// ReadID3v1Footer reads a 128-byte ID3v1 footer from the end of f.
// ID3v1 is a terrible format.
func ReadID3v1Footer(f *os.File, fi os.FileInfo) (length int64, artist, title, album string, err error) {
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

// GetID3v2TextFrame returns the first ID3v2 text frame with the supplied ID from gen.
// If the frame isn't present, an empty string and nil error are returned.
//
// The taglib library has built-in support for some frames ("TPE1", "TIT2", "TALB", etc.)
// and provides generic support for custom "TXXX" frames, but it doesn't seem to provide
// an easy way to read other well-known frames like "TPE2".
func GetID3v2TextFrame(gen taglib.GenericTag, id string) (string, error) {
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

// ComputeAudioSHA1 returns a SHA1 hash of the audio (i.e. non-metadata) portion of f.
func ComputeAudioSHA1(f *os.File, fi os.FileInfo, headerLen, footerLen int64) (string, error) {
	if _, err := f.Seek(headerLen, 0); err != nil {
		return "", err
	}
	hasher := sha1.New()
	if _, err := io.CopyN(hasher, f, fi.Size()-headerLen-footerLen); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// FrameInfo contains information about an MPEG (MP3?) audio frame header.
type FrameInfo struct {
	KbitRate    int   // in 1000 bits per second (not 1024)
	SampleRate  int   // in hertz
	ChannelMode uint8 // 0x0 stereo, 0x1 joint stereo, 0x2 dual channel, 0x3 single channel
	HasCRC      bool  // 16-bit CRC follows header
	HasPadding  bool  // frame is padded with one extra bit
}

func (fi *FrameInfo) Size() int64 {
	// See https://www.opennet.ru/docs/formats/mpeghdr.html. Calculation may be more complicated per
	// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header, but if we're off we'll
	// probably see a problem when reading the next frame.
	s := (SamplesPerFrame / 8) * int64(fi.KbitRate*1000) / int64(fi.SampleRate)
	if fi.HasPadding {
		s++
	}
	return s
}

func (fi *FrameInfo) Empty() bool {
	return fi.Size() == 104
}

// This constant is specific to MPEG 1, Layer 3.
const SamplesPerFrame = 1152

// This table is specific to MPEG 1, Layer 3.
var kbitRates = [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}

// This table is specific to MPEG 1.
var sampleRates = [...]int{44100, 48000, 32000, 0}

// ReadFrameInfo reads an MPEG audio frame header at the specified offset in f.
// Format details at http://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header.
func ReadFrameInfo(f *os.File, start int64) (*FrameInfo, error) {
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
		return nil, errors.New("no 0x7ff sync")
	}
	if v := getBits(11, 2); v != 0x3 {
		return nil, fmt.Errorf("unsupported MPEG Audio version (got %#x instead of 0x3)", v)
	}
	if v := getBits(13, 2); v != 0x1 {
		return nil, fmt.Errorf("unsupported layer (got %#x instead of 0x1)", v)
	}

	return &FrameInfo{
		KbitRate:    kbitRates[getBits(16, 4)],
		SampleRate:  sampleRates[getBits(20, 2)],
		ChannelMode: uint8(getBits(24, 2)),
		HasCRC:      getBits(15, 1) == 0x0,
		HasPadding:  getBits(22, 1) == 0x1,
	}, nil
}

// I've seen some files that seemed to have a bunch of junk (or at least not an MPEG header
// starting with sync bits) after the header offset identified by taglib-go. Look over this
// many bytes to try to find something that looks like a proper header.
const maxFrameSearchBytes = 8192

// ComputeAudioDuration reads Xing data from the frame at headerLen in f to return the audio length.
// If no Xing header is present, it assumes that the file has a constant bitrate.
// Only supports MPEG Audio 1, Layer 3.
func ComputeAudioDuration(f *os.File, fi os.FileInfo, headerLen, footerLen int64) (
	dur time.Duration, xingFrames int, xingBytes int64, err error) {
	var finfo *FrameInfo
	fstart := headerLen
	for ; fstart < headerLen+maxFrameSearchBytes; fstart++ {
		if finfo, err = ReadFrameInfo(f, fstart); err == nil {
			break
		}
	}
	if err != nil {
		return 0, 0, 0, fmt.Errorf("didn't find header after %#x", headerLen)
	}

	xingStart := fstart + 4
	if finfo.ChannelMode == 0x3 { // mono
		xingStart += 17
	} else {
		xingStart += 32
	}
	if finfo.HasCRC {
		xingStart += 2
	}

	b := make([]byte, 16)
	if _, err := f.ReadAt(b, xingStart); err != nil {
		return 0, 0, 0, fmt.Errorf("unable to read Xing header at %#x: %v", xingStart, err)
	}
	if s := string(b[:4]); s != "Xing" && s != "Info" {
		// Okay, no Xing VBR header. Assume that the file has a fixed bitrate.
		// (The other alternative is to read the whole file to count the number of frames.)
		ms := (fi.Size() - fstart - footerLen) / int64(finfo.KbitRate) * 8
		return time.Duration(ms) * time.Millisecond, 0, 0, nil
	}

	r := bytes.NewReader(b[4:])
	var flags uint32
	if err := binary.Read(r, binary.BigEndian, &flags); err != nil {
		return 0, 0, 0, err
	}
	if flags&0x1 == 0 {
		return 0, 0, 0, errors.New("Xing header lacks number of frames")
	}
	var nframes uint32
	if err := binary.Read(r, binary.BigEndian, &nframes); err != nil {
		return 0, 0, 0, err
	}
	var nbytes uint32
	if flags&0x2 != 0 {
		if err := binary.Read(r, binary.BigEndian, &nbytes); err != nil {
			return 0, 0, 0, err
		}
	}
	ms := SamplesPerFrame * int64(nframes) * 1000 / int64(finfo.SampleRate)
	return time.Duration(ms) * time.Millisecond, int(nframes), int64(nbytes), nil
}
