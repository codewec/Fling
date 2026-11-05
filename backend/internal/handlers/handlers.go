// Package handlers exposes the Fling HTTP API.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"fling/internal/auth"
	"fling/internal/config"
	"fling/internal/storage"
	"fling/internal/store"
)

type Server struct {
	Cfg     *config.Config
	Store   *store.Store
	Files   storage.Store
	AuthSvc *auth.Service
}

// Routes registers the API endpoints on a chi router (the /api prefix and the
// auth middleware are applied by the caller).
func (s *Server) Routes(r chi.Router) {
	r.Get("/auth/me", s.AuthMe)
	r.Post("/auth/login", s.Login)
	r.Post("/auth/logout", s.Logout)

	r.Get("/buckets", s.ListBuckets)
	r.Post("/buckets", s.CreateBucket)
	r.Delete("/buckets/{id}", s.DeleteBucket)
	r.Post("/buckets/{id}/unlock", s.UnlockBucket)
	r.Post("/buckets/{id}/lock", s.LockBucket)
	r.Post("/buckets/{id}/pin", s.PinBucket)

	r.Get("/buckets/{id}/messages", s.ListMessages)
	r.Post("/buckets/{id}/messages", s.CreateMessage)
	r.Post("/buckets/{id}/files", s.UploadFile)

	r.Post("/messages/{id}/pin", s.PinMessage)
	r.Delete("/messages/{id}", s.DeleteMessage)

	r.Get("/files/{id}", s.DownloadFile)
}

type bucketJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HasPIN    bool   `json:"has_pin"`
	Pinned    bool   `json:"pinned"`
	Private   bool   `json:"private"`
	Owner     string `json:"owner,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

type messageJSON struct {
	ID        string `json:"id"`
	BucketID  string `json:"bucket_id"`
	Kind      string `json:"kind"`
	Content   string `json:"content,omitempty"`
	Filename  string `json:"filename,omitempty"`
	Mime      string `json:"mime,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Sender    string `json:"sender,omitempty"`
	Pinned    bool   `json:"pinned"`
	CreatedAt int64  `json:"created_at"`
}

func toBucketJSON(b *store.Bucket) bucketJSON {
	return bucketJSON{ID: b.ID, Name: b.Name, HasPIN: b.HasPIN, Pinned: b.Pinned, Private: b.Private, Owner: b.Owner, CreatedAt: b.CreatedAt.Unix()}
}

func toMessageJSON(m *store.Message) messageJSON {
	return messageJSON{
		ID:        m.ID,
		BucketID:  m.BucketID,
		Kind:      m.Kind,
		Content:   m.Content,
		Filename:  m.Filename,
		Mime:      m.Mime,
		Size:      m.Size,
		Sender:    m.Sender,
		Pinned:    m.Pinned,
		CreatedAt: m.CreatedAt.Unix(),
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return false
	}
	return true
}

// bucketOr404 loads the bucket from the chi URL param or writes an error.
func (s *Server) bucketOr404(w http.ResponseWriter, r *http.Request, id string) *store.Bucket {
	b, err := s.Store.GetBucket(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return nil
	}
	if b == nil {
		writeError(w, http.StatusNotFound, "bucket not found")
		return nil
	}
	return b
}

// requireUnlocked gates a bucket route behind its PIN.
func (s *Server) requireUnlocked(w http.ResponseWriter, r *http.Request, b *store.Bucket) bool {
	if !b.HasPIN || s.AuthSvc.CheckUnlock(r, b.ID) {
		return true
	}
	writeError(w, http.StatusForbidden, "pin required")
	return false
}

// currentUser is the identity asserted by the trusted reverse proxy ("" when
// header auth is disabled or the header is absent).
func (s *Server) currentUser(r *http.Request) string {
	return s.AuthSvc.HeaderUser(r, s.Cfg)
}

// requireAccess gates a bucket route behind ownership: private buckets are
// visible only to their owner.
func (s *Server) requireAccess(w http.ResponseWriter, r *http.Request, b *store.Bucket) bool {
	if !b.Private || b.Owner == s.currentUser(r) {
		return true
	}
	writeError(w, http.StatusForbidden, "access denied")
	return false
}

func nowUnix() int64 { return time.Now().Unix() }
