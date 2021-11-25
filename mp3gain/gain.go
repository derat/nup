// Copyright 2021 Daniel Erat.
// All rights reserved.

package mp3gain

import (
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

// Info contains information about gain adjustments for a song.
type Info struct {
	// TrackGain is the track's dB gain adjustment independent of its album.
	TrackGain float64
	// AlbumGain is the album's dB gain adjustment.
	AlbumGain float64
	// PeakAmp is the peak amplitude of the song, with 1.0 being the maximum
	// amplitude playable without clipping.
	PeakAmp float64
}

// ComputeAlbum uses mp3gain to compute gain adjustments for the
// specified MP3 files, all of which should be from the same album.
// Keys in the returned map are the supplied paths.
func ComputeAlbum(paths []string) (map[string]Info, error) {
	// Return hardcoded data for tests if instructed.
	if infoForTest != nil {
		m := make(map[string]Info)
		for _, p := range paths {
			m[p] = *infoForTest
		}
		return m, nil
	}

	out, err := exec.Command("mp3gain", append([]string{
		"-o",      // "output is a database-friendly tab-delimited list"
		"-q",      // "quiet mode: no status messages"
		"-s", "s", // "skip (ignore) stored tag info (do not read or write tags)"
	}, paths...)...).Output()
	if err != nil {
		return nil, fmt.Errorf("mp3gain failed: %v", err)
	}

	m, err := parseMP3GainOutput(string(out))
	if err != nil {
		return nil, fmt.Errorf("bad mp3gain output: %v", err)
	}
	return m, nil
}

// parseMP3GainOutput parses output from the mp3gain command for computeGains.
func parseMP3GainOutput(out string) (map[string]Info, error) {
	lns := strings.Split(strings.TrimSpace(out), "\n")
	if len(lns) < 3 {
		return nil, fmt.Errorf("output %q not at least 3 lines", out)
	}

	// The last line contains the album summary.
	p, albumGain, _, err := parseMP3GainLine(lns[len(lns)-1])
	if err != nil {
		return nil, fmt.Errorf("failed parsing %q", lns[len(lns)-1])
	}
	if p != `"Album"` {
		return nil, fmt.Errorf(`expected "Album" for summary %q`, lns[len(lns)-1])
	}

	// Skip the header and the album summary.
	m := make(map[string]Info)
	for _, ln := range lns[1 : len(lns)-1] {
		p, gain, peakAmp, err := parseMP3GainLine(ln)
		if err != nil {
			return nil, fmt.Errorf("failed parsing %q", ln)
		}
		m[p] = Info{TrackGain: gain, AlbumGain: albumGain, PeakAmp: peakAmp}
	}
	return m, nil
}

// parseMP3GainLine parses an individual line of output for parseMP3GainOutput.
func parseMP3GainLine(ln string) (path string, gain, peakAmp float64, err error) {
	fields := strings.Split(ln, "\t")
	if len(fields) != 6 {
		return "", 0, 0, fmt.Errorf("got %d field(s); want 6", len(fields))
	}
	// Fields are path, MP3 gain, dB gain, max amplitude, max global_gain, min global_gain.
	if gain, err = strconv.ParseFloat(fields[2], 64); err != nil {
		return "", 0, 0, err
	}
	if peakAmp, err = strconv.ParseFloat(fields[3], 64); err != nil {
		return "", 0, 0, err
	}
	peakAmp /= 32767 // output seems to be based on 16-bit samples
	peakAmp = math.Round(peakAmp*100000) / 100000

	return fields[0], gain, peakAmp, nil
}

// infoForTest contains hardcoded gain information to return.
var infoForTest *Info

// SetInfoForTest sets a hardcoded Info object to use instead of
// actually running the mp3gain program.
func SetInfoForTest(info *Info) {
	infoForTest = info
}
