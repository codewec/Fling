package handlers

import (
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// ShareMessage exposes a single message by its unguessable ID, without any
// bucket checks: knowing the link is the capability. Used by the public
// /s/<id> share page.
func (s *Server) ShareMessage(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, map[string]any{"message": toMessageJSON(m)})
}

// ShareFile serves the file content of a shared message. Same capability
// model as ShareMessage — no bucket PIN/private checks.
func (s *Server) ShareFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := s.Store.GetMessage(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database error")
		return
	}
	if m == nil || m.Kind != "file" || m.StorageKey == "" {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	f, err := s.Files.Open(r.Context(), m.StorageKey)
	if err != nil {
		writeError(w, http.StatusNotFound, "file content is missing")
		return
	}
	defer f.Close()

	mt := m.Mime
	if mt == "" {
		mt = "application/octet-stream"
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" || !isPreviewable(mt) {
		disposition = "attachment"
	}
	filename := m.Filename
	if filename == "" {
		filename = m.ID
	}
	w.Header().Set("Content-Type", mt)
	w.Header().Set("Content-Length", strconv.FormatInt(m.Size, 10))
	w.Header().Set("Content-Disposition", disposition+"; filename*=UTF-8''"+url.PathEscape(filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
}
