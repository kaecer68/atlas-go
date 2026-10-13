// Command atlas-mcp is the Model Context Protocol (MCP) server for atlas-go.
//
// It bridges external AI agents (Claude Desktop, Cursor, OpenCode, etc.) to
// the atlas-go HTTP API via JSON-RPC 2.0. Phase 1 wired stdio (default).
// Phase 4 wired SSE + streamable-HTTP transports with Bearer auth (see
// cmd/atlas-mcp/server/transport.go). Earlier phases added audit log
// retention, per-tool rate limiting, multi-tenant token management, and
// the admin HTTP API.
//
// Configuration precedence (highest first): CLI flag > environment > default.
//
// CLI flags:
//
//	-transport stdio|sse|streamable-http   MCP transport (env: ATLAS_MCP_TRANSPORT, default: stdio)
//	-addr     127.0.0.1:9090              listen address for sse/streamable-http
//	                                       (env: ATLAS_MCP_ADDR, default: 127.0.0.1:9090)
//
// Environment:
//
//	ATLAS_BASE_URL                  atlas core HTTP base (default: same as constants.AtlasBaseURL)
//	ATLAS_API_KEY                   admin API key (passed through when invoking atlas HTTP API)
//	ATLAS_MCP_AUDIT_LOG             JSONL audit-log path (default: $TMPDIR/atlas-mcp-audit.log)
//	ATLAS_MCP_AUDIT_RETENTION_DAYS  prune audit entries older than N days (default 30, 0 = disabled)
//	ATLAS_MCP_RATE_LIMIT_PER_MINUTE per-(tool, tenant) requests per minute (default 120; 0 = disabled)
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
//	ATLAS_MCP_STOCKPICKER_DB        SQLite ledger with stockpicker win-rate
//	                                aggregates (stock_win_rate +
//	                                stock_signal_outcomes); empty = stock_get_win_rate
//	                                reports "no data configured". Used only when
//	                                DATABASE_URL is unset or ATLAS_STORE_BACKEND
//	                                does not declare postgres (C6).
//	ATLAS_MCP_ROOTS_ALLOWED         comma-separated file:// roots allowed when client declares none
//	ATLAS_MCP_ROOTS_READ_SIZE_CAP   max bytes per roots file read (default 1048576)
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kaecer68/atlas-go/cmd/atlas-mcp/server"
	"github.com/kaecer68/atlas-go/internal/buildinfo"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/db"
)

func main() {
	// CLI flags take precedence over env vars; env vars fall back to defaults.
	// Flags default to "" so we can distinguish "user passed nothing" from
	// "user passed the default value".
	fs := flag.NewFlagSet("atlas-mcp", flag.ExitOnError)
	transportFlag := fs.String("transport", "",
		"MCP transport: stdio | sse | streamable-http (CLI > env ATLAS_MCP_TRANSPORT > stdio)")
	addrFlag := fs.String("addr", "",
		"listen address for sse/streamable-http (CLI > env ATLAS_MCP_ADDR > 127.0.0.1:9090)")
	versionFlag := fs.Bool("version", false, "print version and exit")

	if err := fs.Parse(os.Args[1:]); err != nil {
		// ExitOnError already exits; this branch is defensive.
		log.Fatalf("atlas-mcp: parse flags: %v", err)
	}

	if *versionFlag {
		info := buildinfo.Current()
		fmt.Printf("atlas-mcp %s (commit %s, built %s, %s)\n", info.Version, info.Commit, info.BuildTime, info.GoVersion)
		return
	}
	adminToken := os.Getenv("ATLAS_MCP_ADMIN_TOKEN")
	adminAddr := os.Getenv("ATLAS_MCP_ADMIN_ADDR")
	if adminToken != "" && adminAddr == "" {
		adminAddr = "127.0.0.1:9090"
	}

	cfg := server.Config{
		AtlasBaseURL:       envOr("ATLAS_BASE_URL", constants.AtlasBaseURL),
		APIToken:           os.Getenv("ATLAS_API_KEY"),
		AuditLogPath:       envOr("ATLAS_MCP_AUDIT_LOG", defaultAuditLogPath()),
		AuditRetentionDays: envIntOr("ATLAS_MCP_AUDIT_RETENTION_DAYS", 30),
		RateLimitPerMinute: envIntOr("ATLAS_MCP_RATE_LIMIT_PER_MINUTE", 120),
		RateLimitBurst:     envIntOr("ATLAS_MCP_RATE_LIMIT_BURST", 0),
		MCPToken:           os.Getenv("ATLAS_MCP_TOKEN"),
		AdminAddr:          adminAddr,
		AdminToken:         adminToken,
		MetricsAddr:        os.Getenv("ATLAS_MCP_METRICS_ADDR"),
		Transport:          firstNonEmpty(*transportFlag, os.Getenv("ATLAS_MCP_TRANSPORT"), server.TransportStdio),
		Addr:               firstNonEmpty(*addrFlag, os.Getenv("ATLAS_MCP_ADDR"), "127.0.0.1:9090"),
		SamplingEnabled:    envBoolOr("ATLAS_MCP_SAMPLING_ENABLED", false),
		ElicitationEnabled: envBoolOr("ATLAS_MCP_ELICITATION_ENABLED", false),
		StockpickerDBPath:  os.Getenv("ATLAS_MCP_STOCKPICKER_DB"),
		Roots: func() server.RootsConfig {
			cfg, err := resolveRootsConfig()
			if err != nil {
				log.Fatalf("atlas-mcp: invalid MCP roots config: %v", err)
			}
			return cfg
		}(),
	}

	// Initialize PostgreSQL and run migrations if DATABASE_URL is configured.
	// Resolve migrations path to absolute to support running from any CWD
	// (e.g., CI built binary in temp dir). Skip gracefully if not found.
	if pgURL := os.Getenv("DATABASE_URL"); pgURL != "" {
		migrationsPath := resolveMigrationsPath("sql/migrations")
		if _, err := os.Stat(migrationsPath); err == nil {
			log.Printf("atlas-mcp: running migrations from %s", migrationsPath)
			pool, err := db.Init(context.Background(), pgURL, migrationsPath)
			if err != nil {
				log.Printf("atlas-mcp: connect to postgres (non-fatal): %v", err)
			} else {
				defer pool.Close()
				cfg.TokenStore = server.NewPGTokenStore(pool)
			}
		} else {
			log.Printf("atlas-mcp: sql/migrations not found at %s, skipping token store init", migrationsPath)
		}
	}

	// C6: the two stockpicker win-rate tools read PostgreSQL when the
	// deployment declares the postgres store backend. Wired after the main
	// pool so a deployment whose DSN is broken still starts (the win-rate
	// tools then probe PG per call and fall back to the SQLite ledger).
	wireStockpickerWinRatePG(&cfg, os.Getenv("DATABASE_URL"), os.Getenv("ATLAS_STORE_BACKEND"))
	if winRateDB := cfg.WinRatePGDB; winRateDB != nil {
		defer func() { _ = winRateDB.Close() }()
	}

	if err := server.Run(context.Background(), cfg); err != nil {
		log.Fatalf("atlas-mcp: %v", err)
	}
}

