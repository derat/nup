// Copyright 2021 Daniel Erat.
// All rights reserved.

package storage

import (
	"context"
	"io"
	"time"

	"google.golang.org/appengine/log"

	"cloud.google.com/go/storage"
)

// ObjectReader implements io.ReadCloser and io.ReadSeeker for reading a Cloud Storage object.
type ObjectReader struct {
	ctx  context.Context
	cl   *storage.Client
	obj  *storage.ObjectHandle
	r    *storage.Reader
	pos  int64
	size int64

	// LastMod is the object's last-modified time.
	LastMod time.Time
}

func NewObjectReader(ctx context.Context, bucket, name string) (*ObjectReader, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	handle := client.Bucket(bucket).Object(name)
	attrs, err := handle.Attrs(ctx)
	if err != nil {
		client.Close()
		return nil, err
	}
	log.Debugf(ctx, "Creating reader for %v:%v with size %d", bucket, name, attrs.Size)
	return &ObjectReader{
		ctx:     ctx,
		cl:      client,
		obj:     handle,
		size:    attrs.Size,
		LastMod: attrs.Updated,
	}, nil
}

func (or *ObjectReader) Close() error {
	// TODO: Report errors, maybe.
	if or.r != nil {
		or.r.Close()
	}
	or.cl.Close()
	return nil
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
