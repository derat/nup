// Copyright 2022 Daniel Erat.
// All rights reserved.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	bundleEntryPoint = "index.js"  // entry point used to create bundle

	indexFile             = "index.html"      // file that initially loads JS
	entryPointPlaceholder = "{{ENTRY_POINT}}" // script placeholder in indexFile
)

const (
	cssType  = "text/css"
	htmlType = "text/html"
	jsType   = "application/javascript"
	jsonType = "application/json"
	svgType  = "image/svg+xml"
)

var minifyExts = map[string]string{
	".css":  cssType,
	".html": htmlType,
	".js":   jsType,
	".svg":  svgType,
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
	// JS is minified by esbuild when creating the bundle.
}

// staticFiles maps from request path (e.g. "/common.css") to *staticFile structs
// corresponding to previously-loaded and -processed static files.
var staticFiles sync.Map

// staticFile contains data about a static file to be served over HTTP.
type staticFile struct {
	data  []byte
	mtime time.Time
}

// getStaticFile returns the contents of the file at the specified path
// (without a leading slash) within staticDir.
func getStaticFile(p string, bundle, minify bool) (*staticFile, error) {
	if fi, ok := staticFiles.Load(p); ok {
		return fi.(*staticFile), nil
	}

	if p == bundleFile && bundle {
		sf, err := buildBundle(minify)
		if err != nil {
			return nil, err
		}
		staticFiles.Store(p, sf)
		return sf, nil
	}

	fp := filepath.Join(staticDir, p)
	if !strings.HasPrefix(fp, staticDir+"/") {
		return nil, os.ErrNotExist
	}

	var sf staticFile
	fi, err := os.Stat(fp)
	if err != nil {
		return nil, err
	}
	sf.mtime = fi.ModTime()

	f, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if ctype, ok := minifyExts[filepath.Ext(fp)]; ok && minify {
		sf.data, err = minifyData(f, ctype)
	} else {
		sf.data, err = ioutil.ReadAll(f)
	}
	if err != nil {
		return nil, err
	}

	// Make index.html load the appropriate script depending on whether bundling is enabled.
	if p == indexFile {
		ep := bundleEntryPoint
		if bundle {
			ep = bundleFile
		}
		sf.data = bytes.ReplaceAll(sf.data, []byte(entryPointPlaceholder), []byte(ep))
	}

	staticFiles.Store(p, &sf)
	return &sf, nil
}

// minifyData reads from r and returns a minified version of its data.
// ctype should be a value from minifyExts, e.g. cssType or htmlType.
func minifyData(r io.Reader, ctype string) ([]byte, error) {
	// Super gross: look for createTemplate() calls and minify the contents as HTML.
	// The opening "createTemplate(`" must appear at the end of a line and the
	// closing "`);" must appear on a line by itself.
	if ctype == jsType {
		var b bytes.Buffer
		var inTemplate bool
		var template string
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			ln := sc.Text()
			switch {
			case !inTemplate && strings.HasSuffix(ln, "createTemplate(`"):
				io.WriteString(&b, ln) // omit trailing newline
				inTemplate = true
			case !inTemplate:
				io.WriteString(&b, ln+"\n")
			case inTemplate && ln == "`);":
				min, err := minifier.String(htmlType, template)
				if err != nil {
					return nil, err
				}
				io.WriteString(&b, min+ln+"\n")
				inTemplate = false
				template = ""
			case inTemplate:
				template += ln + "\n"
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

// buildBundle builds a single bundle file consisting of
// bundleEntryPoint and all of its imports.
func buildBundle(minify bool) (*staticFile, error) {
	// Write all the (possibly minified) .js files to a temp dir for esbuild.
	// I think it'd be possible to write an esbuild plugin that returns these
	// files from memory, but the plugin API is still experimental and we only
	// need to do this once at startup.
	td, err := ioutil.TempDir("", "nup_bundle.*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(td)

	paths, err := filepath.Glob(filepath.Join(staticDir, "*.js"))
	if err != nil {
		return nil, err
	}
	var mtime time.Time
	for _, p := range paths {
		base := filepath.Base(p)
		sf, err := getStaticFile(base, true /* bundle */, minify)
		if err != nil {
			return nil, err
		}
		if err := ioutil.WriteFile(filepath.Join(td, base), sf.data, 0644); err != nil {
			return nil, err
		}
		if sf.mtime.After(mtime) {
			mtime = sf.mtime
		}
	}

	// TODO: Write source map?
	res := api.Build(api.BuildOptions{
		Bundle:            true,
		EntryPoints:       []string{bundleEntryPoint},
		Outfile:           bundleFile,
		AbsWorkingDir:     td,
		Charset:           api.CharsetUTF8,
		Format:            api.FormatESModule,
		MinifyWhitespace:  minify,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
	})
	if len(res.Errors) > 0 {
		return nil, fmt.Errorf("bundle: %v", res.Errors[0].Text)
	}
	if n := len(res.OutputFiles); n != 1 {
		return nil, fmt.Errorf("got %d output files", n)
	}
	return &staticFile{
		data:  res.OutputFiles[0].Contents,
		mtime: mtime,
	}, nil
}
