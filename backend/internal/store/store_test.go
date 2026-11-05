package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestBucketFlow(t *testing.T) {
	s := newTestStore(t)

	b, err := s.CreateBucket("photos", "hash", "salt", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !b.HasPIN {
		t.Fatal("expected HasPIN")
	}

	got, err := s.GetBucket(b.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBucket: %v %#v", err, got)
	}
	if got.Name != "photos" || !got.HasPIN {
		t.Fatalf("unexpected bucket: %+v", got)
	}

	hash, salt, err := s.BucketPIN(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "hash" || salt != "salt" {
		t.Fatalf("pin = %q/%q", hash, salt)
	}

	open, err := s.CreateBucket("open", "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	hash, _, err = s.BucketPIN(open.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hash != "" {
		t.Fatalf("expected empty pin hash, got %q", hash)
	}

	list, err := s.ListBuckets("")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d", len(list))
	}
}

func TestPrivateBuckets(t *testing.T) {
	s := newTestStore(t)

	pub, _ := s.CreateBucket("pub", "", "", "", false)
	priv, _ := s.CreateBucket("priv", "", "", "alice", true)

	// Owner sees both, stranger and anonymous see only public.
	for user, want := range map[string]int{"alice": 2, "bob": 1, "": 1} {
		list, err := s.ListBuckets(user)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != want {
			t.Fatalf("user %q: len = %d, want %d", user, len(list), want)
		}
	}

	got, err := s.GetBucket(priv.ID)
	if err != nil || got == nil {
		t.Fatalf("GetBucket: %v %#v", err, got)
	}
	if !got.Private || got.Owner != "alice" {
		t.Fatalf("unexpected private bucket: %+v", got)
	}
	if pub.Private {
		t.Fatal("public bucket marked private")
	}
}

func TestMessagesAndCascade(t *testing.T) {
	s := newTestStore(t)
	b, _ := s.CreateBucket("b1", "", "", "", false)

	for _, content := range []string{"one", "two", "three"} {
		if err := s.AddMessage(&Message{BucketID: b.ID, Kind: "text", Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddMessage(&Message{BucketID: b.ID, Kind: "file", Filename: "a.bin", Size: 5, StorageKey: b.ID + "/k1"}); err != nil {
		t.Fatal(err)
	}

	msgs, err := s.ListMessages(b.ID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("len = %d", len(msgs))
	}
	if msgs[3].Filename != "a.bin" || msgs[3].StorageKey == "" {
		t.Fatalf("bad file message: %+v", msgs[3])
	}

	// Incremental fetch after the second message.
	inc, err := s.ListMessages(b.ID, msgs[1].ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(inc) != 2 || inc[0].ID != msgs[2].ID {
		t.Fatalf("incremental: %+v", inc)
	}

	keys, err := s.FileKeysByBucket(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != b.ID+"/k1" {
		t.Fatalf("keys = %v", keys)
	}

	if err := s.DeleteBucket(b.ID); err != nil {
		t.Fatal(err)
	}
	msgs, err = s.ListMessages(b.ID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected cascade delete, got %d", len(msgs))
	}
}

func TestPINAttempts(t *testing.T) {
	s := newTestStore(t)

	attempts, lockedUntil, err := s.PINState("fp1", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || !lockedUntil.IsZero() {
		t.Fatalf("initial state: %d %v", attempts, lockedUntil)
	}

	// 3 failed attempts of 3 max → lockout on the third.
	lockout := time.Minute
	n, locked, err := s.AddPINAttempt("fp1", "b1", lockout, 3)
	if err != nil || locked || n != 1 {
		t.Fatalf("attempt1: %d %v %v", n, locked, err)
	}
	n, locked, err = s.AddPINAttempt("fp1", "b1", lockout, 3)
	if err != nil || locked || n != 2 {
		t.Fatalf("attempt2: %d %v %v", n, locked, err)
	}
	n, locked, err = s.AddPINAttempt("fp1", "b1", lockout, 3)
	if err != nil || !locked || n != 3 {
		t.Fatalf("attempt3: %d %v %v", n, locked, err)
	}

	_, lockedUntil, err = s.PINState("fp1", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(lockedUntil) <= lockout-time.Minute || time.Until(lockedUntil) > lockout {
		t.Fatalf("locked_until = %v", lockedUntil)
	}

	// Another fingerprint/bucket pair is independent.
	n, locked, err = s.AddPINAttempt("fp2", "b1", lockout, 3)
	if err != nil || locked || n != 1 {
		t.Fatalf("fp2: %d %v %v", n, locked, err)
	}

	if err := s.ResetPINAttempts("fp1", "b1"); err != nil {
		t.Fatal(err)
	}
	attempts, lockedUntil, err = s.PINState("fp1", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || !lockedUntil.IsZero() {
		t.Fatalf("after reset: %d %v", attempts, lockedUntil)
	}
}
