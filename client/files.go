// Copyright 2021 Daniel Erat.
// All rights reserved.

package client

import (
	"path/filepath"
	"strings"
)

// IsMusicPath returns true if path p has an extension suggesting that it's a music file.
func IsMusicPath(p string) bool {
	return strings.ToLower(filepath.Ext(p)) == ".mp3"
}
