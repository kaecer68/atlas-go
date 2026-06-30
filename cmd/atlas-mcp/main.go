// Command atlas-mcp is the Model Context Protocol (MCP) server for atlas-go.
//
// It bridges external AI agents (Claude Desktop, Cursor, OpenCode, etc.) to
// the atlas-go HTTP API via JSON-RPC 2.0 over stdio. Phase 1 supports stdio;
// Phase 2 added SSE + streamable-HTTP transports with Bearer auth.
// Phase 3 added audit log retention, per-tool rate limiting,
// multi-tenant token management (Item 3), and admin HTTP API.
//
// Configuration via environment:
//
//	ATLAS_BASE_URL                  atlas core HTTP base (default: http://127.0.0.1:8080)
//	ATLAS_API_KEY                   admin API key (passed through when invoking atlas HTTP API)
//	ATLAS_MCP_AUDIT_LOG             JSONL audit-log path (default: $TMPDIR/atlas-mcp-audit.log)
//	ATLAS_MCP_AUDIT_RETENTION_DAYS  prune audit entries older than N days (default 30, 0 = disabled)
//	ATLAS_MCP_RATE_LIMIT_PER_MINUTE per-(tool, tenant) requests per minute (default 0 = disabled)
//	ATLAS_MCP_RATE_LIMIT_BURST     burst capacity (default = per minute)
//	ATLAS_MCP_TOKEN                 env-var fallback token (unused when DB token store is active)
//	ATLAS_MCP_ADMIN_TOKEN           admin API token for token management API (empty = disabled)
//	ATLAS_MCP_ADMIN_ADDR            admin HTTP listen address (default: 127.0.0.1:9090 when token is set)
//	PGHOST/PGPORT/PGUSER/...        PostgreSQL connection (standard libpq env vars)
package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/kaecer68/atlas-go/cmd/atlas-mcp/server"
	"github.com/kaecer68/atlas-go/internal/db"
)

func main() {
	adminToken := os.Getenv("ATLAS_MCP_ADMIN_TOKEN")
	adminAddr := os.Getenv("ATLAS_MCP_ADMIN_ADDR")
	if adminToken != "" && adminAddr == "" {
		adminAddr = "127.0.0.1:9090"
	}

	cfg := server.Config{
		AtlasBaseURL:       envOr("ATLAS_BASE_URL", "http://127.0.0.1:8080"),
		APIToken:           os.Getenv("ATLAS_API_KEY"),
		AuditLogPath:       envOr("ATLAS_MCP_AUDIT_LOG", defaultAuditLogPath()),
		AuditRetentionDays: envIntOr("ATLAS_MCP_AUDIT_RETENTION_DAYS", 30),
		RateLimitPerMinute: envIntOr("ATLAS_MCP_RATE_LIMIT_PER_MINUTE", 0),
		RateLimitBurst:     envIntOr("ATLAS_MCP_RATE_LIMIT_BURST", 0),
		MCPToken:           os.Getenv("ATLAS_MCP_TOKEN"),
		AdminAddr:          adminAddr,
		AdminToken:         adminToken,
	}

	// Initialize PostgreSQL and run migrations if DATABASE_URL is configured.
	if pgURL := os.Getenv("DATABASE_URL"); pgURL != "" {
		pool, err := db.Init(context.Background(), pgURL, "sql/migrations")
		if err != nil {
			log.Fatalf("atlas-mcp: connect to postgres: %v", err)
		}
		defer pool.Close()
		cfg.TokenStore = server.NewPGTokenStore(pool)
	}

	if err := server.Run(context.Background(), cfg); err != nil {
		log.Fatalf("atlas-mcp: %v", err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envIntOr(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func defaultAuditLogPath() string {
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		return tmp + "atlas-mcp-audit.log"
	}
	return "/tmp/atlas-mcp-audit.log"
}
