# Build stages run on the build platform (native speed, no QEMU emulation):
# Go cross-compiles via GOOS/GOARCH, the frontend output is platform-neutral.
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /build/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/.npmrc* ./
RUN corepack enable && corepack prepare pnpm@10.34.6 --activate && pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS backend
WORKDIR /build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /build/frontend/dist ./cmd/server/static
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/app ./cmd/server

FROM alpine:3
RUN adduser -D -u 10001 fling
RUN mkdir -p /data && chown fling:fling /data
USER fling
WORKDIR /app
COPY --from=backend /out/app ./fling
ENV FLING_ADDR=:8080 FLING_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["./fling"]
