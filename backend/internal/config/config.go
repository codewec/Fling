package config

import (
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Addr        string
	DataDir     string
	DatabaseURL string
	FilesDir    string

	SecretKey string

	// FlingPassword enables shared-password auth for requests that are not
	// trusted. When empty, the app is fully open.
	FlingPassword string

	// TrustedNetworks is a list of CIDRs; requests from these IPs skip auth.
	TrustedNetworks []netip.Prefix
	// TrustedHosts is a list of hostnames; requests with these Host headers
	// skip auth (e.g. the internal LAN domain published only inside the network).
	TrustedHosts []string
	// TrustProxy makes the trusted-network check use X-Forwarded-For instead of
	// the direct peer address (set when Fling runs behind a reverse proxy).
	TrustProxy bool

	// AuthUserHeader names a request header carrying the authenticated user's
	// identity (e.g. "Remote-User" when Tinyauth or a similar forward-auth
	// proxy guards the app). Empty = header auth disabled, everyone is
	// anonymous. Only set this when every request really passes through a
	// proxy that strips client-supplied copies of the header, otherwise anyone
	// can spoof an identity.
	AuthUserHeader string

	MaxUploadBytes int64

	PINMaxAttempts    int
	PINLockoutMinutes int

	// OIDC is reserved for a future external-access auth method and is not
	// wired up in the MVP.
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
}

func Load() *Config {
	_ = godotenv.Load()

	dataDir := env("FLING_DATA_DIR", "./data")
	_ = os.MkdirAll(dataDir, 0o755)
	filesDir := env("FLING_FILES_DIR", filepath.Join(dataDir, "files"))
	_ = os.MkdirAll(filesDir, 0o755)

	return &Config{
		Addr:              env("FLING_ADDR", ":8080"),
		DataDir:           dataDir,
		DatabaseURL:       env("FLING_DATABASE_URL", filepath.Join(dataDir, "fling.db")),
		FilesDir:          filesDir,
		SecretKey:         env("FLING_SECRET_KEY", "change-me-fling-dev-secret"),
		FlingPassword:     os.Getenv("FLING_PASSWORD"),
		TrustedNetworks:   parseCIDRs(env("FLING_TRUSTED_NETWORKS", "127.0.0.0/8,::1/128,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12")),
		TrustedHosts:      parseList(env("FLING_TRUSTED_HOSTS", "")),
		TrustProxy:        envBool("FLING_TRUST_PROXY", false),
		AuthUserHeader:    os.Getenv("FLING_AUTH_USER_HEADER"),
		MaxUploadBytes:    envInt64("FLING_MAX_UPLOAD_MB", 512) * 1024 * 1024,
		PINMaxAttempts:    envInt("FLING_PIN_MAX_ATTEMPTS", 5),
		PINLockoutMinutes: envInt("FLING_PIN_LOCKOUT_MINUTES", 15),
		OIDCIssuer:        os.Getenv("FLING_OIDC_ISSUER"),
		OIDCClientID:      os.Getenv("FLING_OIDC_CLIENT_ID"),
		OIDCClientSecret:  os.Getenv("FLING_OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:   os.Getenv("FLING_OIDC_REDIRECT_URL"),
	}
}

func (c *Config) AuthEnabled() bool {
	return c.FlingPassword != ""
}

// IsTrusted reports whether a request from the given remote address/host
// should skip shared-password auth.
func (c *Config) IsTrusted(remoteAddr, host string) bool {
	host = strings.Split(host, ":")[0]
	for _, h := range c.TrustedHosts {
		if strings.EqualFold(h, host) {
			return true
		}
	}
	ipStr := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		ipStr = h
	}
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}
	for _, n := range c.TrustedNetworks {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func parseCIDRs(s string) []netip.Prefix {
	var out []netip.Prefix
	for _, part := range parseList(s) {
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func parseList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
