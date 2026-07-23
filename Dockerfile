# Multi-stage build for Atlas-Go

# Stage 1: Frontend build
FROM node:22-alpine AS nodebuilder
WORKDIR /build
# Shared static assets used by admin_web and client_web builds.
COPY shared_web/ ./shared_web/

# Install dependencies for both embedded frontends.
COPY admin_web/package.json admin_web/package-lock.json ./admin_web/
COPY client_web/package.json client_web/package-lock.json ./client_web/
RUN cd admin_web && npm ci
RUN cd client_web && npm ci

# Copy build configs and static assets, then build each dist directory.
COPY admin_web/esbuild.config.mjs ./admin_web/
COPY client_web/esbuild.config.mjs ./client_web/
COPY admin_web/static ./admin_web/static
COPY client_web/static ./client_web/static
RUN cd admin_web && npm run build
RUN cd client_web && npm run build

# Stage 2: Go build
FROM golang:1.26.4-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .
COPY --from=nodebuilder /build/admin_web/dist ./admin_web/dist
COPY --from=nodebuilder /build/client_web/dist ./client_web/dist

ARG TARGETARCH
ARG VERSION
ARG BUILDTIME
ARG GIT_COMMIT
RUN test -n "${GIT_COMMIT:-}" && test "${GIT_COMMIT}" != unknown || \
    (echo "GIT_COMMIT must be set to a non-unknown source commit" >&2 && exit 1)

# VERSION/BUILDTIME/GIT_COMMIT are injected via build-args (CI or docker
# compose) so the binary embeds meaningful metadata even though .git is
# excluded from the Docker build context. GIT_COMMIT is required and cannot
# use the "unknown" sentinel because the freshness gate audits it.
#
# The internal/buildinfo.* ldflags are the canonical runtime-parity source;
# main.version / main.buildTime are kept for backwards compatibility with
# existing consumers (do not remove).
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev} -X main.buildTime=${BUILDTIME:-$(date -u +%Y%m%d%H%M%S)} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Version=${VERSION:-dev} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Commit=${GIT_COMMIT:-unknown} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.BuildTime=${BUILDTIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
    -o atlas-go \
    ./cmd/atlas
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev} -X main.buildTime=${BUILDTIME:-$(date -u +%Y%m%d%H%M%S)} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Version=${VERSION:-dev} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Commit=${GIT_COMMIT:-unknown} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.BuildTime=${BUILDTIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
    -o daily-replay-sync ./cmd/daily-replay-sync
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X github.com/kaecer68/atlas-go/internal/buildinfo.Version=${VERSION:-dev} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Commit=${GIT_COMMIT} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.BuildTime=${BUILDTIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
    -o backfill-replay ./cmd/backfill-replay
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev} -X main.buildTime=${BUILDTIME:-$(date -u +%Y%m%d%H%M%S)} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Version=${VERSION:-dev} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Commit=${GIT_COMMIT:-unknown} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.BuildTime=${BUILDTIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
    -o atlas-mcp ./cmd/atlas-mcp
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev} -X main.buildTime=${BUILDTIME:-$(date -u +%Y%m%d%H%M%S)} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Version=${VERSION:-dev} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.Commit=${GIT_COMMIT:-unknown} \
    -X github.com/kaecer68/atlas-go/internal/buildinfo.BuildTime=${BUILDTIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" \
    -o calibrate-seasonal ./cmd/calibrate-seasonal

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl

# Create non-root user
RUN addgroup -g 1000 atlas && \
    adduser -u 1000 -G atlas -s /bin/sh -D atlas

# Set working directory
WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/atlas-go /app/
COPY --from=builder /build/daily-replay-sync /app/
COPY --from=builder /build/backfill-replay /app/
COPY --from=builder /build/atlas-mcp /app/
COPY --from=builder /build/calibrate-seasonal /app/

# Copy configuration files
COPY --from=builder /build/configs /app/configs
COPY --from=builder /build/prompts /app/prompts
COPY --from=builder /build/scripts /app/scripts
COPY --from=builder /build/sql /app/sql

# Create necessary directories
RUN mkdir -p /app/data /app/reports /app/logs && \
    chown -R atlas:atlas /app

# Switch to non-root user
USER atlas

# Expose port
EXPOSE 18080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:18080/health || exit 1

# Run the application
ENTRYPOINT ["/app/atlas-go"]
CMD ["-api"]
