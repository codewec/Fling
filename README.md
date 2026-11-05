<p align="center">
  <img src="logo.svg" alt="FileLit logo" width="96" height="96">
</p>

<h1 align="center">FileLit</h1>

Self-hosted file exchange service for your local network, styled as a modern messenger. Move text and files between computers over LAN through a Telegram-like web UI — no accounts, no cloud, single binary.

> **What it is:** FileLit is not a messenger. It is a quick, temporary way to shuttle files and text between devices on the same network, wrapped in a familiar chat interface. It is **not** a secure storage service — don't keep anything there that you can't afford to lose or expose.

![Stack](https://img.shields.io/badge/stack-Go%20%2B%20Svelte-0ea5e9) ![CGO](https://img.shields.io/badge/CGO-free-10b981)

## Features

- **Buckets** — create a shared space (we call it a *bucket*) and drop messages or files into it. Anyone on the network can open the list of buckets; a bucket protected by a 4-character PIN only opens after the PIN is entered (with per-client rate limiting and lockout).
- **Files** — upload anything; images, PDFs and text files are previewable in the browser, everything else downloads with one click. Upload progress is shown live. Buckets support drag & drop.
- **Pinning** — pin buckets to the top of the list and pin messages inside a bucket (click a pinned message in the header to jump to it).
- **Messages** — right-click context menus for bucket and message actions (pin, delete, copy link). Pinned messages show in a bar above the chat.
- **Reverse-proxy auth (optional)** — put [Tinyauth](https://github.com/tinyauthapp/tinyauth) (or any forward-auth proxy) in front and FileLit will trust the `Remote-User` identity header. Authenticated users can create **private buckets** visible only to them; everyone else works anonymously as before.
- **Single binary** — the entire web UI is embedded into one Go binary. SQLite (pure-Go, no CGO) for metadata, plain files on disk for content.

## Quick start (Docker)

```bash
docker build -t filelit .
docker run -d --name filelit -p 8080:8080 -v filelit-data:/data filelit
```

Open http://localhost:8080 — that's it. No users, no passwords, fully open on the local network.

`docker-compose.yml`:

```yaml
services:
  filelit:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - filelit-data:/data
    environment:
      # see table below; everything is optional
      - MAX_UPLOAD_MB=512

volumes:
  filelit-data:
```

## Configuration

All configuration is via environment variables (a local `.env` file is picked up automatically).

| Variable | Default | Description |
|---|---|---|
| `ADDR` | `:8080` | Listen address. |
| `DATA_DIR` | `./data` | Directory for the SQLite database and uploaded files. |
| `FILES_DIR` | `$DATA_DIR/files` | Where file content is stored. |
| `DATABASE_URL` | `$DATA_DIR/filelit.db` | SQLite database path. |
| `SECRET_KEY` | dev value | HMAC secret for signed cookies. **Change in any real deployment.** |
| `MAX_UPLOAD_MB` | `512` | Per-file upload size limit. |
| `PIN_MAX_ATTEMPTS` | `5` | Wrong PIN attempts before lockout. |
| `PIN_LOCKOUT_MINUTES` | `15` | Lockout duration after too many wrong PINs. |

### Access control

By default the app is fully open — that is the intended LAN mode. Two optional layers exist for the "available from outside" case; they compose freely.

**1. Shared password** — challenge with a single password for everyone:

```bash
FILELIT_PASSWORD=s3cret ./app
```

Requests from trusted networks/hosts skip the challenge (handy for "internal domain in LAN, password from the internet"):

```bash
TRUSTED_NETWORKS=192.168.0.0/16,10.0.0.0/8   # default already covers RFC1918 + localhost
TRUSTED_HOSTS=files.lan                       # hostnames that skip auth
TRUST_PROXY=true                              # behind a reverse proxy: trust X-Forwarded-For
```

**2. Forward-auth identity header (Tinyauth)** — instead of a FileLit password, let a reverse proxy authenticate users and pass the identity through a header:

```bash
AUTH_USER_HEADER=Remote-User ./app
```

Only set this when **every** request really passes through a proxy that strips client-supplied copies of the header (Tinyauth's forward-auth does exactly that) — otherwise anyone can spoof an identity.

With `AUTH_USER_HEADER` set, requests carrying the header are treated as that user; requests without it are anonymous. Authenticated users get one extra feature: **private buckets** (a toggle in the "New bucket" dialog) that are hidden from everyone else at the SQL level and return 403 for non-owners on every endpoint. Public buckets behave exactly as before.

### Tinyauth example (Caddy)

```caddyfile
files.example.com {
	forward_auth tinyauth:3000 {
		uri /api/auth/caddy
		copy_headers Remote-User Remote-Name Remote-Email Remote-Groups
	}
	reverse_proxy filelit:8080
}
```

```yaml
# docker-compose.yml fragment
services:
  filelit:
    build: .
    environment:
      - AUTH_USER_HEADER=Remote-User
  tinyauth:
    image: ghcr.io/tinyauthapp/tinyauth:latest
    # ... see https://tinyauth.app/docs/
```

The same mechanism works with any forward-auth gateway that sets a user header (Authelia, Authentik, Pocket ID, …) — point `AUTH_USER_HEADER` at whatever header your proxy sends.

## Architecture

```
┌────────────┐   HTTP    ┌──────────────────────────────────────┐
│  Svelte 5  │ ────────▶ │  Go server (single binary)           │
│  + Tailwind│   /api/*  │   handlers (chi)                     │
│  + shadcn- │           │     ├── auth: guards, signed cookies │
│  svelte    │           │     ├── store: SQLite (metadata)     │
└────────────┘           │     └── storage: files on disk       │
                         └──────────────────────────────────────┘
```

- **Frontend** — Svelte 5 (runes) + Vite + Tailwind 4 + [shadcn-svelte](https://www.shadcn-svelte.com/) (chat UI built on its `message-scroller`/`message`/`bubble` AI elements), compiled to static files embedded into the binary via `go:embed`.
- **Backend** — Go 1.27, chi router, `modernc.org/sqlite` (pure Go, CGO-free, WAL mode).
- **Storage abstraction** — file content goes through a `storage.Store` interface (`Save`/`Open`/`Delete`). The default implementation writes to the local filesystem (`FILES_DIR`); an S3 backend can be plugged in later without touching the API or handlers.
- **No user entities** — identity, when present, is just the trusted `Remote-User` header value per request. Nothing to register, nothing to manage.

### Database migrations

Schema is created with a single idempotent DDL block (`CREATE TABLE IF NOT EXISTS …`), so a fresh database is fully initialized on first start and re-running the migration is a no-op.

## Building from source

Requires Go 1.27+ and pnpm.

```bash
make build   # builds frontend, embeds it, produces ./app
make run     # build + run
make test    # go test ./...
```

### Seed demo data

To populate a database with demo buckets and messages (useful during development or first deployment):

```bash
go run ./cmd/seed -db ./data/filelit.db -files ./data/files
```

Creates a public "General" bucket with a conversation, a pinned message and a demo file, a pinned "Quick transfer" bucket, a PIN-protected bucket (PIN `1234`) and a private bucket owned by `-owner` (default `alice`). Seed only into an empty/test database — it always inserts new rows.

For development run backend and frontend separately (`make dev-backend` / `make dev-frontend` — the Vite dev server proxies `/api` to the backend).

## Notes

- **Ephemeral by design.** FileLit is built for fast, temporary transfers inside a trusted LAN, not for archiving. There are no backups, no encryption at rest, and deletion is permanent — treat it as a clipboard between devices, not a vault.
- PIN codes are stored salted and hashed (SHA-256); rate limiting is keyed by a fingerprint cookie, not by IP, so all clients behind one NAT/proxy are limited independently.
- Message order follows SQLite `rowid`, so messages created within the same second keep insertion order.
- HEIC/HEIF files are treated as regular files since browsers generally cannot render them.
