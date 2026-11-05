// Package storage abstracts file content storage. The MVP ships a local
// filesystem implementation; an S3 implementation can be added later without
// touching call sites.
package storage

import (
	"context"
	"io"
)

// Info describes a stored object.
type Info struct {
	Size int64
}

// File is an opened stored object.
type File interface {
	io.ReadSeekCloser
	Stat() (Info, error)
}

// Store persists opaque blobs addressed by keys. Keys are owned by the
// application (e.g. "<bucketID>/<messageID>").
type Store interface {
	Save(ctx context.Context, key string, r io.Reader) (Info, error)
	Open(ctx context.Context, key string) (File, error)
	Delete(ctx context.Context, key string) error
}
