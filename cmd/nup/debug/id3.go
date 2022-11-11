// Copyright 2022 Daniel Erat.
// All rights reserved.

package debug

import (
	"errors"
	"os"
	"strconv"

	"github.com/derat/mpeg"
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

	var ret []id3Frame

	// Return miscellaneous info using fake user-defined frames.
	appendTextFrame := func(fields []string) {
		var size int
		for _, s := range fields {
			size += len(s)
		}
		ret = append(ret, id3Frame{id: "TXXX", size: size, fields: fields})
	}

	if gen, err := taglib.Decode(f, fi.Size()); err == nil {
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
			appendTextFrame([]string{"ID3v2 version unsupported"})
		}
	}

	if tag, err := mpeg.ReadID3v1Footer(f, fi); err == nil && tag != nil {
		add := func(name, val string) {
			if val != "" {
				appendTextFrame([]string{name, val})
			}
		}
		add("ID3v1 Artist", tag.Artist)
		add("ID3v1 Title", tag.Title)
		add("ID3v1 Album", tag.Album)
		add("ID3v1 Year", tag.Year)
		add("ID3v1 Comment", tag.Comment)
		if tag.Track != 0 {
			add("ID3v1 Track", strconv.Itoa(int(tag.Track)))
		}
		if tag.Genre != 255 { // 0 is "Blues", 255 is none: https://exiftool.org/TagNames/ID3.html
			add("ID3v1 Genre", strconv.Itoa(int(tag.Genre)))
		}
	}

	if len(ret) == 0 {
		return nil, errors.New("no ID3 tag")
	}
	return ret, nil
}
