// Package store keeps buckets, messages and PIN rate-limit state in SQLite.
// File content lives outside the database in a storage.Store.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Bucket struct {
	ID        string
	Name      string
	HasPIN    bool
	Pinned    bool
	Owner     string // identity from the trusted auth header, "" = anonymous
	Private   bool   // visible to the owner only
	CreatedAt time.Time
}

type Message struct {
	ID         string
	BucketID   string
	Kind       string // "text" | "file"
	Content    string
	Filename   string
	Mime       string
	Size       int64
	StorageKey string
	Sender     string
	Pinned     bool
	CreatedAt  time.Time
}

func New(dbPath string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // sqlite: single writer
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS buckets (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  pin_hash TEXT,
  pin_salt TEXT,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  bucket_id TEXT NOT NULL REFERENCES buckets(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('text','file')),
  content TEXT,
  filename TEXT,
  mime TEXT,
  size INTEGER,
  storage_key TEXT,
  sender TEXT,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_bucket ON messages(bucket_id, created_at, id);
CREATE TABLE IF NOT EXISTS pin_attempts (
  fp TEXT NOT NULL,
  bucket_id TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  locked_until INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (fp, bucket_id)
);`)
	if err != nil {
		return err
	}
	// Add pinned columns to databases created before pinning existed.
	for _, stmt := range []string{
		`ALTER TABLE buckets ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE messages ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE buckets ADD COLUMN owner TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE buckets ADD COLUMN private INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}

// NewID returns a short URL-friendly random code.
func NewID() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])[:8]
}

func (s *Store) CreateBucket(name, pinHash, pinSalt, owner string, private bool) (*Bucket, error) {
	b := &Bucket{ID: NewID(), Name: name, HasPIN: pinHash != "", Owner: owner, Private: private, CreatedAt: time.Now()}
	priv := 0
	if private {
		priv = 1
	}
	_, err := s.db.Exec(`INSERT INTO buckets(id, name, pin_hash, pin_salt, owner, private, created_at) VALUES(?,?,?,?,?,?,?)`,
		b.ID, name, nullIfEmpty(pinHash), nullIfEmpty(pinSalt), owner, priv, b.CreatedAt.Unix())
	if err != nil {
		return nil, err
	}
	return b, nil
}

