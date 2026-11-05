// Package auth implements the access model: no user entities exist. Access is
// granted either because the request comes from a trusted network/host, or via
// a shared password session. OIDC is planned as an additional Guard
// implementation for external access and is not wired up in the MVP.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"fling/internal/config"
)

// Guard decides whether a request may proceed without a login challenge.
// Implementations: trusted-networks/shared-password now, OIDC later.
type Guard interface {
	Allow(r *http.Request) bool
}

// AllowAll grants every request (auth disabled).
type AllowAll struct{}

func (AllowAll) Allow(*http.Request) bool { return true }

// TrustedNetworksGuard allows requests from configured CIDRs/hosts without a
// session. It trusts X-Forwarded-For only when config.TrustProxy is set.
type TrustedNetworksGuard struct {
	Cfg *config.Config
}

func (g TrustedNetworksGuard) Allow(r *http.Request) bool {
	remote := r.RemoteAddr
	if g.Cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			remote = strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	return g.Cfg.IsTrusted(remote, r.Host)
}

// PasswordSessionGuard allows requests carrying a valid shared-password
// session cookie.
type PasswordSessionGuard struct {
	Svc *Service
}

func (g PasswordSessionGuard) Allow(r *http.Request) bool {
	return g.Svc.ValidSession(r)
}

// AnyGuard allows a request when at least one guard allows it.
type AnyGuard struct {
	Guards []Guard
}

func (g AnyGuard) Allow(r *http.Request) bool {
	for _, guard := range g.Guards {
		if guard.Allow(r) {
			return true
		}
	}
	return false
}

type Service struct {
	secret []byte
}

const (
	cookieSession = "fling_session"
	cookieUnlock  = "fling_unlock"
	cookieFP      = "fling_fp"

	sessionTTL = 30 * 24 * time.Hour
)

func NewService(cfg *config.Config) *Service {
	return &Service{secret: []byte(cfg.SecretKey)}
}

func (s *Service) Guard(cfg *config.Config) Guard {
	// With no password configured the app is fully open.
	if !cfg.AuthEnabled() {
		return AllowAll{}
	}
	return AnyGuard{Guards: []Guard{
		TrustedNetworksGuard{Cfg: cfg},
		PasswordSessionGuard{Svc: s},
	}}
}

// HeaderUser returns the identity asserted by the trusted reverse proxy, or ""
// when header auth is not configured or the header is absent (anonymous).
func (s *Service) HeaderUser(r *http.Request, cfg *config.Config) string {
	if cfg.AuthUserHeader == "" {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(cfg.AuthUserHeader))
}

// Trusted reports whether the request comes from a trusted network/host.
func (s *Service) Trusted(r *http.Request, cfg *config.Config) bool {
	return TrustedNetworksGuard{Cfg: cfg}.Allow(r)
}

// Middleware rejects untrusted unauthenticated API requests with 401. The
// auth handshake endpoints stay public so clients can log in and query the
// auth mode.
func (s *Service) Middleware(cfg *config.Config) func(http.Handler) http.Handler {
	guard := s.Guard(cfg)
	public := map[string]bool{
		"/api/auth/me":     true,
		"/api/auth/login":  true,
		"/api/auth/logout": true,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if guard.Allow(r) || public[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", "FlingPassword")
			http.Error(w, `{"error":"authentication required"}`, http.StatusUnauthorized)
		})
	}
}

func (s *Service) ValidSession(r *http.Request) bool {
	return s.valid(cookieSession, r) != ""
}

func (s *Service) CreateSession(w http.ResponseWriter) {
	s.set(w, cookieSession, "1", sessionTTL)
}

func (s *Service) ClearSession(w http.ResponseWriter) {
	s.clear(w, cookieSession)
}

// CheckUnlock reports whether the requester unlocked the given bucket.
func (s *Service) CheckUnlock(r *http.Request, bucketID string) bool {
	v := s.valid(cookieUnlock, r)
	if v == "" {
		return false
	}
	for _, id := range strings.Split(v, ",") {
		if id == bucketID {
			return true
		}
	}
	return false
}

func (s *Service) AddUnlock(w http.ResponseWriter, r *http.Request, bucketID string) {
	v := s.valid(cookieUnlock, r)
	ids := map[string]bool{bucketID: true}
	if v != "" {
		for _, id := range strings.Split(v, ",") {
			ids[id] = true
		}
	}
	list := make([]string, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	s.set(w, cookieUnlock, strings.Join(list, ","), sessionTTL)
}

func (s *Service) RemoveUnlock(w http.ResponseWriter, r *http.Request, bucketID string) {
	v := s.valid(cookieUnlock, r)
	var list []string
	for _, id := range strings.Split(v, ",") {
		if id != "" && id != bucketID {
			list = append(list, id)
		}
	}
	if len(list) == 0 {
		s.clear(w, cookieUnlock)
		return
	}
	s.set(w, cookieUnlock, strings.Join(list, ","), sessionTTL)
}

// Fingerprint returns the client's rate-limit identity, issuing a new
// long-lived cookie on first contact.
func (s *Service) Fingerprint(w http.ResponseWriter, r *http.Request) string {
	if v := s.valid(cookieFP, r); v != "" {
		return v
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	fp := hex.EncodeToString(buf)
	s.set(w, cookieFP, fp, 365*24*time.Hour)
	return fp
}

func (s *Service) set(w http.ResponseWriter, name, value string, ttl time.Duration) {
	v := s.sign(value)
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    v,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(ttl),
	})
}

func (s *Service) clear(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// sign produces "<base64 value>.<hmac>"; valid verifies and decodes it.
func (s *Service) sign(value string) string {
	raw := base64.RawURLEncoding.EncodeToString([]byte(value))
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(raw))
	return raw + "." + hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) valid(name string, r *http.Request) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	raw, sig, ok := strings.Cut(c.Value, ".")
	if !ok || raw == "" {
		return ""
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(raw))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return ""
	}
	return string(decoded)
}

// HashPIN hashes a 4-character PIN with a per-bucket salt.
func HashPIN(pin, salt string) string {
	h := sha256.Sum256([]byte(salt + ":" + pin))
	return hex.EncodeToString(h[:])
}

// NewSalt returns a random hex salt for PIN hashing.
func NewSalt() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
