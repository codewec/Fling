// Package fs implements storage.Store on the local filesystem.
package fs

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"fling/internal/storage"
)

type Store struct {
	root string
}

// New creates a filesystem store under root, creating it if needed.
func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) path(key string) string {
	return filepath.Join(s.root, filepath.FromSlash(key))
}

func (s *Store) Save(_ context.Context, key string, r io.Reader) (storage.Info, error) {
	final := s.path(key)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return storage.Info{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(final), ".upload-*")
	if err != nil {
		return storage.Info{}, err
	}
	tmpName := tmp.Name()
	size, copyErr := io.Copy(tmp, r)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmpName)
		if copyErr != nil {
			return storage.Info{}, copyErr
		}
		return storage.Info{}, closeErr
	}
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return storage.Info{}, err
	}
	return storage.Info{Size: size}, nil
}

type file struct {
	*os.File
}

func (f file) Stat() (storage.Info, error) {
	fi, err := f.File.Stat()
	if err != nil {
		return storage.Info{}, err
	}
	return storage.Info{Size: fi.Size()}, nil
}

func (s *Store) Open(_ context.Context, key string) (storage.File, error) {
	f, err := os.Open(s.path(key))
	if err != nil {
		return nil, err
	}
	return file{File: f}, nil
}

func (s *Store) Delete(_ context.Context, key string) error {
	if err := os.Remove(s.path(key)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
