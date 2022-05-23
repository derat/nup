// Copyright 2021 Daniel Erat.
// All rights reserved.

package main

import "testing"

func TestParseRangeHeader(t *testing.T) {
	for _, tc := range []struct {
		head       string
		start, end int64
		ok         bool
	}{
		{"", 0, -1, true},                        // empty header
		{"bytes=0-", 0, -1, true},                // open end with zero start
		{"bytes=123-", 123, -1, true},            // open end with nonzero start
		{"bytes=0-123", 0, 123, true},            // zero start and nonzero end
		{"bytes=123-456", 123, 456, true},        // nonzero start and end
		{"bytes=123-456, 789-1234", 0, 0, false}, // multiple ranges unsupported
		{"bytes=-456", 0, 0, false},              // suffix unsupported
	} {
		if start, end, ok := parseRangeHeader(tc.head); start != tc.start || end != tc.end || ok != tc.ok {
			t.Errorf("parseRangeHeader(%q) = (%v, %v, %v); want (%v, %v, %v)",
				tc.head, start, end, ok, tc.start, tc.end, tc.ok)
		}
	}
}
