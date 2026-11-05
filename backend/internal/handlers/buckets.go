package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"fling/internal/auth"
)

type createBucketRequest struct {
	Name    string `json:"name"`
	PIN     string `json:"pin"`
	Private bool   `json:"private"`
}

func (s *Server) ListBuckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := s.Store.ListBuckets(s.currentUser(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]bucketJSON, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, toBucketJSON(b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": out})
}

func (s *Server) CreateBucket(w http.ResponseWriter, r *http.Request) {
	var req createBucketRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(name) > 128 {
		writeError(w, http.StatusBadRequest, "name is too long")
		return
	}
	pin := strings.TrimSpace(req.PIN)
	var pinHash, pinSalt string
	if pin != "" {
		if len(pin) != 4 {
			writeError(w, http.StatusBadRequest, "pin must be exactly 4 characters")
			return
		}
		salt, err := auth.NewSalt()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "cannot generate salt")
			return
		}
		pinSalt = salt
		pinHash = auth.HashPIN(pin, salt)
	}
	owner := s.currentUser(r)
	if req.Private && owner == "" {
		writeError(w, http.StatusBadRequest, "private buckets require an authenticated user")
		return
	}
	b, err := s.Store.CreateBucket(name, pinHash, pinSalt, owner, req.Private)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if b.HasPIN {
		s.AuthSvc.AddUnlock(w, r, b.ID)
	}
	writeJSON(w, http.StatusCreated, toBucketJSON(b))
}

func (s *Server) DeleteBucket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b := s.bucketOr404(w, r, id)
	if b == nil {
		return
	}
	if !s.requireAccess(w, r, b) {
		return
	}
	if !s.requireUnlocked(w, r, b) {
		return
	}
	keys, err := s.Store.FileKeysByBucket(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if err := s.Store.DeleteBucket(id); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	for _, k := range keys {
		_ = s.Files.Delete(r.Context(), k)
	}
	s.AuthSvc.RemoveUnlock(w, r, id)
	w.WriteHeader(http.StatusNoContent)
}

type unlockRequest struct {
	PIN string `json:"pin"`
}

func (s *Server) UnlockBucket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b := s.bucketOr404(w, r, id)
	if b == nil {
		return
	}
	if !s.requireAccess(w, r, b) {
		return
	}
	if !b.HasPIN {
		writeError(w, http.StatusBadRequest, "bucket is not protected by a pin")
		return
	}

	fp := s.AuthSvc.Fingerprint(w, r)
	_, lockedUntil, err := s.Store.PINState(fp, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if time.Now().Before(lockedUntil) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":       "too many attempts, try again later",
			"retry_after": int64(time.Until(lockedUntil).Seconds()) + 1,
		})
		return
	}

	var req unlockRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	hash, salt, err := s.Store.BucketPIN(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if auth.HashPIN(strings.TrimSpace(req.PIN), salt) != hash {
		lockout := time.Duration(s.Cfg.PINLockoutMinutes) * time.Minute
		attempts, locked, err := s.Store.AddPINAttempt(fp, id, lockout, s.Cfg.PINMaxAttempts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if locked {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error":       "too many attempts, try again later",
				"retry_after": int64(lockout.Seconds()),
			})
			return
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error":              "wrong pin",
			"attempts_remaining": s.Cfg.PINMaxAttempts - attempts,
		})
		return
	}

	if err := s.Store.ResetPINAttempts(fp, id); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	s.AuthSvc.AddUnlock(w, r, id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) LockBucket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b := s.bucketOr404(w, r, id)
	if b == nil {
		return
	}
	if !s.requireAccess(w, r, b) {
		return
	}
	s.AuthSvc.RemoveUnlock(w, r, id)
	w.WriteHeader(http.StatusNoContent)
}

type pinBucketRequest struct {
	Pinned bool `json:"pinned"`
}

func (s *Server) PinBucket(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	b := s.bucketOr404(w, r, id)
	if b == nil {
		return
	}
	if !s.requireAccess(w, r, b) {
		return
	}
	var req pinBucketRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.Store.SetBucketPinned(id, req.Pinned); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
