// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"time"
)

// wait calls waitFull with reasonable defaults.
func wait(f func() error) error {
	// Some tests use a 10-second song, so wait longer than that
	// to avoid races when tests are waiting for playback to compete.
	return waitFull(f, 15*time.Second, 10*time.Millisecond)
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
