package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"erat.org/nup"
)

const (
	exportPath = "export"

	progressInterval = 100

	// TODO: Tune these numbers.
	defaultSongBatchSize = 400
	defaultPlayBatchSize = 800
	chanSize             = 50
)

func getEntities(cfg *nup.ClientConfig, entityType string, extraArgs string, batchSize int, f func([]byte)) {
	client := http.Client{}
	u, err := nup.GetServerUrl(cfg.ServerUrl, exportPath)
	if err != nil {
		log.Fatal("Failed to get server URL: ", err)
	}

	cursor := ""
	for {
		u.RawQuery = fmt.Sprintf("type=%s&max=%d", entityType, batchSize)
		if len(extraArgs) > 0 {
			u.RawQuery += "&" + extraArgs
		}
		if len(cursor) > 0 {
			u.RawQuery += "&cursor=" + cursor
		}

		req, err := http.NewRequest("GET", u.String(), nil)
		if err != nil {
			log.Fatal("Failed to create request: ", err)
		}
		req.SetBasicAuth(cfg.Username, cfg.Password)

		resp, err := client.Do(req)
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

func getSongs(cfg *nup.ClientConfig, batchSize int, includeCovers bool, ch chan *nup.Song) {
	extraArgs := "covers="
	if includeCovers {
		extraArgs += "1"
	} else {
		extraArgs += "0"
	}

	getEntities(cfg, "song", extraArgs, batchSize, func(b []byte) {
		var s nup.Song
		if err := json.Unmarshal(b, &s); err == nil {
			ch <- &s
		} else {
			log.Fatalf("Got unexpected line from server: %v", string(b))
		}
	})
	ch <- nil
}

func getPlays(cfg *nup.ClientConfig, batchSize int, ch chan *nup.PlayDump) {
	getEntities(cfg, "play", "", batchSize, func(b []byte) {
		var pd nup.PlayDump
		if err := json.Unmarshal(b, &pd); err == nil {
			ch <- &pd
		} else {
			log.Fatalf("Got unexpected line from server: %v", string(b))
		}
	})
	ch <- nil
}

func main() {
	songBatchSize := flag.Int("song-batch-size", defaultSongBatchSize, "Size for each batch of entities")
	playBatchSize := flag.Int("play-batch-size", defaultPlayBatchSize, "Size for each batch of entities")
	configFile := flag.String("config", "", "Path to config file")
	includeCovers := flag.Bool("covers", false, "Include cover filenames")
	flag.Parse()

	var cfg nup.ClientConfig
	if err := nup.ReadJSON(*configFile, &cfg); err != nil {
		log.Fatal("Unable to read config file: ", err)
	}

	songChan := make(chan *nup.Song, chanSize)
	go getSongs(&cfg, *songBatchSize, *includeCovers, songChan)

	playChan := make(chan *nup.PlayDump, chanSize)
	go getPlays(&cfg, *playBatchSize, playChan)

	e := json.NewEncoder(os.Stdout)

	numSongs := 0
	pd := <-playChan
	for {
		s := <-songChan
		if s == nil {
			break
		}

		for pd != nil && pd.SongId == s.SongId {
			s.Plays = append(s.Plays, pd.Play)
			pd = <-playChan
		}

		if err := e.Encode(s); err != nil {
			log.Fatal("Failed to encode song: %v", err)
		}

		numSongs++
		if numSongs%progressInterval == 0 {
			log.Printf("Wrote %d songs", numSongs)
		}
	}
	log.Printf("Wrote %d songs", numSongs)

	if pd != nil {
		log.Fatal("Got orphaned play for song %v: %v", pd.SongId, pd.Play)
	}
}
