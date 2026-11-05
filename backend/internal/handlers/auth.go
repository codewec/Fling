package handlers

import (
	"crypto/subtle"
	"net/http"
)

func (s *Server) AuthMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"auth_required": s.Cfg.AuthEnabled() && !s.AuthSvc.ValidSession(r),
		"authenticated": s.AuthSvc.ValidSession(r) || !s.Cfg.AuthEnabled(),
		"trusted":       s.AuthSvc.Trusted(r, s.Cfg),
		"oidc_enabled":  false, // reserved for external-access OIDC
		"user_auth":     s.Cfg.AuthUserHeader != "",
		"user":          s.currentUser(r),
	})
}

type loginRequest struct {
	Password string `json:"password"`
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(s.Cfg.FileLitPassword)) != 1 {
		writeError(w, http.StatusUnauthorized, "wrong password")
		return
	}
	s.AuthSvc.CreateSession(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	s.AuthSvc.ClearSession(w)
	w.WriteHeader(http.StatusNoContent)
}
