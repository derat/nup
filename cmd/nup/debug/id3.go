// Copyright 2022 Daniel Erat.
// All rights reserved.

package debug

import (
	"errors"
	"os"

	"github.com/derat/taglib-go/taglib"
	"github.com/derat/taglib-go/taglib/id3"
)

// Maximum ID3v2 frame content size. Longer frames (e.g. image data) are not decoded.
const maxID3FrameSize = 256

type id3Frame struct {
	id     string
	size   int // bytes
	fields []string
}

func readID3Frames(p string) ([]id3Frame, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}

	gen, err := taglib.Decode(f, fi.Size())
	if err != nil {
		return nil, err
	}

	var ret []id3Frame
	switch tag := gen.(type) {
	case *id3.Id3v23Tag:
		for id, frames := range tag.Frames {
			for _, frame := range frames {
				info := id3Frame{id: id, size: len(frame.Content)}
				if info.size <= maxID3FrameSize {
					info.fields, _ = id3.GetId3v23TextIdentificationFrame(frame)
				}
				ret = append(ret, info)
			}
		}
	case *id3.Id3v24Tag:
		for id, frames := range tag.Frames {
			for _, frame := range frames {
				info := id3Frame{id: id, size: len(frame.Content)}
				if info.size <= maxID3FrameSize {
					info.fields, _ = id3.GetId3v24TextIdentificationFrame(frame)
				}
				ret = append(ret, info)
			}
		}
	default:
		return nil, errors.New("unsupported ID3 version")
	}
	return ret, nil
}
