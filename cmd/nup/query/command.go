// Copyright 2022 Daniel Erat.
// All rights reserved.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
	"github.com/google/subcommands"
)

type Command struct {
	Cfg *client.Config

	filename string // Song.Filename (i.e. relative to Cfg.MusicDir)
	path     string // Song.Filename as absolute path or relative to CWD
	pretty   bool   // pretty-print JSON objects
	printID  bool   // print Song.SongID instead of JSON objects
	single   bool   // require exactly one song to be matched
}

func (*Command) Name() string     { return "query" }
func (*Command) Synopsis() string { return "run song queries against the server" }
func (*Command) Usage() string {
	return `query <flags>:
	Query the server and and print JSON-marshaled songs to stdout.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.filename, "filename", "", "Song filename (relative to music dir) to query for")
	f.StringVar(&cmd.path, "path", "", "Song path (resolved to music dir) to query for")
	f.BoolVar(&cmd.pretty, "pretty", false, "Pretty-print JSON objects")
	f.BoolVar(&cmd.printID, "print-id", false, "Print song IDs instead of full JSON objects")
	f.BoolVar(&cmd.single, "single", false, "Require exactly one song to be matched")
}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	req, err := cmd.makeRequest(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed creating request:", err)
		return subcommands.ExitUsageError
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Request failed:", err)
		return subcommands.ExitFailure
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "Got non-OK status:", resp.Status)
		return subcommands.ExitFailure
	}

	var songs []*db.Song
	if err := json.NewDecoder(resp.Body).Decode(&songs); err != nil {
		fmt.Fprintln(os.Stderr, "Failed decoding songs:", err)
		return subcommands.ExitFailure
	}
	if cmd.single && len(songs) != 1 {
		fmt.Fprintf(os.Stderr, "Got %d songs instead of 1\n", len(songs))
		return subcommands.ExitFailure
	}

	switch {
	case cmd.printID:
		for _, song := range songs {
			fmt.Println(song.SongID)
		}
	default:
		enc := json.NewEncoder(os.Stdout)
		if cmd.pretty {
			enc.SetIndent("", "  ")
		}
		for i, song := range songs {
			if err := enc.Encode(&song); err != nil {
				fmt.Fprintf(os.Stderr, "Failed encoding song %d: %v\n", i, err)
				return subcommands.ExitFailure
			}
		}
	}

	return subcommands.ExitSuccess
}

func (cmd *Command) makeRequest(ctx context.Context) (*http.Request, error) {
	vals := make(url.Values)
	if cmd.path != "" {
		// Use -path to set -filename.
		if cmd.Cfg.MusicDir == "" {
			return nil, errors.New("music dir needed for -path but not specified in config file")
		}
		abs, err := filepath.Abs(cmd.path)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(abs, cmd.Cfg.MusicDir+"/") {
			return nil, fmt.Errorf("path %q not under music dir %q", abs, cmd.Cfg.MusicDir)
		}
		if cmd.filename, err = filepath.Rel(cmd.Cfg.MusicDir, abs); err != nil {
			return nil, err
		}
	}
	if cmd.filename != "" {
		vals.Set("filename", cmd.filename)
	}
	if len(vals) == 0 {
		return nil, errors.New("no query parameters supplied")
	}

	qurl := cmd.Cfg.GetURL("/query")
	qurl.RawQuery = vals.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", qurl.String(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(cmd.Cfg.Username, cmd.Cfg.Password)
	return req, nil
}