// wireStockpickerWinRatePG wires the read-only PostgreSQL handle used by the
// stockpicker win-rate tools (C6) into cfg, or logs why the SQLite ledger
// stays the read path.
//
// Both switches are read from the process environment, the same way the
// DATABASE_URL read above does: the MCP server deliberately does not call
// config.Load(), which would load .env files INTO this process's environment
// as a side effect. The declared backend value is still normalized through the
// repository's single rule (server.ResolveWinRateBackend, which delegates to
// ledger.ResolveStoreBackend), so the read path cannot disagree with the
// writers about what "declared postgres" means.
//
// Every branch logs, because a deployment that believes it reads PostgreSQL
// while actually reading a stale SQLite file is the failure mode this change
// exists to remove; the alternative - silently leaving cfg.WinRatePGDB nil -
// would reintroduce it in the opposite direction.
func wireStockpickerWinRatePG(cfg *server.Config, pgURL, declaredBackend string) {
	if pgURL == "" {
		log.Printf("atlas-mcp: stockpicker win-rate reads: SQLite ledger (DATABASE_URL not set)")
		return
	}
	backend, err := server.ResolveWinRateBackend(declaredBackend)
	if err != nil {
		//nolint:gosec // G706: the value is whitelisted by loggableBackendValue (enum-shaped, length-capped) and printed with %q; gosec's taint analysis does not model the helper.
		log.Printf("atlas-mcp: stockpicker win-rate reads: SQLite ledger (ATLAS_STORE_BACKEND=%q is not a known backend)", loggableBackendValue(declaredBackend))
		return
	}
	if backend != server.WinRateBackendPostgres {
		//nolint:gosec // G706: same as above — whitelisted, length-capped, %q-quoted; the value is an operator-set env var in this process.
		log.Printf("atlas-mcp: stockpicker win-rate reads: SQLite ledger (ATLAS_STORE_BACKEND=%q does not declare postgres)", loggableBackendValue(declaredBackend))
		return
	}
	handle, err := server.OpenReadOnlyWinRatePGDB(pgURL)
	if err != nil {
		log.Printf("atlas-mcp: stockpicker win-rate reads: SQLite ledger (postgres handle: %v)", err)
		return
	}
	cfg.WinRatePGDB = handle
	log.Printf("atlas-mcp: stockpicker win-rate reads: PostgreSQL (read-only session)")
}

// loggableBackendValue renders an operator-supplied ATLAS_STORE_BACKEND value
// for the startup log. The value reaches a log line, so it is reduced to the
// characters a store backend name can contain before it is printed: an
// environment variable must not be able to forge log structure (gosec G706,
// log injection). Anything outside that set is reported as a marker instead of
// being echoed.
func loggableBackendValue(v string) string {
	if v == "" {
		return "(unset)"
	}
	if len(v) > 32 {
		return "(too long)"
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_', r == '-', r == '.':
		default:
			return "(contains characters outside [A-Za-z0-9_.-])"
		}
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// firstNonEmpty returns the first non-empty string in values. Used for
// CLI-flag > env-var > default precedence: pass (cliValue, envValue, default).
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
	return constants.ParametersFile
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
		//nolint:gosec // G706: defaultParamsPath is constants.ParametersFile; not user-controlled.
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

func resolveMigrationsPath(rel string) string {
	abs, err := filepath.Abs(rel)
	if err != nil {
		return rel
	}
	return abs
}
