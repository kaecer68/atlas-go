package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// WebhookPublisherConfig configures the Alertmanager-style webhook target.
// Zero HTTPTimeout defaults to 5s; zero URL panics in NewWebhookPublisher
// because misconfiguration must surface at boot, not on the first anomaly.
type WebhookPublisherConfig struct {
	URL         string        // full webhook URL (must be https in production; localhost allowed for tests)
	HTTPTimeout time.Duration // per-request timeout; default 5s
	Receiver    string        // Alertmanager receiver label; default "atlas-mcp"
}

// WebhookPublisher is the production Publisher implementation: it serializes
// AnomalyEvent to the Alertmanager webhook schema and POSTs to the configured
// URL. It is safe for concurrent use.
type WebhookPublisher struct {
	cfg WebhookPublisherConfig
	cli *http.Client
}

// NewWebhookPublisher constructs a WebhookPublisher. A blank URL panics — the
// caller (server wiring) must validate config at boot.
func NewWebhookPublisher(cfg WebhookPublisherConfig) *WebhookPublisher {
	if cfg.URL == "" {
		panic("alerting: WebhookPublisherConfig.URL is required")
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 5 * time.Second
	}
	if cfg.Receiver == "" {
		cfg.Receiver = "atlas-mcp"
	}
	return &WebhookPublisher{
		cfg: cfg,
		cli: &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

// PublishAnomaly serializes the AnomalyEvent to the Alertmanager webhook
// schema and POSTs to the configured URL. Non-2xx responses return a
// descriptive error so the caller can retry or log; ctx cancellation aborts
// the in-flight POST.
func (p *WebhookPublisher) PublishAnomaly(ctx context.Context, ev AnomalyEvent) error {
	payload := AlertmanagerPayload{
		Version:  "4",
		Status:   "firing",
		Receiver: p.cfg.Receiver,
		Alerts:   []AlertmanagerAlert{p.toAlertmanagerAlert(ev)},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("alerting: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("alerting: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.cli.Do(req)
	if err != nil {
		return fmt.Errorf("alerting: post webhook: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logging.Warn("alerting", "webhook_close_failed", logging.Err(cerr))
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("alerting: webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// toAlertmanagerAlert maps our domain event to the Alertmanager schema. Keep
// label keys stable — Alertmanager rules index by label.
func (p *WebhookPublisher) toAlertmanagerAlert(ev AnomalyEvent) AlertmanagerAlert {
	labels := map[string]string{
		"alertname":    "mcp_anomaly_detected",
		"tenant_id":    ev.TenantID,
		"anomaly_type": ev.Type,
		"severity":     ev.Severity,
	}
	if ev.Tool != "" {
		labels["tool"] = ev.Tool
	}
	annotations := map[string]string{
		"anomaly_id":  ev.AnomalyID,
		"score":       strconv.FormatFloat(ev.Score, 'f', 4, 64),
		"detected_at": ev.DetectedAt.UTC().Format(time.RFC3339),
		"summary":     fmt.Sprintf("MCP anomaly %s for tenant %s (score=%.4f)", ev.Type, ev.TenantID, ev.Score),
	}
	return AlertmanagerAlert{
		Status:      "firing",
		Labels:      labels,
		Annotations: annotations,
		StartsAt:    ev.DetectedAt,
	}
}
