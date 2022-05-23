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
	"time"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/json"
	"github.com/tdewolff/minify/v2/svg"
)

const staticDir = "web"

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

	// Don't use the minify package on JS files. Given a statement like "static THEME = 'theme';",
	// it fails with "expected ( instead of = in method definition".
}

// staticFile contains data about a static file to be served over HTTP.
type staticFile struct {
	data  []byte
	mtime time.Time
}

// readStaticFile reads the file at the specified path within staticDir.
func readStaticFile(p string, minify bool) (*staticFile, error) {
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
	return &sf, err
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
