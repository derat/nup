// Copyright 2022 Daniel Erat.
// All rights reserved.

package debug

import (
	"context"
	"flag"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/google/subcommands"
)

type Command struct {
	Cfg *client.Config

	id3  bool // print all ID3v2 text frames
	mpeg bool // read MPEG frames and print size/duration
}

func (*Command) Name() string     { return "debug" }
func (*Command) Synopsis() string { return "print information about a song file" }
func (*Command) Usage() string {
	return `debug <flags> <path>...:
	Print information about one or more song files.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.BoolVar(&cmd.id3, "id3", false, "Print all ID3v2 text frames")
	f.BoolVar(&cmd.mpeg, "mpeg", false, "Read MPEG frames and print size/duration info")
}

func (cmd *Command) Execute(ctx context.Context, fs *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, cmd.Usage())
		return subcommands.ExitUsageError
	}
	if !cmd.id3 && !cmd.mpeg {
		fmt.Fprintln(os.Stderr, "No action requested via flags")
		return subcommands.ExitUsageError
	}

	var failed bool
	for i, p := range fs.Args() {
		if len(fs.Args()) > 1 {
			if i > 0 {
				fmt.Println()
			}
			fmt.Println(p)
		}
		if cmd.id3 {
			if err := cmd.doID3(p); err != nil {
				fmt.Fprintln(os.Stderr, "Failed reading ID3 tag:", err)
				failed = true
			}
		}
		if cmd.mpeg {
			if err := cmd.doMPEG(p); err != nil {
				fmt.Fprintln(os.Stderr, "Failed reading MPEG frames:", err)
				failed = true
			}
		}
	}
	if failed {
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (cmd *Command) doID3(p string) error {
	frames, err := readID3Frames(p)
	if err != nil {
		return err
	}
	for _, frame := range frames {
		var val string
		if len(frame.fields) == 0 {
			val = fmt.Sprintf("[%d bytes]", frame.size)
		} else {
			quoted := make([]string, len(frame.fields))
			for i, s := range frame.fields {
				quoted[i] = fmt.Sprintf("%q", s)
			}
			val = strings.Join(quoted, " ")
		}
		fmt.Println(frame.id + " " + val)
	}
	return nil
}

func (cmd *Command) doMPEG(p string) error {
	info, err := getMPEGInfo(p)
	if err != nil {
		return err
	}

	fmt.Printf("%d bytes: %d header, %d data, %d footer (%v)\n",
		info.size, info.header, info.size-info.header-info.footer, info.footer, info.sha1)
	for _, s := range info.skipped {
		fmt.Printf("  skipped %d-%d (%d) %v\n", s.offset, s.offset+s.size, s.size, s.err)
	}

	var actualBitrate string
	if info.vbr {
		actualBitrate = fmt.Sprintf("%0.1f kb/s VBR", info.avgKbitRate)
	} else {
		actualBitrate = fmt.Sprintf("%0.0f kb/s CBR", math.Round(info.avgKbitRate))
	}

	format := func(d time.Duration) string {
		return fmt.Sprintf("%d:%06.3f", int(d.Minutes()), (d % time.Minute).Seconds())
	}
	fmt.Printf("Xing:   %s (%d frames, %d data)\n",
		format(info.xingDur), info.xingFrames, info.xingBytes)
	fmt.Printf("Actual: %s (%d frames, %d data, %s)\n",
		format(info.actualDur), info.actualFrames, info.actualBytes, actualBitrate)
	if info.emptyFrame >= 0 {
		fmt.Printf("Audio:  %s (%d frames, then empty starting at offset %d)\n",
			format(info.emptyTime), info.emptyFrame, info.emptyOffset)
	}
	return nil
}