// ListBuckets returns public buckets plus the requesting user's private ones,
// pinned first, then newest first.
func (s *Store) ListBuckets(user string) ([]*Bucket, error) {
	rows, err := s.db.Query(`SELECT id, name, pin_hash IS NOT NULL, pinned, owner, private, created_at
		FROM buckets WHERE private = 0 OR owner = ?
		ORDER BY pinned DESC, created_at DESC`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Bucket
	for rows.Next() {
		b := &Bucket{}
		var ts int64
		var priv int
		if err := rows.Scan(&b.ID, &b.Name, &b.HasPIN, &b.Pinned, &b.Owner, &priv, &ts); err != nil {
			return nil, err
		}
		b.Private = priv != 0
		b.CreatedAt = time.Unix(ts, 0)
		out = append(out, b)
	}
	return out, rows.Err()
}

// SetBucketPinned marks/unmarks a bucket as pinned (shown at the top of the list).
func (s *Store) SetBucketPinned(id string, pinned bool) error {
	v := 0
	if pinned {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE buckets SET pinned = ? WHERE id = ?`, v, id)
	if err != nil {
		return err
	}
	return ensureAffected(res)
}

func (s *Store) GetBucket(id string) (*Bucket, error) {
	b := &Bucket{}
	var pinHash, pinSalt sql.NullString
	var ts int64
	var priv int
	err := s.db.QueryRow(`SELECT id, name, pin_hash, pin_salt, owner, private, created_at FROM buckets WHERE id = ?`, id).
		Scan(&b.ID, &b.Name, &pinHash, &pinSalt, &b.Owner, &priv, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.HasPIN = pinHash.Valid
	b.Private = priv != 0
	b.CreatedAt = time.Unix(ts, 0)
	return b, nil
}

// BucketPIN returns the stored PIN hash and salt, or nil values for an
// unlocked bucket.
func (s *Store) BucketPIN(id string) (hash, salt string, err error) {
	var pinHash, pinSalt sql.NullString
	err = s.db.QueryRow(`SELECT pin_hash, pin_salt FROM buckets WHERE id = ?`, id).Scan(&pinHash, &pinSalt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if !pinHash.Valid {
		return "", "", nil
	}
	return pinHash.String, pinSalt.String, nil
}

// DeleteBucket removes the bucket, its messages and PIN counters.
func (s *Store) DeleteBucket(id string) error {
	_, err := s.db.Exec(`DELETE FROM buckets WHERE id = ?`, id)
	return err
}

// FileKeysByBucket returns storage keys of file messages for cleanup.
func (s *Store) FileKeysByBucket(bucketID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT storage_key FROM messages WHERE bucket_id = ? AND storage_key IS NOT NULL`, bucketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) AddMessage(m *Message) error {
	if m.ID == "" {
		m.ID = NewID()
	}
	m.CreatedAt = time.Now()
	_, err := s.db.Exec(`INSERT INTO messages(id, bucket_id, kind, content, filename, mime, size, storage_key, sender, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.BucketID, m.Kind, nullIfEmpty(m.Content), nullIfEmpty(m.Filename), nullIfEmpty(m.Mime),
		nullInt64(m.Size), nullIfEmpty(m.StorageKey), nullIfEmpty(m.Sender), m.CreatedAt.Unix())
	return err
}

// ListMessages returns messages of a bucket newer than afterID ("" = all),
// oldest first, capped at limit. Ordering relies on SQLite rowid so messages
// created within the same second keep insertion order.
func (s *Store) ListMessages(bucketID, afterID string, limit int) ([]*Message, error) {
	afterRowID, err := s.messageRowID(bucketID, afterID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT m.rowid, m.id, m.bucket_id, m.kind, m.content, m.filename, m.mime, m.size, m.storage_key, m.sender, m.pinned, m.created_at
		FROM messages m
		WHERE m.bucket_id = ? AND m.rowid > ?
		ORDER BY m.rowid LIMIT ?`, bucketID, afterRowID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

func (s *Store) messageRowID(bucketID, messageID string) (int64, error) {
	if messageID == "" {
		return 0, nil
	}
	var rowID int64
	err := s.db.QueryRow(`SELECT rowid FROM messages WHERE bucket_id = ? AND id = ?`, bucketID, messageID).Scan(&rowID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return rowID, err
}

func (s *Store) GetMessage(id string) (*Message, error) {
	rows, err := s.db.Query(`SELECT m.rowid, m.id, m.bucket_id, m.kind, m.content, m.filename, m.mime, m.size, m.storage_key, m.sender, m.pinned, m.created_at
		FROM messages m WHERE m.id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ms, err := scanMessages(rows)
	if err != nil {
		return nil, err
	}
	if len(ms) == 0 {
		return nil, nil
	}
	return ms[0], nil
}

func scanMessages(rows *sql.Rows) ([]*Message, error) {
	var out []*Message
	for rows.Next() {
		m := &Message{}
		var rowID int64
		var content, filename, mime, storageKey, sender sql.NullString
		var size sql.NullInt64
		var pinned int
		var ts int64
		if err := rows.Scan(&rowID, &m.ID, &m.BucketID, &m.Kind, &content, &filename, &mime, &size, &storageKey, &sender, &pinned, &ts); err != nil {
			return nil, err
		}
		m.Pinned = pinned != 0
		m.Content, m.Filename, m.Mime, m.StorageKey, m.Sender = content.String, filename.String, mime.String, storageKey.String, sender.String
		if size.Valid {
			m.Size = size.Int64
		}
		m.CreatedAt = time.Unix(ts, 0)
		out = append(out, m)
	}
	return out, rows.Err()
}

// PIN rate limiting, keyed by fingerprint cookie (not IP — all clients behind
// a reverse proxy share one IP).

func (s *Store) PINState(fp, bucketID string) (attempts int, lockedUntil time.Time, err error) {
	var until int64
	err = s.db.QueryRow(`SELECT attempts, locked_until FROM pin_attempts WHERE fp = ? AND bucket_id = ?`, fp, bucketID).
		Scan(&attempts, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	if until > 0 {
		lockedUntil = time.Unix(until, 0)
	}
	return attempts, lockedUntil, nil
}

func (s *Store) AddPINAttempt(fp, bucketID string, lockout time.Duration, maxAttempts int) (attempts int, locked bool, err error) {
	_, err = s.db.Exec(`INSERT INTO pin_attempts(fp, bucket_id, attempts, locked_until)
		VALUES(?,?,1,0)
		ON CONFLICT(fp, bucket_id) DO UPDATE SET attempts = attempts + 1`, fp, bucketID)
	if err != nil {
		return 0, false, err
	}
	attempts, _, err = s.PINState(fp, bucketID)
	if err != nil {
		return 0, false, err
	}
	if attempts >= maxAttempts {
		_, err = s.db.Exec(`UPDATE pin_attempts SET locked_until = ? WHERE fp = ? AND bucket_id = ?`,
			time.Now().Add(lockout).Unix(), fp, bucketID)
		if err != nil {
			return 0, false, err
		}
		return attempts, true, nil
	}
	return attempts, false, nil
}

func (s *Store) ResetPINAttempts(fp, bucketID string) error {
	_, err := s.db.Exec(`DELETE FROM pin_attempts WHERE fp = ? AND bucket_id = ?`, fp, bucketID)
	return err
}

// ListPinnedMessages returns the pinned messages of a bucket, oldest first.
func (s *Store) ListPinnedMessages(bucketID string) ([]*Message, error) {
	rows, err := s.db.Query(`SELECT m.rowid, m.id, m.bucket_id, m.kind, m.content, m.filename, m.mime, m.size, m.storage_key, m.sender, m.pinned, m.created_at
		FROM messages m
		WHERE m.bucket_id = ? AND m.pinned = 1
		ORDER BY m.rowid`, bucketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessages(rows)
}

// SetMessagePinned marks/unmarks a message as pinned within its bucket.
func (s *Store) SetMessagePinned(id string, pinned bool) error {
	v := 0
	if pinned {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE messages SET pinned = ? WHERE id = ?`, v, id)
	if err != nil {
		return err
	}
	return ensureAffected(res)
}

// DeleteMessage removes a message row and returns its storage key (if any)
// so the caller can clean up the file content.
func (s *Store) DeleteMessage(id string) (storageKey string, err error) {
	var key sql.NullString
	err = s.db.QueryRow(`DELETE FROM messages WHERE id = ? RETURNING storage_key`, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return key.String, nil
}

func ensureAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}
