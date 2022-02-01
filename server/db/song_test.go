// Copyright 2022 Daniel Erat.
// All rights reserved.

package db

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"foo", "foo"},
		{"Foo", "foo"},
		{"N*E*R*D", "n*e*r*d"},
		{"Björk", "bjork"},
		{"múm", "mum"},
		{"Björk feat. múm", "bjork feat. mum"},
		{"Queensrÿche", "queensryche"},
		{"mañana", "manana"},
		{"Antônio Carlos Jobim", "antonio carlos jobim"},
		{"Mattias Häggström Gerdt", "mattias haggstrom gerdt"},
		{"Tomáš Dvořák", "tomas dvorak"},
		{"µ-Ziq", "μ-ziq"}, // MICRO SIGN mapped to GREEK SMALL LETTER MU
		{"μ-Ziq", "μ-ziq"}, // GREEK SMALL LETTER MU unchanged
		{"2winz²", "2winz2"},
		{"®", "®"}, // surprised that this doesn't become "r"!
		{"™", "tm"},
		{"✝", "✝"},
		{"…", "..."},
		{"Сергей Васильевич Рахманинов", "сергеи васильевич рахманинов"},
		{"永田権太", "永田権太"},
	} {
		if got, err := Normalize(tc.in); err != nil {
			t.Errorf("Normalize(%q) failed: %v", tc.in, err)
		} else if got != tc.want {
			t.Errorf("Normalize(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}
