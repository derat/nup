// Copyright 2022 Daniel Erat.
// All rights reserved.

// esbuild contains code for transpiling and bundling TypeScript and JavaScript code.
package esbuild

import (
	"errors"
	"fmt"

	"github.com/evanw/esbuild/pkg/api"
)

const (
	charset = api.CharsetUTF8
	format  = api.FormatESModule
	target  = api.ES2020
)

// Transform transforms the supplied code to an ES module.
// loader (api.LoaderTS or api.LoaderJS) describes src's language.
func Transform(src []byte, loader api.Loader, minify bool) ([]byte, error) {
	res := api.Transform(string(src), api.TransformOptions{
		Charset:           charset,
		Format:            format,
		Loader:            loader,
		MinifyIdentifiers: minify,
		MinifySyntax:      minify,
		MinifyWhitespace:  minify,
		Target:            target,
	})
	if len(res.Errors) > 0 {
		return nil, errors.New(getMessage(res.Errors[0], false))
	}
	return res.Code, nil
}

// Bundle builds the supplied files into a single ES module.
func Bundle(dir string, entryPoints []string, outFile string, minify bool) ([]byte, error) {
	// TODO: Write source map?
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
		Target:            target,
	})
	if len(res.Errors) > 0 {
		return nil, errors.New(getMessage(res.Errors[0], true))
	}
	if n := len(res.OutputFiles); n != 1 {
		return nil, fmt.Errorf("got %d output files; want 1", n)
	}
	return res.OutputFiles[0].Contents, nil
}

func getMessage(msg api.Message, includeFile bool) string {
	var s string
	loc := msg.Location
	if loc != nil {
		if includeFile {
			s += loc.File + ":"
		}
		s += fmt.Sprintf("%d:%d: ", loc.Line, loc.Column)
	}
	s += msg.Text
	return s
}