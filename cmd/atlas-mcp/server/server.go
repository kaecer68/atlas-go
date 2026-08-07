package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kaecer68/atlas-go/internal/alerting"
	"github.com/kaecer68/atlas-go/internal/mcp/anomaly"
)

// Config is the immutable configuration for a server.
type Config struct {
	AtlasBaseURL       string        // atlas HTTP base, e.g. http://127.0.0.1:18080
	APIToken           string        // admin API key forwarded to atlas HTTP API (ATLAS_API_KEY)
	AuditLogPath       string        // JSONL audit log file path
	HTTPTimeout        time.Duration // per-call timeout to atlas HTTP (default 10s)
	AuditRetentionDays int           // 0 = disabled; >0 = remove entries older than N days (default 30)
	RateLimitPerMinute int           // per-(tool, tenant) requests per minute; default 120; 0 = disabled
	RateLimitBurst     int           // burst capacity; 0 = defaults to PerMinute
	MCPToken           string        // env-var fallback token (ATLAS_MCP_TOKEN)
	TokenStore         TokenStore    // optional DB-backed token store (nil = env-only)
	AdminAddr          string        // admin HTTP listen address, e.g. "127.0.0.1:9090" (empty = disabled)
	AdminToken         string        // admin API token (ATLAS_ADMIN_TOKEN)
	MetricsAddr        string        // Prometheus metrics listen address, e.g. "127.0.0.1:9091" (empty = disabled)
	SamplingEnabled    bool          // ATLAS_MCP_SAMPLING_ENABLED (default false)
	ElicitationEnabled bool          // ATLAS_MCP_ELICITATION_ENABLED (default false)
	Roots              RootsConfig   // roots filesystem boundary configuration

	// Phase 4 transport wiring. Empty Transport defaults to "stdio" for
	// backwards compatibility with existing deployments (Claude Desktop,
	// Cursor, OpenCode). Addr is required for SSE and streamable-HTTP.
	Transport string // one of TransportStdio (default), TransportSSE, TransportStreamableHTTP
	Addr      string // listen address for HTTP transports, e.g. "127.0.0.1:9090"

	// Phase 4 T1.4 — anomaly alert integration. Empty AnomalyAlertWebhook
	// means the emitter uses NoopAnomalyPublisher (no alert side-effects).
	AnomalyAlertWebhook     string        // Alertmanager-style webhook URL
	AnomalyAlertHTTPTimeout time.Duration // per-POST timeout; default 5s
	AnomalyAlertInterval    time.Duration // emitter tick interval; default 1s
	AnomalyStoreCapacity    int           // in-memory ack store cap; default 1000
}

