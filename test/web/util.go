// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"time"
)

const (
	waitTimeout = 10 * time.Second
	waitSleep   = 10 * time.Millisecond
)

// wait calls waitFull with reasonable defaults.
func wait(f func() error) error {
	return waitFull(f, waitTimeout, waitSleep)
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
