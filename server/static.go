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
	staticDir = "web" // directory containing static files, relative to app

	bundleFile       = "bundle.js" // generated file containing bundled JS
	bundleEntryPoint = "index.ts"  // entry point used to create bundle

	indexFile         = "index.html" // file that initially loads JS
	scriptPlaceholder = "{{SCRIPT}}" // script placeholder in indexFile
)

const (
	cssType  = "text/css"
	htmlType = "text/html"
	jsType   = "application/javascript"
	jsonType = "application/json"
	svgType  = "image/svg+xml"
	tsType   = "application/typescript"
)

var extTypes = map[string]string{
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
	// JS and TS are minified by esbuild.
}

// staticFiles maps from a staticKey to a []byte containing the content of
// the previously-loaded and -processed static file.
var staticFiles sync.Map

// staticKey contains arguments passed to getStaticFile.
type staticKey struct {
	path   string // relative request path (e.g. "index.html")
	minify bool
}

// getStaticFile returns the contents of the file at the specified path
// (without a leading slash) within staticDir.
//
// Files are transformed in various ways:
//  - If minify is true, the returned file is minified.
//  - If indexFile is requested, scriptPlaceholder is replaced by bundleFile
//    if minify is true or by the JS version of bundleEntryPoint otherwise.
//  - If bundleFile is requested, bundleEntryPoint and all of its dependencies
//    are returned as a single ES module. The code is minified regardless of
//    whether minification was requested.
//  - If a nonexistent .js file is requested, its .ts counterpart is transpiled
//    and returned.
func getStaticFile(p string, minify bool) ([]byte, error) {
	key := staticKey{p, minify}
	if b, ok := staticFiles.Load(key); ok {
		return b.([]byte), nil
	}

	// The bundle file doesn't actually exist on-disk, so handle it first.
	if p == bundleFile {
		b, err := buildBundle()
		if err != nil {
			return nil, err
		}
		staticFiles.Store(key, b)
		return b, nil
	}

	fp := filepath.Join(staticDir, p)
	if !strings.HasPrefix(fp, staticDir+"/") {
		return nil, os.ErrNotExist
	}

	ext := filepath.Ext(fp)
	ctype := extTypes[ext]
	f, err := os.Open(fp)
	if os.IsNotExist(err) && ext == ".js" {
		// If a .js file doesn't exist, try to read its .ts counterpart.
		f, err = os.Open(replaceSuffix(fp, ".js", ".ts"))
		ctype = tsType
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var b []byte
	if minify && ctype != "" {
		if b, err = minifyAndTransformData(f, ctype); err != nil {
			return nil, err
		}
	} else {
		if b, err = ioutil.ReadAll(f); err != nil {
			return nil, err
		}
		// Transform TypeScript code to JavaScript. esbuild also appears to do some
		// degree of minimization no matter what: blank lines and comments are stripped.
		if ctype == tsType {
			if b, err = esbuild.Transform(b, api.LoaderTS, false); err != nil {
				return nil, err
			}
		}
	}

	// Make index.html load the appropriate script depending on whether minification is enabled.
	if p == indexFile {
		ep := replaceSuffix(bundleEntryPoint, ".ts", ".js")
		if minify {
			ep = bundleFile
		}
		b = bytes.ReplaceAll(b, []byte(scriptPlaceholder), []byte(ep))
	}

	staticFiles.Store(key, b)
	return b, nil
}

// replaceSuffix returns s with the specified suffix replaced.
// If s doesn't end in from, it is returned unchanged.
func replaceSuffix(s string, from, to string) string {
	if !strings.HasSuffix(s, from) {
		return s
	}
	return strings.TrimSuffix(s, from) + to
}

// minifyData reads from r and returns a minified version of its data.
// ctype should be a value from extTypes, e.g. cssType or htmlType.
// If ctype is jsType or tsType, the data will also be transformed to an ES module.
func minifyAndTransformData(r io.Reader, ctype string) ([]byte, error) {
	switch ctype {
	case jsType, tsType:
		// Minify embedded HTML and CSS and then use esbuild to minify and transform the code.
		b, err := minifyTemplates(r)
		if err != nil {
			return nil, err
		}
		loader := api.LoaderJS
		if ctype == tsType {
			loader = api.LoaderTS
		}
		return esbuild.Transform(b, loader, true)
	default:
		var b bytes.Buffer
		err := minifier.Minify(ctype, &b, r)
		return b.Bytes(), err
	}
}

// minifyTemplates looks for createTemplate() and replaceSync() calls in the
// supplied JavaScript code and minifies the contents as HTML and CSS, respectively.
//
// The opening "createTemplate(`" or ".replaceSync(`" must appear at the end of a
// line and the closing "`);" must appear on a line by itself.
func minifyTemplates(r io.Reader) ([]byte, error) {
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
	return b.Bytes(), sc.Err()
}

// buildBundle builds a single minified bundle file consisting of bundleEntryPoint
// and all of its imports.
func buildBundle() ([]byte, error) {
	// Write all the (possibly minified) .js and .ts files to a temp dir for esbuild.
	// I think it'd be possible to write an esbuild plugin that returns these
	// files from memory, but the plugin API is still experimental and we only
	// hit this code path once per instance.
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
		// Don't minify or transform the code yet (esbuild.Bundle will do that later),
		// but minify embedded HTML and CSS.
		base := filepath.Base(p)
		b, err := getStaticFile(base, false /* minify */)
		if err != nil {
			return nil, err
		}
		if b, err = minifyTemplates(bytes.NewReader(b)); err != nil {
			return nil, err
		}
		if err := ioutil.WriteFile(filepath.Join(td, base), b, 0644); err != nil {
			return nil, err
		}
	}

	return esbuild.Bundle(td, []string{bundleEntryPoint}, bundleFile, true /* minify */)
}
