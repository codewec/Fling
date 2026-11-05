package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"filelit/internal/store"
)

const (
	maxTextMessageLen = 64 * 1024
	defaultMsgLimit   = 200
	maxMsgLimit       = 1000
)

func (s *Server) ListMessages(w http.ResponseWriter, r *http.Request) {
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
	limit := defaultMsgLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = min(n, maxMsgLimit)
		}
	}
	msgs, err := s.Store.ListMessages(id, r.URL.Query().Get("after_id"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	pinned, err := s.Store.ListPinnedMessages(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]messageJSON, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessageJSON(m))
	}
	pinnedOut := make([]messageJSON, 0, len(pinned))
	for _, m := range pinned {
		pinnedOut = append(pinnedOut, toMessageJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": out, "pinned_messages": pinnedOut})
}

type createMessageRequest struct {
	Content string `json:"content"`
	Sender  string `json:"sender"`
}

func (s *Server) CreateMessage(w http.ResponseWriter, r *http.Request) {
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
	var req createMessageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if len(content) > maxTextMessageLen {
		writeError(w, http.StatusBadRequest, "message is too long")
		return
	}
	m := &store.Message{BucketID: id, Kind: "text", Content: content, Sender: strings.TrimSpace(req.Sender)}
	if err := s.Store.AddMessage(m); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	writeJSON(w, http.StatusCreated, toMessageJSON(m))
}

type pinMessageRequest struct {
	Pinned bool `json:"pinned"`
}

func (s *Server) PinMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := s.Store.GetMessage(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	b := s.bucketOr404(w, r, m.BucketID)
	if b == nil {
		return
	}
	if !s.requireAccess(w, r, b) {
		return
	}
	if !s.requireUnlocked(w, r, b) {
		return
	}
	var req pinMessageRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := s.Store.SetMessagePinned(id, req.Pinned); err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := s.Store.GetMessage(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if m == nil {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	b := s.bucketOr404(w, r, m.BucketID)
	if b == nil {
		return
	}
	if !s.requireAccess(w, r, b) {
		return
	}
	if !s.requireUnlocked(w, r, b) {
		return
	}
	key, err := s.Store.DeleteMessage(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if key != "" {
		_ = s.Files.Delete(r.Context(), key)
	}
	w.WriteHeader(http.StatusNoContent)
}
