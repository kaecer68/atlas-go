package server

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/anomaly"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/metrics"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Config is the immutable configuration for a server.
type Config struct {
	AtlasBaseURL       string        // atlas HTTP base, e.g. http://127.0.0.1:8080
	APIToken           string        // admin API key forwarded to atlas HTTP API (ATLAS_API_KEY)
	AuditLogPath       string        // JSONL audit log file path
	HTTPTimeout        time.Duration // per-call timeout to atlas HTTP (default 10s)
	AuditRetentionDays int           // 0 = disabled; >0 = remove entries older than N days (default 30)
	RateLimitPerMinute int           // per-(tool, tenant) requests per minute; 0 = disabled
	RateLimitBurst     int           // burst capacity; 0 = defaults to PerMinute
	MCPToken           string        // env-var fallback token (ATLAS_MCP_TOKEN)
	TokenStore         TokenStore    // optional DB-backed token store (nil = env-only)
	AdminAddr          string        // admin HTTP listen address, e.g. "127.0.0.1:9090" (empty = disabled)
	AdminToken         string        // admin API token (ATLAS_ADMIN_TOKEN)
	MetricsAddr        string        // metrics HTTP listen address, e.g. "127.0.0.1:9091" (empty = disabled)
	AnomalyConfig      anomaly.Config
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

	auth := NewTokenAuth(cfg.MCPToken)
	if cfg.TokenStore != nil {
		auth.SetStore(cfg.TokenStore)
	}

	// Start admin HTTP handler if configured. It requires a TokenStore and
	// must bind loopback only.
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

	anomalyCfg := cfg.AnomalyConfig
	if anomalyCfg.DetectIntervalSec == 0 {
		anomalyCfg = anomaly.DefaultConfig()
	}
	anomalyBus := eventbus.NewChannelEventBus(100)
	anomalyRegistry := anomaly.NewRegistry(anomalyCfg, &anomalyEventPublisher{bus: anomalyBus}, anomaly.NoopAlerter{})
	anomalyRegistry.Register(anomaly.NewBaseline5m24hDetector(anomalyCfg))
	anomalyRegistry.Register(anomaly.NewPerToolErrorDetector(anomalyCfg))
	anomalyRegistry.Register(anomaly.NewPerTenantErrorDetector(anomalyCfg))

	srv := &server{
		cfg:     cfg,
		audit:   audit,
		cli:     newHTTPClient(cfg),
		limiter: limiter,
		auth:    auth,
		metrics: metrics.NewRegistry(),
		anomaly: anomalyRegistry,
	}

	go anomalyRegistry.RunLoop(ctx, srv.fetchAuditEntriesForAnomaly)

	go func() {
		<-ctx.Done()
		_ = anomalyBus.Close()
	}()

	// Start metrics HTTP endpoint if configured.
	if cfg.MetricsAddr != "" {
		go func() {
			if err := srv.metrics.StartServer(ctx, cfg.MetricsAddr); err != nil && err != context.Canceled {
				fmt.Fprintf(os.Stderr, "atlas-mcp: metrics server: %v\n", err)
			}
		}()
	}

	impl := &mcp.Implementation{Name: "atlas-mcp", Version: "v0.1.0"}
	mcpSrv := mcp.NewServer(impl, &mcp.ServerOptions{})

	registerTools(mcpSrv, srv)
	registerAuditTools(mcpSrv, srv)
	registerResources(mcpSrv, srv)
	registerPrompts(mcpSrv)

	return mcpSrv.Run(ctx, &mcp.StdioTransport{})
}

// runRetentionLoop prunes the audit log every 24h until ctx is done.
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

// server holds shared state for all tool handlers.
type server struct {
	cfg     Config
	audit   *AuditWriter
	cli     *httpClient
	limiter *RateLimiter
	auth    *TokenAuth
	metrics *metrics.Registry
	anomaly *anomaly.Registry
}

// HTTPClient returns the shared HTTP client.
func (s *server) HTTPClient() *httpClient { return s.cli }

// Audit returns the audit writer.
func (s *server) Audit() *AuditWriter { return s.audit }

// Auth returns the token authenticator.
func (s *server) Auth() *TokenAuth { return s.auth }

// Anomaly returns the anomaly detector registry.
func (s *server) Anomaly() *anomaly.Registry { return s.anomaly }

// anomalyEventPublisher adapts anomaly events to the project event bus.
type anomalyEventPublisher struct {
	bus *eventbus.ChannelEventBus
}

func (p *anomalyEventPublisher) Publish(_ context.Context, eventType string, payload any) error {
	p.bus.Publish(eventbus.BusEvent{
		ID:            fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:          eventbus.EventType(eventType),
		Timestamp:     time.Now(),
		Payload:       payload,
		Severity:      "warning",
		SchemaVersion: 1,
	})
	return nil
}

func (s *server) fetchAuditEntriesForAnomaly(ctx context.Context) ([]anomaly.AuditEntryV2, error) {
	entries, err := ReadAuditEntries(s.cfg.AuditLogPath, 1, time.Now())
	if err != nil {
		return nil, fmt.Errorf("read audit entries: %w", err)
	}
	return toAnomalyEntries(entries), nil
}

func toAnomalyEntries(entries []AuditEntry) []anomaly.AuditEntryV2 {
	out := make([]anomaly.AuditEntryV2, len(entries))
	for i, e := range entries {
		out[i] = anomaly.AuditEntryV2{
			SchemaVersion: e.SchemaVersion,
			TS:            e.TS,
			SessionID:     e.SessionID,
			AgentID:       e.AgentID,
			TenantID:      e.TenantID,
			Tool:          e.Tool,
			ArgsHash:      e.ArgsHash,
			Status:        e.Status,
			LatencyMS:     e.LatencyMS,
			DurationMS:    e.DurationMS,
			Transport:     e.Transport,
			Error:         e.Error,
		}
	}
	return out
}
