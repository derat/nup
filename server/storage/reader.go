// Copyright 2021 Daniel Erat.
// All rights reserved.

package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/log"

	"cloud.google.com/go/storage"
)

// ObjectReader implements io.ReadCloser and io.ReadSeeker for reading a Cloud Storage object.
type ObjectReader struct {
	ctx     context.Context
	cl      *storage.Client
	obj     *storage.ObjectHandle
	r       *storage.Reader
	pos     int64
	size    int64
	lastMod time.Time
}

func NewObjectReader(ctx context.Context, bucket, name string) (*ObjectReader, error) {
	// Tests shouldn't be trying to access Cloud Storage.
	if appengine.IsDevAppServer() {
		return nil, errors.New("accessing bucket from test")
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	handle := client.Bucket(bucket).Object(name)
	attrs, err := handle.Attrs(ctx)
	if err != nil {
		client.Close()
		if err == storage.ErrObjectNotExist {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	log.Debugf(ctx, "Creating reader for %q in %v with size %d", name, bucket, attrs.Size)
	return &ObjectReader{
		ctx:     ctx,
		cl:      client,
		obj:     handle,
		size:    attrs.Size,
		lastMod: attrs.Updated,
	}, nil
}

// Name returns the object's name within its bucket.
func (or *ObjectReader) Name() string {
	return or.obj.ObjectName()
}

// Size returns the object's full size in bytes.
func (or *ObjectReader) Size() int64 { return or.size }

// LastMod returns the object's last-modified time.
func (or *ObjectReader) LastMod() time.Time { return or.lastMod }

func (or *ObjectReader) Close() error {
	var err error
	if or.r != nil {
		err = or.r.Close()
	}
	if cerr := or.cl.Close(); err == nil {
		err = cerr
	}
	return err
}

func (or *ObjectReader) Read(buf []byte) (int, error) {
	if or.r == nil {
		// We need to read to the end of the object since we don't yet know if there
		// will be additional reads without seeking.
		var err error
		if or.r, err = or.obj.NewRangeReader(or.ctx, or.pos, -1); err != nil {
			return 0, err
		}
	}
	n, err := or.r.Read(buf)
	or.pos += int64(n)
	return n, err
}

func (or *ObjectReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		or.pos = offset
	case io.SeekCurrent:
		or.pos += offset
	case io.SeekEnd:
		or.pos = or.size + offset
	}
	if or.r != nil {
		or.r.Close()
		or.r = nil
	}
	return or.pos, nil
}
