// Command atlas-mcp is the Model Context Protocol (MCP) server for atlas-go.
//
// It bridges external AI agents (Claude Desktop, Cursor, OpenCode, etc.) to
// the atlas-go HTTP API via JSON-RPC 2.0. Phase 1 wired stdio (default).
// Phase 4 wired SSE + streamable-HTTP transports with Bearer auth (see
// cmd/atlas-mcp/server/transport.go). Earlier phases added audit log
// retention, per-tool rate limiting, multi-tenant token management, and
// the admin HTTP API.
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
//	ATLAS_MCP_METRICS_ADDR          Prometheus metrics listen address (default: disabled; use 127.0.0.1:9091)
//	ATLAS_MCP_TRANSPORT             transport: stdio | sse | streamable-http (default: stdio)
//	ATLAS_MCP_ADDR                  listen address for sse/streamable-http (default: 127.0.0.1:9090)
//	PGHOST/PGPORT/PGUSER/...        PostgreSQL connection (standard libpq env vars)
//	ATLAS_MCP_SAMPLING_ENABLED      enable mcp_sample_llm (default false)
//	ATLAS_MCP_ELICITATION_ENABLED   enable mcp_elicit_user (default false)
//	ATLAS_MCP_ROOTS_ALLOWED         comma-separated file:// roots allowed when client declares none
//	ATLAS_MCP_ROOTS_READ_SIZE_CAP   max bytes per roots file read (default 1048576)
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"

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
		MetricsAddr:        os.Getenv("ATLAS_MCP_METRICS_ADDR"),
		Transport:          envOr("ATLAS_MCP_TRANSPORT", server.TransportStdio),
		Addr:               envOr("ATLAS_MCP_ADDR", "127.0.0.1:9090"),
		SamplingEnabled:    envBoolOr("ATLAS_MCP_SAMPLING_ENABLED", false),
		ElicitationEnabled: envBoolOr("ATLAS_MCP_ELICITATION_ENABLED", false),
		Roots: func() server.RootsConfig {
			cfg, err := resolveRootsConfig()
			if err != nil {
				log.Fatalf("atlas-mcp: invalid MCP roots config: %v", err)
			}
			return cfg
		}(),
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

func envBoolOr(key string, def bool) bool {
	v := strings.ToLower(os.Getenv(key))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes"
}

func envInt64Or(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return def
	}
	return n
}

func parseAllowedRoots(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// defaultParamsPath returns the conventional repo-relative path to
// configs/parameters.json. main callers can override via ATLAS_MCP_PARAMS.
func defaultParamsPath() string {
	return "configs/parameters.json"
}

// resolveRootsConfig merges parameters.json (mcp.roots section) with
// environment variable overrides. Precedence: env > parameters.json > zero.
// Returns an error if the merged AllowedRoots fails validation (see
// issue #870 / #903). The caller is expected to fail-fast on the error;
// this is intentional: granting the MCP server read access to system
// paths is a security-critical misconfiguration.
func resolveRootsConfig() (server.RootsConfig, error) {
	var base *mcpRootsConfig
	if path := os.Getenv("ATLAS_MCP_PARAMS"); path != "" {
		if loaded, err := loadMCPConfig(path); err != nil {
			//nolint:gosec // G706: path is from a trusted admin env-var; logging it for diagnostics is intentional.
			log.Printf("atlas-mcp: warning: failed to load %s: %v (using env-only)", path, err)
		} else {
			base = loaded
		}
	} else if loaded, err := loadMCPConfig(defaultParamsPath()); err != nil {
		//nolint:gosec // G706: defaultParamsPath is a constant "configs/parameters.json"; not user-controlled.
		log.Printf("atlas-mcp: warning: failed to load %s: %v (using env-only)", defaultParamsPath(), err)
	} else {
		base = loaded
	}

	env := envMCPRootsConfig{
		AllowedRoots:  parseAllowedRoots(os.Getenv("ATLAS_MCP_ROOTS_ALLOWED")),
		ReadSizeCap:   envInt64Or("ATLAS_MCP_ROOTS_READ_SIZE_CAP", 0),
		AlertOnChange: envBoolOr("ATLAS_MCP_ROOTS_ALERT_ON_CHANGE", false),
	}
	merged := mergeMCPConfig(base, env)
	if err := validateAllowedRoots(merged.AllowedRoots); err != nil {
		return server.RootsConfig{}, err
	}
	return server.RootsConfig{
		AllowedRoots: merged.AllowedRoots,
		ReadSizeCap:  merged.ReadSizeCap,
	}, nil
}

func defaultAuditLogPath() string {
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		return tmp + "atlas-mcp-audit.log"
	}
	return "/tmp/atlas-mcp-audit.log"
}
