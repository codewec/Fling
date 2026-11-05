// Command seed populates a Fling database with demo data: a few buckets
// (public, pinned, PIN-protected, private) and sample messages, including a
// small text file uploaded through the storage abstraction.
//
// Usage:
//
//	go run ./cmd/seed -db ./data/fling.db -files ./data/files
package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"path/filepath"

	"fling/internal/auth"
	"fling/internal/storage/fs"
	"fling/internal/store"
)

func main() {
	dbPath := flag.String("db", "./data/fling.db", "path to the SQLite database")
	filesDir := flag.String("files", "./data/files", "path to the file storage directory")
	owner := flag.String("owner", "alice", "owner identity for the private bucket (set via AUTH_USER_HEADER)")
	flag.Parse()

	st, err := store.New(*dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	filesDirAbs, err := filepath.Abs(*filesDir)
	if err != nil {
		log.Fatalf("files dir: %v", err)
	}
	files, err := fs.New(filesDirAbs)
	if err != nil {
		log.Fatalf("file storage: %v", err)
	}
	ctx := context.Background()

	addText := func(bucketID, sender, content string, pinned bool) {
		m := &store.Message{BucketID: bucketID, Kind: "text", Content: content, Sender: sender}
		if err := st.AddMessage(m); err != nil {
			log.Fatalf("add message: %v", err)
		}
		if pinned {
			if err := st.SetMessagePinned(m.ID, true); err != nil {
				log.Fatalf("pin message: %v", err)
			}
		}
	}

	// Public bucket with a conversation and a pinned message.
	open, err := st.CreateBucket("General", "", "", "", false)
	if err != nil {
		log.Fatalf("create bucket: %v", err)
	}
	addText(open.ID, "alice", "Welcome to Fling! Drop files here to move them between devices.", true)
	addText(open.ID, "bob", "Nice — like a clipboard that lives on the LAN.", false)
	addText(open.ID, "alice", "Exactly. Text is selectable, files are one click away.", false)

	content := []byte("# Fling demo\n\nThis text file was created by the seed script.\n")
	key := open.ID + "/" + store.NewID()
	info, err := files.Save(ctx, key, bytes.NewReader(content))
	if err != nil {
		log.Fatalf("save file: %v", err)
	}
	if err := st.AddMessage(&store.Message{
		BucketID: open.ID, Kind: "file", Filename: "notes.md",
		Mime: "text/markdown", Size: info.Size, StorageKey: key, Sender: "alice",
	}); err != nil {
		log.Fatalf("add file message: %v", err)
	}

	// Pinned bucket.
	pinned, err := st.CreateBucket("Quick transfer", "", "", "", false)
	if err != nil {
		log.Fatalf("create bucket: %v", err)
	}
	if err := st.SetBucketPinned(pinned.ID, true); err != nil {
		log.Fatalf("pin bucket: %v", err)
	}
	addText(pinned.ID, "", "Temporary stuff goes here.", false)

	// PIN-protected bucket (PIN 1234).
	salt, err := auth.NewSalt()
	if err != nil {
		log.Fatalf("salt: %v", err)
	}
	locked, err := st.CreateBucket("PIN protected", auth.HashPIN("1234", salt), salt, "", false)
	if err != nil {
		log.Fatalf("create bucket: %v", err)
	}
	addText(locked.ID, "alice", "This bucket opens with PIN 1234.", false)

	// Private bucket, visible only to -owner.
	priv, err := st.CreateBucket("My private stash", "", "", *owner, true)
	if err != nil {
		log.Fatalf("create bucket: %v", err)
	}
	addText(priv.ID, *owner, "Only you can see this bucket.", false)

	log.Printf("seeded %s with %d buckets", *dbPath, 4)
}
