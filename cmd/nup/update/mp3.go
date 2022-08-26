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
	kbitRate    int   // in 1000 bits per second (not 1024)
	sampleRate  int   // in hertz
	channelMode uint8 // 0x0 stereo, 0x1 joint stereo, 0x2 dual channel, 0x3 single channel
	hasCRC      bool  // 16-bit CRC follows header
	hasPadding  bool  // frame is padded with one extra bit
}

func (fi *frameInfo) size() int64 {
	// See https://www.opennet.ru/docs/formats/mpeghdr.html. Calculation may be more complicated per
	// https://www.codeproject.com/Articles/8295/MPEG-Audio-Frame-Header, but if we're off we'll
	// probably see a problem when reading the next frame.
	s := (samplesPerFrame / 8) * int64(fi.kbitRate*1000) / int64(fi.sampleRate)
	if fi.hasPadding {
		s++
	}
	return s
}

func (fi *frameInfo) empty() bool {
	return fi.size() == 104
}

// This constant is specific to MPEG 1, Layer 3.
const samplesPerFrame = 1152

// This table is specific to MPEG 1, Layer 3.
var kbitRates = [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}

// This table is specific to MPEG 1.
var sampleRates = [...]int{44100, 48000, 32000, 0}

// readFrameInfo reads an MPEG audio frame header at the specified offset in f.
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

// I've seen some files that seemed to have a bunch of junk (or at least not an MPEG header
// starting with sync bits) after the header offset identified by taglib-go. Look over this
// many bytes to try to find something that looks like a proper header.
const maxFrameSearchBytes = 8192

// computeAudioDuration reads Xing data from the frame at headerLen in f to return the audio length.
// If no Xing header is present, it assumes that the file has a constant bitrate.
// Only supports MPEG Audio 1, Layer 3.
func computeAudioDuration(f *os.File, fi os.FileInfo, headerLen, footerLen int64) (
	dur time.Duration, xingFrames int, xingBytes int64, err error) {
	var finfo *frameInfo
	fstart := headerLen
	for ; fstart < headerLen+maxFrameSearchBytes; fstart++ {
		if finfo, err = readFrameInfo(f, fstart); err == nil {
			break
		}
	}
	if err != nil {
		return 0, 0, 0, fmt.Errorf("didn't find header after %#x", headerLen)
	}

	xingStart := fstart + 4
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
		return 0, 0, 0, fmt.Errorf("unable to read Xing header at %#x: %v", xingStart, err)
	}
	if s := string(b[:4]); s != "Xing" && s != "Info" {
		// Okay, no Xing VBR header. Assume that the file has a fixed bitrate.
		// (The other alternative is to read the whole file to count the number of frames.)
		ms := (fi.Size() - fstart - footerLen) / int64(finfo.kbitRate) * 8
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
	ms := samplesPerFrame * int64(nframes) * 1000 / int64(finfo.sampleRate)
	return time.Duration(ms) * time.Millisecond, int(nframes), int64(nbytes), nil
}

// debugInfo contains debugging info about an MP3 file.
type debugInfo struct {
	size         int64         // entire file
	header       int64         // ID3v2 header size
	footer       int64         // ID3v1 footer size
	sha1         string        // SHA1 of data between header and footer
	kbitRate     int           // from first audio frame
	sampleRate   int           // from first audio frame
	xingFrames   int           // number of frames from Xing header
	xingBytes    int64         // audio data size from Xing header
	xingDur      time.Duration // audio duration from Xing header (or CBR)
	actualFrames int           // actual frame count
	actualBytes  int64         // actual audio data size
	actualDur    time.Duration // actual duration
	emptyFrame   int           // first empty frame at end of file
	emptyOffset  int64         // offset of emptyFrame from start of file
	emptyTime    time.Duration // time of emptyFrame
}

// getSongDebugInfo returns debug information about the MP3 file at p.
func getSongDebugInfo(p string) (*debugInfo, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	info := debugInfo{size: fi.Size(), emptyFrame: -1}
	if n, _, _, _, err := readID3v1Footer(f, fi); err == nil {
		info.footer = n
	}
	if tag, err := taglib.Decode(f, fi.Size()); err == nil {
		info.header = int64(tag.TagSize())
	}
	if info.sha1, err = computeAudioSHA1(f, fi, info.header, info.footer); err != nil {
		return &info, fmt.Errorf("failed computing SHA1: %v", err)
	}

	// Read the Xing header.
	info.xingDur, info.xingFrames, info.xingBytes, err = computeAudioDuration(
		f, fi, info.header, info.footer)
	if err != nil {
		return &info, fmt.Errorf("failed computing duration: %v", err)
	}

	// Read all of the frames in the file.
	off := info.header
	for i := 0; off < info.size-info.footer; i++ {
		finfo, err := readFrameInfo(f, off)
		if err != nil {
			return &info, fmt.Errorf("frame %d at %d: %v", i, off, err)
		}

		if i == 0 {
			info.kbitRate = finfo.kbitRate
			info.sampleRate = finfo.sampleRate
		}

		// Check for empty frames at the end of the file.
		if finfo.empty() {
			if info.emptyFrame < 0 {
				info.emptyFrame = i
				info.emptyOffset = off
			}
		} else {
			info.emptyFrame = -1
		}
		info.actualFrames++
		info.actualBytes += finfo.size()
		off += finfo.size()
	}

	// The Xing header apparently doesn't include itself in the frame count
	// (but confusingly *does* include itself in the bytes count):
	// https://www.mail-archive.com/mp3encoder@minnie.tuhs.org/msg02868.html
	// https://hydrogenaud.io/index.php?topic=85690.0
	// If it was present, adjust fields so they'll be comparable to xingFrames.
	if info.xingFrames != 0 {
		info.actualFrames--
		if info.emptyFrame > 0 {
			info.emptyFrame--
		}
	}

	// Compute durations. The sample rate is fixed and there's a constant number
	// of samples per frame, so we just need the number of frames.
	computeDur := func(frames int) time.Duration {
		return time.Duration(samplesPerFrame*frames) * time.Second /
			time.Duration(info.sampleRate)
	}
	info.actualDur = computeDur(info.actualFrames)
	if info.emptyFrame >= 0 {
		info.emptyTime = computeDur(info.emptyFrame)
	}

	return &info, nil
}
