// Copyright 2020 Daniel Erat.
// All rights reserved.

package dump

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/derat/nup/cmd/nup/client"
	"github.com/derat/nup/server/db"
	"github.com/google/subcommands"
)

const (
	progressInterval = 100

	// TODO: Tune these numbers.
	defaultSongBatchSize = 400
	defaultPlayBatchSize = 800
	chanSize             = 50
)

type Command struct {
	Cfg *client.Config

	songBatchSize int // batch size for Song entities
	playBatchSize int // batch size for Play entities
}

func (*Command) Name() string     { return "dump" }
func (*Command) Synopsis() string { return "dump songs from the server" }
func (*Command) Usage() string {
	return `dump <flags>:
	Dump JSON-marshaled song data from the server to stdout.

`
}

func (cmd *Command) SetFlags(f *flag.FlagSet) {
	f.IntVar(&cmd.songBatchSize, "song-batch-size", defaultSongBatchSize, "Size for each batch of entities")
	f.IntVar(&cmd.playBatchSize, "play-batch-size", defaultPlayBatchSize, "Size for each batch of entities")
}

func (cmd *Command) Execute(ctx context.Context, _ *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	songChan := make(chan *db.Song, chanSize)
	go getSongs(cmd.Cfg, cmd.songBatchSize, songChan)

	playChan := make(chan *db.PlayDump, chanSize)
	go getPlays(cmd.Cfg, cmd.playBatchSize, playChan)

	e := json.NewEncoder(os.Stdout)

	numSongs := 0
	pd := <-playChan
	for {
		s := <-songChan
		if s == nil {
			break
		}

		for pd != nil && pd.SongID == s.SongID {
			s.Plays = append(s.Plays, pd.Play)
			pd = <-playChan
		}

		if err := e.Encode(s); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to encode song:", err)
			return subcommands.ExitFailure
		}

		numSongs++
		if numSongs%progressInterval == 0 {
			log.Printf("Wrote %d songs", numSongs)
		}
	}
	log.Printf("Wrote %d songs", numSongs)

	if pd != nil {
		fmt.Fprintf(os.Stderr, "Got orphaned play for song %v: %v\n", pd.SongID, pd.Play)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func getEntities(cfg *client.Config, entityType string, extraArgs []string, batchSize int, f func([]byte)) {
	u := cfg.GetURL("/export")
	var cursor string
	for {
		u.RawQuery = fmt.Sprintf("type=%s&max=%d", entityType, batchSize)
		if len(extraArgs) > 0 {
			u.RawQuery += "&" + strings.Join(extraArgs, "&")
		}
		if len(cursor) > 0 {
			u.RawQuery += "&cursor=" + cursor
		}

		req, err := http.NewRequest("GET", u.String(), nil)
		if err != nil {
			log.Fatal("Failed to create request: ", err)
		}
		req.SetBasicAuth(cfg.Username, cfg.Password)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatalf("Failed to fetch %v: %v", u.String(), err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			log.Fatal("Got non-OK status: ", resp.Status)
		}

		cursor = ""
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if err := json.Unmarshal(scanner.Bytes(), &cursor); err != nil {
				f(scanner.Bytes())
			}
		}
		if err = scanner.Err(); err != nil {
			log.Fatal("Got error while reading from server: ", err)
		}

		if len(cursor) == 0 {
			break
		}
	}
}

func getSongs(cfg *client.Config, batchSize int, ch chan *db.Song) {
	getEntities(cfg, "song", nil, batchSize, func(b []byte) {
		var s db.Song
		if err := json.Unmarshal(b, &s); err == nil {
			ch <- &s
		} else {
			log.Fatalf("Got unexpected line from server: %v", string(b))
		}
	})
	ch <- nil
}

func getPlays(cfg *client.Config, batchSize int, ch chan *db.PlayDump) {
	getEntities(cfg, "play", nil, batchSize, func(b []byte) {
		var pd db.PlayDump
		if err := json.Unmarshal(b, &pd); err == nil {
			ch <- &pd
		} else {
			log.Fatalf("Got unexpected line from server: %v", string(b))
		}
	})
	ch <- nil
}
