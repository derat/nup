// Copyright 2022 Daniel Erat.
// All rights reserved.

package debug

import (
	"errors"
	"os"

	"github.com/derat/taglib-go/taglib"
	"github.com/derat/taglib-go/taglib/id3"
)

type id3TextFrame struct {
	id     string
	fields []string
}

func readID3Frames(p string) ([]id3TextFrame, error) {
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

	var ret []id3TextFrame
	switch tag := gen.(type) {
	case *id3.Id3v23Tag:
		for id, frames := range tag.Frames {
			for _, frame := range frames {
				if fields, err := id3.GetId3v23TextIdentificationFrame(frame); err == nil {
					ret = append(ret, id3TextFrame{id: id, fields: append([]string(nil), fields...)})
				}
			}
		}
	case *id3.Id3v24Tag:
		for id, frames := range tag.Frames {
			for _, frame := range frames {
				if fields, err := id3.GetId3v24TextIdentificationFrame(frame); err == nil {
					ret = append(ret, id3TextFrame{id: id, fields: append([]string(nil), fields...)})
				}
			}
		}
	default:
		return nil, errors.New("unsupported ID3 version")
	}
	return ret, nil
}
