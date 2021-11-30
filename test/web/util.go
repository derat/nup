// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"log"
	"net"
	"time"
)

// wait calls waitFull with reasonable defaults.
func wait(f func() error) error {
	return waitFull(f, 10*time.Second, 10*time.Millisecond)
}

// waitFull waits up to timeout for f to return nil, sleeping sleep between attempts.
func waitFull(f func() error, timeout time.Duration, sleep time.Duration) error {
	start := time.Now()
	for {
		err := f()
		if err == nil {
			return nil
		}
		if time.Now().Sub(start) >= timeout {
			return fmt.Errorf("timed out: %v", err)
		}
		time.Sleep(sleep)
	}
}

// findUnusedPort returns an unused TCP port.
func findUnusedPort() int {
	ls, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatal("Failed finding unused port: ", err)
	}
	defer ls.Close()
	return ls.Addr().(*net.TCPAddr).Port
}