// Run constructs a server with config and runs the stdio transport to completion.
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

	// Lifecycle instrumentation (#1267): log PID and Run start so operators
	// can correlate MCP process lifecycle with audit log entries. Transport
	// is included for multi-instance diagnostics.
	fmt.Fprintf(os.Stderr, "atlas-mcp: Run start pid=%d transport=%s\n", os.Getpid(), cfg.Transport)
	if cfg.AuditRetentionDays > 0 {
		if removed, cErr := audit.Cleanup(cfg.AuditRetentionDays, time.Now()); cErr != nil {
			fmt.Fprintf(os.Stderr, "atlas-mcp: startup audit cleanup failed: %v\n", cErr)
		} else if removed > 0 {
			fmt.Fprintf(os.Stderr, "atlas-mcp: startup audit cleanup removed %d entries\n", removed)
		}
		go runRetentionLoop(ctx, audit, cfg.AuditRetentionDays)
	}

	limiter := NewRateLimiter(RateLimiterConfig{
		PerMinute: cfg.RateLimitPerMinute,
		Burst:     cfg.RateLimitBurst,
	})
	limiter.Run(ctx)
	if cfg.RateLimitPerMinute > 0 {
		burst := cfg.RateLimitBurst
		if burst == 0 {
			burst = cfg.RateLimitPerMinute
		}
		fmt.Fprintf(os.Stderr, "atlas-mcp: rate limit active: %d/min burst=%d transport=%s\n",
			cfg.RateLimitPerMinute, burst, cfg.Transport)
	}

	auth := NewTokenAuth(cfg.MCPToken)
	if cfg.TokenStore != nil {
		auth.SetStore(cfg.TokenStore)
	}

	metrics := NewMetrics()
	detector := anomaly.NewDetector(anomaly.Config{}, metrics, nil)
	auth.SetMetrics(metrics)

	// Phase 4 T1.4 — anomaly alert/eventbus integration. The emitter
	// polls the detector's ring buffer at AnomalyAlertInterval and fans
	// out to (a) alert publisher, (b) ack store, (c) event bus (nil if
	// standalone), (d) metrics. A blank AnomalyAlertWebhook selects the
	// NoopAnomalyPublisher so the rest of the pipeline still runs in
	// tests.
	anomalyAckStore := anomaly.NewMemoryStore(cfg.AnomalyStoreCapacity)
	var anomalyPub alerting.AnomalyPublisher = &alerting.NoopAnomalyPublisher{}
	if cfg.AnomalyAlertWebhook != "" {
		anomalyPub = alerting.NewWebhookPublisher(alerting.WebhookPublisherConfig{
			URL:         cfg.AnomalyAlertWebhook,
			HTTPTimeout: cfg.AnomalyAlertHTTPTimeout,
		})
	}
	anomalyEmitter := anomaly.NewEmitter(anomaly.EmitterConfig{
		Detector:  detector,
		Publisher: anomalyPub,
		AckStore:  anomalyAckStore,
		Observer:  metrics,
		Interval:  cfg.AnomalyAlertInterval,
	})
	go anomalyEmitter.Run(ctx)

	if cfg.MetricsAddr != "" {
		go func() {
			if err := StartMetricsServer(ctx, cfg.MetricsAddr, metrics); err != nil && !errors.Is(err, context.Canceled) {
				fmt.Fprintf(os.Stderr, "atlas-mcp: metrics server: %v\n", err)
			}
		}()
	}
	if cfg.AdminAddr != "" {
		if cfg.TokenStore == nil {
			return fmt.Errorf("server: TokenStore is required when AdminAddr is set")
		}
		if cfg.AdminToken == "" {
			return fmt.Errorf("server: AdminToken is required when AdminAddr is set")
		}
		if !strings.HasPrefix(cfg.AdminAddr, "127.0.0.1:") {
			return fmt.Errorf("server: admin addr %q must bind 127.0.0.1", cfg.AdminAddr)
		}
		adminHandler := NewTokenAdminHandler(cfg.TokenStore, cfg.AdminToken)
		go func() {
			if err := StartAdminServer(ctx, cfg.AdminAddr, adminHandler); err != nil && err != context.Canceled {
				fmt.Fprintf(os.Stderr, "atlas-mcp: admin server: %v\n", err)
			}
		}()
	}

	srv := &server{
		cfg:          cfg,
		audit:        audit,
		cli:          NewHTTPClient(cfg),
		limiter:      limiter,
		auth:         auth,
		metrics:      metrics,
		detector:     detector,
		anomalyStore: anomalyAckStore,
	}

	impl := &mcp.Implementation{Name: "atlas-mcp", Version: "v0.1.0"}
	mcpSrv := mcp.NewServer(impl, &mcp.ServerOptions{
		RootsListChangedHandler: func(ctx context.Context, req *mcp.RootsListChangedRequest) {
			_ = srv.handleRootsListChanged(ctx, req)
		},
	})

	registerTools(mcpSrv, srv)
	registerAuditTools(mcpSrv, srv)
	registerAuditStateTool(mcpSrv, srv)
	registerStrategyForPeriodTool(mcpSrv, srv)
	registerResources(mcpSrv, srv)
	registerPrompts(mcpSrv)

	// Verify tool count matches expected range.
	// registerTools (called just above): 103 business tools (incl. roots 2) +
	// template_detector 2 + sector (industry_sector_list + industry_sector_lookup) 2 +
	// 0-2 sampling/elicitation (feature-gated, default off) = 107..109 tools.
	// registerAuditStateTool (audit_state) + registerStrategyForPeriodTool
	// (strategy_for_period) + stock_get_monthly_revenue (monthly_revenue,
	// in registerTools) add 3 → 117-121.
	n := RegisteredToolCount
	log.Printf("atlas-mcp: registered %d tools", n)
	if n < 115 || n > 121 {
		return fmt.Errorf("server: tool count drift: got %d, expected 115-121", n)
	}

	// Phase 4 transport dispatch. Empty Transport defaults to stdio for
	// backwards compatibility with prior deployments (Claude Desktop,
	// Cursor, OpenCode that pipe JSON-RPC over stdin/stdout).
	transport := cfg.Transport
	if transport == "" {
		transport = TransportStdio
	}
	switch transport {
	case TransportStdio:
		return ServeStdio(ctx, mcpSrv)
	case TransportSSE:
		return ServeSSE(ctx, mcpSrv, cfg.Addr, auth)
	case TransportStreamableHTTP:
		return ServeStreamableHTTP(ctx, mcpSrv, cfg.Addr, auth)
	default:
		return fmt.Errorf("server: unknown transport %q (must be %s|%s|%s)",
			transport, TransportStdio, TransportSSE, TransportStreamableHTTP)
	}
}

// runRetentionLoop prunes the audit log every 24h until ctx is done.
// On failure, retries after 1h instead of waiting a full 24h cycle (#1267).
func runRetentionLoop(ctx context.Context, audit *AuditWriter, days int) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if removed, err := audit.Cleanup(days, time.Now()); err != nil {
				fmt.Fprintf(os.Stderr, "atlas-mcp: scheduled audit cleanup failed: %v (retrying in 1h)\n", err)
				// Reset ticker to retry in 1h instead of waiting 24h.
				ticker.Reset(1 * time.Hour)
			} else if removed > 0 {
				fmt.Fprintf(os.Stderr, "atlas-mcp: scheduled audit cleanup removed %d entries\n", removed)
			}
		}
	}
}

// server holds shared state for all tool handlers.
type server struct {
	cfg          Config
	audit        *AuditWriter
	cli          *HttpClient
	limiter      *RateLimiter
	auth         *TokenAuth
	metrics      *Metrics
	detector     *anomaly.Detector
	alerter      alerting.Publisher
	rootsMu      sync.RWMutex
	rootsCache   []string
	anomalyStore anomaly.AnomalyStore
}

func (s *server) cachedRoots() []string {
	s.rootsMu.RLock()
	defer s.rootsMu.RUnlock()
	return s.rootsCache
}

// HTTPClient returns the shared HTTP client.
func (s *server) HTTPClient() *HttpClient { return s.cli }

// Audit returns the audit writer.
func (s *server) Audit() *AuditWriter { return s.audit }

// Auth returns the token authenticator.
func (s *server) Auth() *TokenAuth { return s.auth }

// AnomalyStore returns the anomaly ack store (Phase 4 T1.4).
func (s *server) AnomalyStore() anomaly.AnomalyStore { return s.anomalyStore }
