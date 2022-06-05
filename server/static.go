// Copyright 2022 Daniel Erat.
// All rights reserved.

package main

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/derat/nup/server/esbuild"

	"github.com/evanw/esbuild/pkg/api"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/svg"
)

const (
	staticDir = "web" // directory relative to app containing static files

	bundleFile       = "bundle.js" // generated file containing bundled JS
	bundleEntryPoint = "index.ts"  // entry point used to create bundle

	indexFile             = "index.html"      // file that initially loads JS
	entryPointPlaceholder = "{{ENTRY_POINT}}" // script placeholder in indexFile
)

const (
	cssType  = "text/css"
	htmlType = "text/html"
	jsType   = "application/javascript"
	jsonType = "application/json"
	svgType  = "image/svg+xml"
	tsType   = "application/typescript"
)

var minifyExts = map[string]string{
	".css":  cssType,
	".html": htmlType,
	".js":   jsType,
	".json": jsonType,
	".svg":  svgType,
	".ts":   tsType,
}

var minifier *minify.M

func init() {
	minifier = minify.New()
	minifier.AddFunc(cssType, css.Minify)
	minifier.Add(htmlType, &html.Minifier{
		KeepDefaultAttrVals: true, // avoid breaking "input[type='text']" selectors
	})
	minifier.AddFunc(jsonType, json.Minify)
	minifier.AddFunc(svgType, svg.Minify)
	// JS and TS are minified by esbuild when creating the bundle.
}

// staticFiles maps from a relative request path (e.g. "index.html") to a []byte
// containing the content of the previously-loaded and -processed static file.
var staticFiles sync.Map

// getStaticFile returns the contents of the file at the specified path
// (without a leading slash) within staticDir.
func getStaticFile(p string, bundle, minify bool) ([]byte, error) {
	if b, ok := staticFiles.Load(p); ok {
		return b.([]byte), nil
	}

	if p == bundleFile && bundle {
		b, err := buildBundle(minify)
		if err != nil {
			return nil, err
		}
		staticFiles.Store(p, b)
		return b, nil
	}

	fp := filepath.Join(staticDir, p)
	if !strings.HasPrefix(fp, staticDir+"/") {
		return nil, os.ErrNotExist
	}

	var ts bool
	ext := filepath.Ext(fp)
	f, err := os.Open(fp)
	if os.IsNotExist(err) && ext == ".js" {
		// If a .js file doesn't exist, try to read a .ts counterpart.
		f, err = os.Open(strings.TrimSuffix(fp, ".js") + ".ts")
		ts = true
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var b []byte
	if ctype, ok := minifyExts[ext]; ok && minify {
		b, err = minifyData(f, ctype)
	} else {
		b, err = ioutil.ReadAll(f)
	}
	if err != nil {
		return nil, err
	}

	// Use esbuild to transform TypeScript into JavaScript.
	if ts {
		if b, err = esbuild.Transform(b, api.LoaderTS, minify); err != nil {
			return nil, err
		}
	}

	// Make index.html load the appropriate script depending on whether bundling is enabled.
	if p == indexFile {
		ep := bundleEntryPoint
		if bundle {
			ep = bundleFile
		}
		b = bytes.ReplaceAll(b, []byte(entryPointPlaceholder), []byte(ep))
	}

	staticFiles.Store(p, b)
	return b, nil
}

// minifyData reads from r and returns a minified version of its data.
// ctype should be a value from minifyExts, e.g. cssType or htmlType.
func minifyData(r io.Reader, ctype string) ([]byte, error) {
	// Super gross: look for createTemplate() and replaceSync() calls and minify the contents as
	// HTML and CSS, respectively. The opening "createTemplate(`" or ".replaceSync(`" must appear at
	// the end of a line and the closing "`);" must appear on a line by itself.
	if ctype == jsType || ctype == tsType {
		var b bytes.Buffer
		var inHTML, inCSS bool
		var quoted string
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			ln := sc.Text()
			inQuoted := inHTML || inCSS
			switch {
			case !inQuoted && strings.HasSuffix(ln, "createTemplate(`"):
				io.WriteString(&b, ln) // omit trailing newline
				inHTML = true
			case !inQuoted && strings.HasSuffix(ln, ".replaceSync(`"):
				io.WriteString(&b, ln) // omit trailing newline
				inCSS = true
			case !inQuoted:
				io.WriteString(&b, ln+"\n")
			case inQuoted && ln == "`);":
				t := htmlType
				if inCSS {
					t = cssType
				}
				min, err := minifier.String(t, quoted)
				if err != nil {
					return nil, err
				}
				io.WriteString(&b, min+ln+"\n")
				inHTML = false
				inCSS = false
				quoted = ""
			case inQuoted:
				quoted += ln + "\n"
			}
		}
		if err := sc.Err(); err != nil {
			return nil, err
		}
		return b.Bytes(), nil
	}

	var b bytes.Buffer
	err := minifier.Minify(ctype, &b, r)
	return b.Bytes(), err
}

// buildBundle builds a single bundle file consisting of bundleEntryPoint and all of its imports.
func buildBundle(minify bool) ([]byte, error) {
	// Write all the (possibly minified) .js and .ts files to a temp dir for esbuild.
	// I think it'd be possible to write an esbuild plugin that returns these
	// files from memory, but the plugin API is still experimental and we only
	// need to do this once at startup.
	td, err := ioutil.TempDir("", "nup_bundle.*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(td)

	paths, err := filepath.Glob(filepath.Join(staticDir, "*.[jt]s"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		base := filepath.Base(p)
		b, err := getStaticFile(base, false /* bundle */, minify)
		if err != nil {
			return nil, err
		}
		if err := ioutil.WriteFile(filepath.Join(td, base), b, 0644); err != nil {
			return nil, err
		}
	}

	return esbuild.Bundle(td, []string{bundleEntryPoint}, bundleFile, minify)
}
