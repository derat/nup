// Copyright 2022 Daniel Erat.
// All rights reserved.

package web

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
)

const webDir = "../../../web"

func TestTypeScript(t *testing.T) {
	cmd := exec.Command("tsc",
		"--allowJs",
		"--forceConsistentCasingInFileNames",
		"--noEmit",
		"--noFallthroughCasesInSwitch",
		"--noImplicitAny",
		"--noUnusedLocals",
		"--strict",
		"--target", "es2020",
		filepath.Join(webDir, "index.ts"),
		filepath.Join(webDir, "global.d.ts"),
	)
	if stdout, err := cmd.Output(); err != nil {
		t.Errorf("tsc failed: %v\n%s", err,
			bytes.ReplaceAll(stdout, []byte(webDir+"/"), nil))
	}
}
