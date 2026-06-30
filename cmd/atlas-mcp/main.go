// Command atlas-mcp is the Model Context Protocol (MCP) server for atlas-go.
//
// It bridges external AI agents (Claude Desktop, Cursor, OpenCode, etc.) to
// the atlas-go HTTP API via JSON-RPC 2.0 over stdio. Phase 1 supports stdio;
// Phase 2 added SSE + streamable-HTTP transports with Bearer auth.
//
// Configuration via environment:
//
//	ATLAS_BASE_URL                  atlas core HTTP base (default: http://127.0.0.1:8080)
//	ATLAS_API_KEY                   admin API key (passed through when invoking atlas HTTP API)
//	ATLAS_MCP_AUDIT_LOG             JSONL audit-log path (default: $TMPDIR/atlas-mcp-audit.log)
//	ATLAS_MCP_AUDIT_RETENTION_DAYS  prune audit entries older than N days (default 30, 0 = disabled)
//
// Phase 2 security: stdio relies on process isolation; SSE/HTTP enforce
// Bearer token via ATLAS_MCP_TOKEN. Audit retention runs daily in the
// background when configured.
package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/kaecer68/atlas-go/cmd/atlas-mcp/server"
)

func main() {
	cfg := server.Config{
		AtlasBaseURL:       envOr("ATLAS_BASE_URL", "http://127.0.0.1:8080"),
		APIToken:           os.Getenv("ATLAS_API_KEY"),
		AuditLogPath:       envOr("ATLAS_MCP_AUDIT_LOG", defaultAuditLogPath()),
		AuditRetentionDays: envIntOr("ATLAS_MCP_AUDIT_RETENTION_DAYS", 30),
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

// envIntOr parses an env var as a non-negative int. Empty, malformed, or
// negative values fall back to def. Used for ATLAS_MCP_AUDIT_RETENTION_DAYS.
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
