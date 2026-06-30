package server

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config is the immutable configuration for a server. Build via New, never mutate
// after construction.
type Config struct {
	AtlasBaseURL       string        // atlas HTTP base, e.g. http://127.0.0.1:8080
	APIToken           string        // admin API key forwarded to atlas HTTP API (ATLAS_API_KEY)
	AuditLogPath       string        // JSONL audit log file path
	HTTPTimeout        time.Duration // per-call timeout to atlas HTTP (default 10s)
	AuditRetentionDays int           // 0 = disabled; >0 = remove entries older than N days (default 30 in main.go)
	RateLimitPerMinute int           // per-(tool, caller) requests per minute; 0 = disabled
	RateLimitBurst     int           // burst capacity; 0 = defaults to PerMinute
}

// Run constructs a server with config, registers the five core tools and
// runs the stdio transport to completion. Returns the first transport error.
//
// If cfg.AuditRetentionDays > 0, Run also (a) prunes the existing audit log
// on startup and (b) starts a background goroutine that prunes daily. A
// failed prune is logged to stderr but does NOT abort startup — stale audit
// data is preferable to no MCP server at all.
func Run(ctx context.Context, cfg Config) error {
	if cfg.AtlasBaseURL == "" {
		return fmt.Errorf("server: AtlasBaseURL is required")
	}
	if cfg.AuditLogPath == "" {
		return fmt.Errorf("server: AuditLogPath is required")
	}
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 10 * time.Second
	}

	audit, err := NewAuditWriter(cfg.AuditLogPath)
	if err != nil {
		return fmt.Errorf("server: open audit log: %w", err)
	}

	if cfg.AuditRetentionDays > 0 {
		// Prune once at startup. Failure is non-fatal.
		if removed, cErr := audit.Cleanup(cfg.AuditRetentionDays, time.Now()); cErr != nil {
			fmt.Fprintf(os.Stderr, "atlas-mcp: startup audit cleanup failed: %v\n", cErr)
		} else if removed > 0 {
			fmt.Fprintf(os.Stderr, "atlas-mcp: startup audit cleanup removed %d entries\n", removed)
		}
		// Schedule daily cleanup until ctx is cancelled.
		go runRetentionLoop(ctx, audit, cfg.AuditRetentionDays)
	}

	// Rate limiter: token-bucket per (tool, caller). Phase 3 B — closes
	// roadmap §4 risk "agent triggers destructive operations". Disabled when
	// RateLimitPerMinute == 0 (no-op, all calls allowed). Background sweeper
	// evicts idle buckets to bound memory.
	limiter := NewRateLimiter(RateLimiterConfig{
		PerMinute: cfg.RateLimitPerMinute,
		Burst:     cfg.RateLimitBurst,
	})
	limiter.Run(ctx)

	srv := &server{
		cfg:     cfg,
		audit:   audit,
		cli:     newHTTPClient(cfg),
		limiter: limiter,
	}

	impl := &mcp.Implementation{Name: "atlas-mcp", Version: "v0.1.0"}
	mcpSrv := mcp.NewServer(impl, &mcp.ServerOptions{})

	registerTools(mcpSrv, srv)

	return mcpSrv.Run(ctx, &mcp.StdioTransport{})
}

// runRetentionLoop prunes the audit log every 24h until ctx is done. Errors
// are logged to stderr; cleanup failure is non-fatal.
func runRetentionLoop(ctx context.Context, audit *AuditWriter, days int) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if removed, err := audit.Cleanup(days, time.Now()); err != nil {
				fmt.Fprintf(os.Stderr, "atlas-mcp: scheduled audit cleanup failed: %v\n", err)
			} else if removed > 0 {
				fmt.Fprintf(os.Stderr, "atlas-mcp: scheduled audit cleanup removed %d entries\n", removed)
			}
		}
	}
}

// server holds shared state for all tool handlers. All fields are
// read-only after construction; readers may proceed without external locking.
type server struct {
	cfg     Config
	audit   *AuditWriter
	cli     *httpClient
	limiter *RateLimiter
}

// HTTPClient returns the shared HTTP client. Used by tool handlers and tests.
func (s *server) HTTPClient() *httpClient { return s.cli }

// Audit returns the audit writer. Used by tool handlers and tests.
func (s *server) Audit() *AuditWriter { return s.audit }
