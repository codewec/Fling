package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"filelit/internal/auth"
	"filelit/internal/config"
	"filelit/internal/handlers"
	filefs "filelit/internal/storage/fs"
	"filelit/internal/store"
)

var (
	version = "dev"
	commit  = "unknown"
)

//go:embed all:static
var staticFS embed.FS

func main() {
	log.Printf("FileLit %s (%s)", version, commit)

	cfg := config.Load()
	if strings.TrimSpace(cfg.SecretKey) == "change-me-filelit-dev-secret" {
		log.Printf("WARNING: using the default SECRET_KEY; set a long random value in production")
	}

	database, err := store.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer database.Close()

	fileStore, err := filefs.New(cfg.FilesDir)
	if err != nil {
		log.Fatalf("file storage: %v", err)
	}

	authSvc := auth.NewService(cfg)
	srv := &handlers.Server{Cfg: cfg, Store: database, Files: fileStore, AuthSvc: authSvc}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	// Note: no chi RealIP middleware — it trusts X-Forwarded-For blindly.
	// The auth layer consults X-Forwarded-For only when TRUST_PROXY=true.
	r.Use(chimw.Recoverer)
	r.Route("/api", func(r chi.Router) {
		r.Use(authSvc.Middleware(cfg))
		srv.Routes(r)
	})

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Println("no embedded static; serving API only (build with `make build`)")
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "frontend not embedded; build with make build", http.StatusNotFound)
		})
	} else {
		fileServer := http.FileServer(http.FS(static))
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			path := strings.TrimPrefix(req.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			if _, err := fs.Stat(static, path); err != nil {
				req.URL.Path = "/"
				fileServer.ServeHTTP(w, req)
				return
			}
			fileServer.ServeHTTP(w, req)
		})
	}

	log.Printf("listening on %s (data: %s, files: %s, auth: %v)",
		cfg.Addr, cfg.DataDir, cfg.FilesDir, cfg.AuthEnabled())
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Fatal(err)
	}
}
