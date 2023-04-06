// Copyright 2022 Daniel Erat.
// All rights reserved.

package debug

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/derat/mpeg"
	"github.com/derat/taglib-go/taglib"
)

// mpegInfo contains debugging info about an MP3 file.
type mpegInfo struct {
	size            int64         // entire file
	header          int64         // ID3v2 header size
	footer          int64         // ID3v1 footer size
	sha1            string        // SHA1 of data between header and footer
	vbr             bool          // bitrate is variable (as opposed to constant)
	avgKbitRate     float64       // averaged across audio frames
	sampleRate      int           // from first audio frame
	samplesPerFrame int           // from first audio frame
	xingFrames      int           // number of frames from Xing header
	xingBytes       int64         // audio data size from Xing header
	xingDur         time.Duration // audio duration from Xing header (or CBR)
	actualFrames    int           // actual frame count
	actualBytes     int64         // actual audio data size
	actualDur       time.Duration // actual duration
	emptyFrame      int           // first empty frame at end of file
	emptyOffset     int64         // offset of emptyFrame from start of file
	emptyTime       time.Duration // time of emptyFrame
	skipped         []skipInfo    // skipped regions
}

// skipInfo contains details about an invalid region of an MP3 file that was skipped.
type skipInfo struct {
	offset, size int64 // in bytes
	err          error // first error
}

// getMPEGInfo returns debug information about the MP3 file at p.
func getMPEGInfo(p string) (*mpegInfo, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	info := mpegInfo{size: fi.Size(), emptyFrame: -1}
	if tag, err := mpeg.ReadID3v1Footer(f, fi); err == nil && tag != nil {
		info.footer = mpeg.ID3v1Length
	}
	if tag, err := taglib.Decode(f, fi.Size()); err == nil {
		info.header = int64(tag.TagSize())
	}
	if info.sha1, err = mpeg.ComputeAudioSHA1(f, fi, info.header, info.footer); err != nil {
		return &info, fmt.Errorf("failed computing SHA1: %v", err)
	}

	// Read the Xing header.
	info.xingDur, info.xingFrames, info.xingBytes, err = mpeg.ComputeAudioDuration(
		f, fi, info.header, info.footer)
	if err != nil {
		return &info, fmt.Errorf("failed computing duration: %v", err)
	}

	// Read all of the frames in the file.
	off := info.header
	var skipped, kbitRateSum int64
	var skipErr error
	var lastKbitRate int

	addSkipped := func() {
		if skipped == 0 {
			return
		}

		start := off - skipped

		// Diagnose common errors.
		b := make([]byte, skipped)
		if _, err := f.ReadAt(b, start); err == nil {
			if bytes.HasPrefix(b, []byte("LYRICSBEGIN")) && bytes.HasSuffix(b, []byte("LYRICSEND")) {
				skipErr = errors.New("Lyrics3v1 tag")
			} else if bytes.HasPrefix(b, []byte("LYRICSBEGIN")) && bytes.HasSuffix(b, []byte("LYRICS200")) {
				skipErr = errors.New("Lyrics3v2 tag")
			} else if bytes.HasPrefix(b, []byte("TAG")) {
				skipErr = errors.New("extra ID3v1 tag")
			} else if bytes.Count(b, []byte{0}) == len(b) {
				skipErr = errors.New("empty")
			}
		}
		info.skipped = append(info.skipped, skipInfo{
			offset: start,
			size:   skipped,
			err:    skipErr,
		})
		skipped = 0
		skipErr = nil
	}

	for off < info.size-info.footer {
		finfo, err := mpeg.ReadFrameInfo(f, off)
		if err != nil {
			off++
			skipped++
			if skipErr == nil {
				skipErr = err
			}
			continue
		}

		addSkipped()

		// Skip the Xing frame for bitrate calculations since its bitrate sometimes differs from the
		// audio frames in CBR files.
		if isXing := info.xingBytes != 0 && info.actualFrames == 0; !isXing {
			if lastKbitRate > 0 && finfo.KbitRate != lastKbitRate {
				info.vbr = true
			}
			lastKbitRate = finfo.KbitRate
			kbitRateSum += int64(finfo.KbitRate)

			// Get the sample rate from the first non-Xing frame we find. Getting it from the Xing
			// frame seemed to work as well, but it feels a bit safer to skip it since I've seen
			// different bitrates there.
			if info.sampleRate == 0 {
				info.sampleRate = finfo.SampleRate
				info.samplesPerFrame = finfo.SamplesPerFrame
			}
		}

		// Check for empty frames at the end of the file.
		if finfo.Empty() {
			if info.emptyFrame < 0 {
				info.emptyFrame = info.actualFrames
				info.emptyOffset = off
			}
		} else {
			info.emptyFrame = -1
		}
		info.actualFrames++
		info.actualBytes += finfo.Size()
		off += finfo.Size()
	}
	addSkipped()

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

	// Do this after the above decrement since we excluded the Xing frame from kbitRateSum.
	if info.actualFrames > 0 {
		info.avgKbitRate = float64(kbitRateSum) / float64(info.actualFrames)
	}

	// Compute durations. The sample rate is fixed and there's a constant number
	// of samples per frame, so we just need the number of frames.
	computeDur := func(frames int) time.Duration {
		if info.sampleRate == 0 {
			return 0
		}
		return time.Duration(info.samplesPerFrame*frames) * time.Second /
			time.Duration(info.sampleRate)
	}
	info.actualDur = computeDur(info.actualFrames)
	if info.emptyFrame >= 0 {
		info.emptyTime = computeDur(info.emptyFrame)
	}

	return &info, nil
}
