// Copyright 2022 Daniel Erat.
// All rights reserved.

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// callGetStaticFile moves to the repository root, calls getStaticFile,
// and moves back to the original directory before returning the result.
func callGetStaticFile(t *testing.T, p string, minify bool) []byte {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(".."); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(dir); err != nil {
			t.Fatal(err)
		}
	}()

	b, err := getStaticFile(p, minify)
	if err != nil {
		t.Fatalf("getStaticFile(%q, %v) failed: %v", p, minify, err)
	}
	return b
}

const (
	wsTag    = " <script>" // tag with whitespace in non-minified index.js
	commonFn = "common.js" // file imported by unbundled index.js
)

func TestGetStaticFile_Index_Minify(t *testing.T) {
	out := callGetStaticFile(t, indexFile, true)
	if _, err := html.Parse(bytes.NewReader(out)); err != nil {
		t.Errorf(`getStaticFile(%q, true) returned invalid HTML: %v`, indexFile, err)
	}
	if bytes.Contains(out, []byte(wsTag)) {
		t.Errorf(`getStaticFile(%q, true) contains %q`, indexFile, wsTag)
	}
	if !bytes.Contains(out, []byte(bundleFile)) {
		t.Errorf(`getStaticFile(%q, true) doesn't contain %q`, indexFile, bundleFile)
	}
}

func TestGetStaticFile_Index_NoMinify(t *testing.T) {
	out := callGetStaticFile(t, indexFile, false)
	if _, err := html.Parse(bytes.NewReader(out)); err != nil {
		t.Errorf(`getStaticFile(%q, false) returned invalid HTML: %v`, indexFile, err)
	}
	if !bytes.Contains(out, []byte(wsTag)) {
		t.Errorf(`getStaticFile(%q, false) doesn't contain %q`, indexFile, wsTag)
	}
	if want := replaceSuffix(bundleEntryPoint, ".ts", ".js"); !bytes.Contains(out, []byte(want)) {
		t.Errorf(`getStaticFile(%q, false) doesn't contain %q`, indexFile, want)
	}
}

func TestGetStaticFile_Bundle(t *testing.T) {
	if out := callGetStaticFile(t, bundleFile, true); bytes.Contains(out, []byte(commonFn)) {
		t.Errorf("getStaticFile(%q, true) unexpectedly contains %q", bundleFile, commonFn)
	}
}

func TestGetStaticFile_IndexScript(t *testing.T) {
	index := replaceSuffix(bundleEntryPoint, ".ts", ".js")
	if out := callGetStaticFile(t, index, false); !bytes.Contains(out, []byte(commonFn)) {
		t.Errorf("getStaticFile(%q, false) doesn't contain %q", index, commonFn)
	}
}

func TestGetStaticFile_Transform(t *testing.T) {
	const fn = "common.js"
	const funcName = "createElement"
	if out := callGetStaticFile(t, fn, true); !bytes.Contains(out, []byte(funcName)) {
		t.Errorf("getStaticFile(%q, true) doesn't contain %q", fn, funcName)
	}
}

func TestMinifyAndTransformData(t *testing.T) {
	for _, tc := range []struct {
		in, ctype, want string
	}{
		{"body {\n  margin: 0;\n}\n", cssType, "body{margin:0}"},
		{"<!DOCTYPE html>\n<html>\nfoo\n</html>\n", htmlType, "<!doctype html>foo"},
		{"{\n  \"foo\": 2\n}\n", jsonType, `{"foo":2}`},
		{"// comment\nexport const foo = 3;\n", jsType, "const o=3;export{o as foo};\n"},
		{"// comment\nexport const foo: number = 3;\n", tsType, "const o=3;export{o as foo};\n"},
	} {
		if got, err := minifyAndTransformData(strings.NewReader(tc.in), tc.ctype); err != nil {
			t.Errorf("minifyAndTransformData(%q, %q) failed: %v", tc.in, tc.ctype, err)
		} else if string(got) != tc.want {
			t.Errorf("minifyAndTransformData(%q, %q) = %q; want %q", tc.in, tc.ctype, got, tc.want)
		}
	}
}

func TestMinifyTemplates(t *testing.T) {
	const bt = "`"
	got, err := minifyTemplates(strings.NewReader(`
// Here's an HTML template:
const html = createTemplate(` + bt + `
<!DOCTYPE html>
<html>
foo
</html>
` + bt + `);

// And here's some CSS:
const css = new CSSStyleSheet();
css.replaceSync(` + bt + `
body {
  margin: 0;
}
` + bt + `);

const foo = 3;
`))

	if err != nil {
		t.Fatal("minifyTemplates failed: ", err)
	}
	if want := `
// Here's an HTML template:
const html = createTemplate(` + bt + `<!doctype html>foo` + bt + `);

// And here's some CSS:
const css = new CSSStyleSheet();
css.replaceSync(` + bt + `body{margin:0}` + bt + `);

const foo = 3;
`; string(got) != want {
		t.Errorf("minifyTemplates returned:\n%s\nwant:\n%s", got, want)
	}
}
