package handlers

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"filelit/internal/store"
)

// UploadFile accepts one file via multipart/form-data (field "file",
// optional field "sender") and streams it to the file store without buffering
// the whole body in memory.
func (s *Server) UploadFile(w http.ResponseWriter, r *http.Request) {
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

	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "expected multipart/form-data")
		return
	}

	var sender string
	var saved *store.Message
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "malformed multipart body")
			return
		}
		switch part.FormName() {
		case "sender":
			buf, _ := io.ReadAll(io.LimitReader(part, 256))
			sender = strings.TrimSpace(string(buf))
		case "file":
			if saved != nil {
				io.Copy(io.Discard, part)
				continue
			}
			saved, err = s.saveFilePart(w, r, id, part, sender)
			if err != nil {
				return
			}
		default:
			io.Copy(io.Discard, part)
		}
		part.Close()
	}
	if saved == nil {
		writeError(w, http.StatusBadRequest, "file field is required")
		return
	}
	writeJSON(w, http.StatusCreated, toMessageJSON(saved))
}

func (s *Server) saveFilePart(w http.ResponseWriter, r *http.Request, bucketID string, part *multipart.Part, sender string) (*store.Message, error) {
	name := part.FileName()
	if name == "" {
		name = "file"
	}
	key := bucketID + "/" + store.NewID()
	body := http.MaxBytesReader(w, part, s.Cfg.MaxUploadBytes)
	info, err := s.Files.Save(r.Context(), key, body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds the maximum upload size")
			return nil, err
		}
		writeError(w, http.StatusInternalServerError, "failed to store file")
		return nil, err
	}
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if mt == "" {
		mt = "application/octet-stream"
	}
	m := &store.Message{
		BucketID:   bucketID,
		Kind:       "file",
		Filename:   filepath.Base(name),
		Mime:       mt,
		Size:       info.Size,
		StorageKey: key,
		Sender:     sender,
	}
	if err := s.Store.AddMessage(m); err != nil {
		_ = s.Files.Delete(r.Context(), key)
		writeError(w, http.StatusInternalServerError, "database error")
		return nil, err
	}
	return m, nil
}

// DownloadFile serves a file message. ?inline=1 allows in-browser preview of
// safe types (text, images, audio, video); otherwise Content-Disposition is
// attachment.
func (s *Server) DownloadFile(w http.ResponseWriter, r *http.Request) {
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
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" && isPreviewable(mt) {
		disposition = "inline"
	}
	filename := m.Filename
	if filename == "" {
		filename = m.ID
	}
	w.Header().Set("Content-Type", mt)
	w.Header().Set("Content-Length", strconvInt64(m.Size))
	w.Header().Set("Content-Disposition", disposition+"; filename*=UTF-8''"+url.PathEscape(filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
}

func isPreviewable(mt string) bool {
	return strings.HasPrefix(mt, "text/") ||
		strings.HasPrefix(mt, "image/") ||
		strings.HasPrefix(mt, "audio/") ||
		strings.HasPrefix(mt, "video/") ||
		isTextLike(mt) ||
		mt == "application/pdf"
}

// isTextLike matches structured text formats whose MIME type is not text/*
// but which are safe and meaningful to display inline (JSON, YAML, …).
func isTextLike(mt string) bool {
	switch mt {
	case "application/json",
		"application/ld+json",
		"application/xml",
		"application/xhtml+xml",
		"application/yaml",
		"application/x-yaml",
		"text/yaml",
		"application/toml",
		"application/javascript",
		"application/x-javascript",
		"application/x-sh",
		"application/sql",
		"image/svg+xml":
		return true
	}
	return strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "+xml")
}

func strconvInt64(n int64) string {
	return strconv.FormatInt(n, 10)
}
