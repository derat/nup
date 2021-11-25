// Copyright 2021 Daniel Erat.
// All rights reserved.

package mp3gain

import (
	"reflect"
	"testing"
)

func TestParseMP3GainOutput(t *testing.T) {
	const out = `File	MP3 gain	dB gain	Max Amplitude	Max global_gain	Min global_gain
/tmp/1.mp3	-6	-9.480000	35738.816406	210	101
/tmp/2.mp3	1	1.820000	20295.107422	210	132
/tmp/3.mp3	-5	-8.200000	36487.070312	210	77
/tmp/4.mp3	-2	-2.630000	28630.636719	210	45
/tmp/5.mp3	-5	-8.140000	36071.472656	210	113
"Album"	-5	-8.220000	36487.070312	210	45
`
	want := map[string]Info{
		"/tmp/1.mp3": Info{-9.48, -8.22, 1.09070},
		"/tmp/2.mp3": Info{1.82, -8.22, 0.61938},
		"/tmp/3.mp3": Info{-8.20, -8.22, 1.11353},
		"/tmp/4.mp3": Info{-2.63, -8.22, 0.87376},
		"/tmp/5.mp3": Info{-8.14, -8.22, 1.10085},
	}
	if got, err := parseMP3GainOutput(out); err != nil {
		t.Fatal("parseMP3GainOutput failed: ", err)
	} else if !reflect.DeepEqual(got, want) {
		t.Errorf("parseMP3GainOutput returned %v; want %v", got, want)
	}
}
