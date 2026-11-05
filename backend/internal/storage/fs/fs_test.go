package fs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
)

func TestSaveOpenDeleteRoundtrip(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	payload := bytes.Repeat([]byte("fling-"), 10000)
	info, err := s.Save(ctx, "bucket/msg", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", info.Size, len(payload))
	}

	f, err := s.Open(ctx, "bucket/msg")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("content mismatch")
	}
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size != int64(len(payload)) {
		t.Fatalf("stat size = %d", fi.Size)
	}

	if err := s.Delete(ctx, "bucket/msg"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, "bucket/msg"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open after delete: %v", err)
	}
	// Deleting a missing key is not an error.
	if err := s.Delete(ctx, "bucket/msg"); err != nil {
		t.Fatal(err)
	}
}

func TestSaveOverwrite(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := s.Save(ctx, "k", bytes.NewReader([]byte("a"))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(ctx, "k", bytes.NewReader([]byte("bb"))); err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, _ := io.ReadAll(f)
	if string(got) != "bb" {
		t.Fatalf("got %q", got)
	}
}
