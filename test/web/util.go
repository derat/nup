// Copyright 2021 Daniel Erat.
// All rights reserved.

package web

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
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

// caller walks down the call stack and returns the first test file
// that it sees as e.g. "foo_test.go:53".
func caller() string {
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
