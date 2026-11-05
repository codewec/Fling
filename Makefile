.PHONY: frontend backend build run dev-backend dev-frontend test clean

BUILD_VERSION ?= dev
BUILD_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf unknown)

DATA_DIR ?= ./data
export DATA_DIR

frontend:
	cd frontend && pnpm install --frozen-lockfile && pnpm run build
	rm -rf backend/cmd/server/static
	mkdir -p backend/cmd/server/static
	cp -r frontend/dist/* backend/cmd/server/static/

backend:
	cd backend && go build -ldflags="-X main.version=$(BUILD_VERSION) -X main.commit=$(BUILD_COMMIT)" -o ../app ./cmd/server

build: frontend backend

run: build
	./app

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && pnpm run dev

test:
	cd backend && go test ./...

clean:
	rm -rf frontend/dist app data
	mkdir -p backend/cmd/server/static
	touch backend/cmd/server/static/.gitkeep
