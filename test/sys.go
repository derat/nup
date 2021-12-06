// Copyright 2021 Daniel Erat.
// All rights reserved.

package test

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Caller walks down the call stack and returns the first test file
// that it sees as e.g. "foo_test.go:53".
func Caller() string {
	for skip := 1; true; skip++ {
		_, file, line, ok := runtime.Caller(skip)
		if !ok {
			break
		}
		if strings.HasSuffix(file, "_test.go") {
			return fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
	}
	return "unknown"
}

// FindUnusedPorts returns n unused TCP ports.
func FindUnusedPorts(n int) ([]int, error) {
	ports := make([]int, n)
	for i := range ports {
		ls, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, err
		}
		defer ls.Close()
		ports[i] = ls.Addr().(*net.TCPAddr).Port
	}
	return ports, nil
}

// HandleSignals installs a signal handler that sends SIGTERM to the current process group
// and exits with status 1.
func HandleSignals(sigs ...os.Signal) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)

	go func() {
		var sig = <-ch
		log.Printf("Received %s; cleaning up", sig)
		if pgid, err := unix.Getpgid(os.Getpid()); err != nil {
			log.Print("Failed getting process group: ", err)
		} else {
			const signum = syscall.SIGTERM
			log.Printf("Sending %d to process group %v", signum, pgid)
			if err := unix.Kill(-pgid, signum); err != nil {
				log.Printf("Killing %v failed: %v", pgid, err)
			}
		}
		os.Exit(1)
	}()
}

// CallerDir returns the caller's directory relative to the current directory.
func CallerDir() (string, error) {
	_, p, _, ok := runtime.Caller(1)
	if !ok {
		return "", errors.New("unable to get runtime caller info")
	}
	return filepath.Dir(p), nil
}

// ServeFiles starts an httptest.Server for dir.
//
// The server sets the Access-Control-Allow-Credentials and Access-Control-Allow-Origin headers to
// allow requests from any origin. It also sets the Cache-Control header to disable caching (but
// note that Chrome sometimes ignores this and caches MP3 files anyway).
func ServeFiles(dir string) *httptest.Server {
	fs := http.FileServer(http.Dir(dir))
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		w.Header().Set("Cache-Control", "no-store")
		fs.ServeHTTP(w, r)
	}))
}
