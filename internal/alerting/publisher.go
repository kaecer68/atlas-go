package alerting

import (
	"context"
	"time"
)

// AnomalyEvent is the cross-package event envelope published when the MCP
// anomaly detector emits a result. It is intentionally distinct from the
// in-process anomaly.AnomalyEvent (which lives in internal/mcp/anomaly and
// already has audit-pipeline coupling) so the alerting package has no
// dependency on the MCP subsystem.
type AnomalyEvent struct {
	AnomalyID  string    // unique id (UUID v4) for the detected anomaly
	Type       string    // "burst" | "tool_error_spike" | "tenant_error_anomaly"
	TenantID   string    // owning tenant; "anonymous" if unset
	Tool       string    // empty for tenant/burst-level anomalies
	Score      float64   // raw score (z-score, error-rate, etc.)
	DetectedAt time.Time // UTC timestamp the detector emitted
	Severity   string    // "low" | "medium" | "high" — derived from Score
}

// Publisher accepts anomaly events and dispatches them to an alert sink
// (Alertmanager webhook, Slack, PagerDuty, log-only, etc.). Implementations
// MUST be safe for concurrent use.
//
// The contract: PublishAnomaly is fire-and-forget on the happy path but
// returns an error so callers can decide whether to retry. A nil return value
// means the sink acknowledged receipt.
type Publisher interface {
	PublishAnomaly(ctx context.Context, ev AnomalyEvent) error
}

// NoOpPublisher is the default Publisher when alert integration is disabled
// (no webhook configured, feature flag off, tests). It satisfies the
// Publisher contract without performing any I/O.
type NoOpPublisher struct{}

// PublishAnomaly returns nil unconditionally and discards the event.
func (n *NoOpPublisher) PublishAnomaly(_ context.Context, _ AnomalyEvent) error {
	return nil
}
