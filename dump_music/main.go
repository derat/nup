package main

import (
	"flag"
	"io"
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

	u, err := nup.GetServerUrl(cfg, exportPath)
	if err != nil {
		log.Fatal("Failed to get server URL: ", err)
	}
	transport, err := cloud.NewTransport(cfg.ClientId, cfg.ClientSecret, oauthScope, cfg.TokenCache)
	if err != nil {
		log.Fatal("Failed to create transport: ", err)
	}

	resp, err := transport.Client().Get(u.String())
	if err != nil {
		log.Fatalf("Failed to fetch %v: %v", u.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Fatal("Got non-OK status: ", resp.Status)
	}
	if _, err = io.Copy(os.Stdout, resp.Body); err != nil {
		log.Fatal("Got error while reading from server: ", err)
	}
}
