package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"

	"erat.org/cloud"
	"erat.org/nup"
)

const (
	oauthScope = "https://www.googleapis.com/auth/userinfo.email"
	exportPath = "export"
)

func main() {
	configFile := flag.String("config", "", "Path to config file")
	flag.Parse()

	var cfg nup.ClientConfig
	if err := cloud.ReadJson(*configFile, &cfg); err != nil {
		log.Fatal("Unable to read config file: ", err)
	}

	client := http.Client{}

	var songCursor, playCursor string
	for {
		u, err := nup.GetServerUrl(cfg.ServerUrl, exportPath)
		if err != nil {
			log.Fatal("Failed to get server URL: ", err)
		}
		if len(songCursor) > 0 {
			u.RawQuery = "songCursor=" + songCursor + "&playCursor=" + playCursor
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

		songCursor = ""
		playCursor = ""
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var s nup.Song
			var cursors []interface{}
			if err := json.Unmarshal(scanner.Bytes(), &s); err == nil {
				os.Stdout.Write(scanner.Bytes())
				os.Stdout.WriteString("\n")
			} else if err := json.Unmarshal(scanner.Bytes(), &cursors); err == nil {
				songCursor = cursors[0].(string)
				playCursor = cursors[1].(string)
			} else {
				log.Fatalf("Got unexpected line from server: %v", scanner.Text())
			}
		}
		if err = scanner.Err(); err != nil {
			log.Fatal("Got error while reading from server: ", err)
		}

		if len(songCursor) == 0 {
			break
		}
	}
}
