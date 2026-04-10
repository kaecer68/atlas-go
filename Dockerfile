# Multi-stage build for Atlas-Go
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always) -X main.buildTime=$(date -u +%Y%m%d%H%M%S)" \
    -o atlas-go \
    ./cmd/atlas

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl

# Create non-root user
RUN addgroup -g 1000 atlas && \
    adduser -u 1000 -G atlas -s /bin/sh -D atlas

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/atlas-go /app/

# Copy configuration files
COPY --from=builder /build/configs /app/configs
COPY --from=builder /build/prompts /app/prompts
COPY --from=builder /build/scripts /app/scripts

# Create necessary directories
RUN mkdir -p /app/data /app/reports /app/logs && \
    chown -R atlas:atlas /app

# Switch to non-root user
USER atlas

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run the application
ENTRYPOINT ["/app/atlas-go"]
CMD ["serve"]
