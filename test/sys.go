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
	"time"

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

// HandleSignals installs a signal handler for sigs that sends SIGTERM to the current process group,
// runs f (in a goroutine) if non-nil, and then exits with 1.
func HandleSignals(sigs []os.Signal, f func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sigs...)

	go func() {
		var sig = <-ch
		log.Printf("Received %s; cleaning up", sig)

		if pgid, err := unix.Getpgid(os.Getpid()); err != nil {
			log.Print("Failed getting process group: ", err)
		} else {
			log.Print("Sending SIGTERM to process group ", pgid)
			if err := unix.Kill(-pgid, syscall.SIGTERM); err != nil {
				log.Printf("Killing %v failed: %v", pgid, err)
			}
		}

		if f != nil {
			f()
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

// TempDirPattern takes a base name like "nup_e2e_test" and returns a pattern
// like "nup_e2e_test-20211214_160354.*" to pass to ioutil.TempDir.
func TempDirPattern(base string) string {
	return fmt.Sprintf("%s-%s.*", base, time.Now().Format("20060102_150405"))
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
