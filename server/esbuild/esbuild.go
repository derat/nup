// Copyright 2022 Daniel Erat.
// All rights reserved.

// esbuild contains code for transpiling and bundling TypeScript and JavaScript code.
package esbuild

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"
)

const (
	// SourceMapExt contains the suffix that esbuild appends to a file (e.g. "foo.js")
	// for its source map ("foo.js.map"). The source map should be served at this location
	// since the code contains a comment referencing it.
	SourceMapExt = ".map"

	charset = api.CharsetUTF8
	format  = api.FormatESModule
	target  = api.ES2020
)

// Transform transforms the supplied code to an ES module.
// loader (api.LoaderTS or api.LoaderJS) describes src's language.
// If fn is supplied, it will be used as the filename in error messages.
func Transform(src []byte, loader api.Loader, minify bool, fn string) ([]byte, error) {
	res := api.Transform(string(src), api.TransformOptions{
		Charset:           charset,
		Format:            format,
		Loader:            loader,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		MinifyWhitespace:  minify,
		Sourcefile:        fn,
		Target:            target,
	})
	if len(res.Errors) > 0 {
		return nil, errors.New(getMessage(res.Errors[0]))
	}
	return res.Code, nil
}

// Bundle builds the supplied files into a single ES module.
func Bundle(dir string, entryPoints []string, outFile string, minify bool) (
	js, sourceMap []byte, err error) {
	res := api.Build(api.BuildOptions{
		AbsWorkingDir:     dir,
		Bundle:            true,
		Charset:           charset,
		EntryPoints:       entryPoints,
		Format:            format,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		MinifyWhitespace:  minify,
		Outfile:           outFile,
		Sourcemap:         api.SourceMapLinked,
		Target:            target,
	})
	if len(res.Errors) > 0 {
		return nil, nil, errors.New(getMessage(res.Errors[0]))
	}
	if n := len(res.OutputFiles); n != 2 {
		return nil, nil, fmt.Errorf("got %d output files; want 2", n)
	}
	for _, of := range res.OutputFiles {
		switch filepath.Base(of.Path) {
		case outFile:
			js = of.Contents
		case outFile + SourceMapExt:
			sourceMap = of.Contents
		default:
			return nil, nil, fmt.Errorf("unexpected output file %q", of.Path)
		}
	}
	return js, sourceMap, nil
}

func getMessage(msg api.Message) string {
	var s string
	if loc := msg.Location; loc != nil {
		s = fmt.Sprintf("%s:%d:%d: ", filepath.Base(loc.File), loc.Line, loc.Column)
	}
	s += msg.Text
	return s
}
