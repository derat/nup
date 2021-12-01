// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flag]... <dir>\n", os.Args[0])
		flag.PrintDefaults()
	}
	addr := flag.String("addr", "localhost:8000", "addr:port to bind")
	flag.Parse()

	if len(flag.Args()) != 1 {
		flag.Usage()
		os.Exit(2)
	}
	dir := flag.Arg(0)

	fmt.Printf("Serving %v on %v\n", dir, *addr)
	http.Handle("/", addHeaders(http.FileServer(http.Dir(dir))))
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal("Serving failed: ", err)
	}
}

// addHeaders returns a handler func that sets the Access-Control-Allow-Origin
// header (allowing cross-origin requests) before forwarding to h. See e.g.
// https://groups.google.com/g/golang-nuts/c/Upzqsbu2zbo
//
// It also sets the Cache-Control header to disable caching (but note that Chrome
// sometimes ignores this and caches MP3 files anyway).
func addHeaders(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	}
}
