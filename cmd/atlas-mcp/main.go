// Command atlas-mcp is the Model Context Protocol (MCP) server for atlas-go.
//
// It bridges external AI agents (Claude Desktop, Cursor, OpenCode, etc.) to the
// atlas-go HTTP API via JSON-RPC 2.0 over stdio. Phase 1 only supports stdio;
// SSE and streamable-HTTP transports are deferred to Phase 2.
//
// Configuration via environment:
//
//	ATLAS_BASE_URL       atlas core HTTP base (default: http://127.0.0.1:8080)
//	ATLAS_API_KEY        admin API key (passed through when invoking atlas HTTP API)
//	ATLAS_MCP_AUDIT_LOG  JSONL audit-log path (default: $TMPDIR/atlas-mcp-audit.log)
//
// Phase 1 stdio security: there is no transport-level token enforcement.
// Process isolation (only the parent process can reach stdin/stdout) is the
// security boundary. The `TokenAuth` code under server/auth.go is forward-
// looking scaffolding for Phase 2 SSE/HTTP transports — do NOT advertise
// `ATLAS_MCP_TOKEN` as a working feature until Phase 2 lands.
package main

import (
	"context"
	"log"
	"os"

	"github.com/kaecer68/atlas-go/cmd/atlas-mcp/server"
)

func main() {
	cfg := server.Config{
		AtlasBaseURL: envOr("ATLAS_BASE_URL", "http://127.0.0.1:8080"),
		APIToken:     os.Getenv("ATLAS_API_KEY"),
		AuditLogPath: envOr("ATLAS_MCP_AUDIT_LOG", defaultAuditLogPath()),
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

func defaultAuditLogPath() string {
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		return tmp + "atlas-mcp-audit.log"
	}
	return "/tmp/atlas-mcp-audit.log"
}
