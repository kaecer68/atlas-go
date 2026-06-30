// Package server wires the OFFICIAL Model Context Protocol SDK
// (github.com/modelcontextprotocol/go-sdk) to atlas-go's HTTP API.
//
// Phase 1 supports the stdio transport only. The server registers five core
// tools (see tools.go). Authentication and audit logging are implemented in
// auth.go and audit.go respectively.
package server

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config is the immutable configuration for a server. Build via New, never mutate
// after construction.
type Config struct {
	AtlasBaseURL string        // atlas HTTP base, e.g. http://127.0.0.1:8080
	APIToken     string        // admin API key forwarded to atlas HTTP API (ATLAS_API_KEY)
	MCPToken     string        // required token for MCP clients (ATLAS_MCP_TOKEN); empty = dev mode
	AuditLogPath string        // JSONL audit log file path
	HTTPTimeout  time.Duration // per-call timeout to atlas HTTP (default 10s)
}

// Run constructs a server with config, registers the five core tools and
// runs the stdio transport to completion. Returns the first transport error.
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
	auth := NewTokenAuth(cfg.MCPToken)

	srv := &server{
		cfg:   cfg,
		audit: audit,
		auth:  auth,
		cli:   newHTTPClient(cfg),
	}

	impl := &mcp.Implementation{Name: "atlas-mcp", Version: "v0.1.0"}
	mcpSrv := mcp.NewServer(impl, &mcp.ServerOptions{
		// Stdio transport only in Phase 1.
	})

	registerTools(mcpSrv, srv)

	return mcpSrv.Run(ctx, &mcp.StdioTransport{})
}

// server holds shared state for all tool handlers. It is not safe for concurrent
// writes; reads via the embedded http.Client are safe.
type server struct {
	cfg   Config
	audit *AuditWriter
	auth  *TokenAuth
	cli   *httpClient
}

// HTTPClient returns the shared HTTP client. Used by tool handlers and tests.
func (s *server) HTTPClient() *httpClient { return s.cli }

// Audit returns the audit writer. Used by tool handlers and tests.
func (s *server) Audit() *AuditWriter { return s.audit }
