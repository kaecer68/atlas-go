// Package alerting receives Alertmanager webhooks and emits outbound
// security/lifecycle events for atlas-go.
//
// # Publisher (Alert-level)
//
// Publisher is the outbound event sink for code paths that detect
// significant state changes (e.g. MCP Roots list changed, MCP Elicitation
// session opened). Implementations must be safe for concurrent use and
// must not block the caller indefinitely. The default implementation
// NoopPublisher discards all events and is the safe choice when alerting
// has not been configured.
//
// # AnomalyPublisher (AnomalyEvent-level)
//
// AnomalyPublisher is the outbound sink for the MCP anomaly detector
// (burst, tool_error_spike, tenant_error_anomaly). It is intentionally a
// separate interface from Publisher because AnomalyEvent carries its own
// schema (UUID v4 id, derived severity, raw score). The default
// implementation NoopAnomalyPublisher discards events and is safe to use
// when alert integration is disabled or in tests.
//
// # Webhook handler
//
// AlertWebhookHandler is the inbound side: it decodes Alertmanager
// payloads posted to /api/v1/alerts and retains them in memory for SSE
// streams and recent-alerts endpoints.
//
// Maturity: experimental
package alerting

import (
	"context"
	"time"
)

// EventType classifies outbound alert events.
//
// Values are stable wire identifiers used in alertmanager labels
// (`alertname`) and downstream routing rules. Adding a new value is
// non-breaking; renaming an existing value is a breaking change.
type EventType string

const (
	// EventSecurityRootsChanged signals that an MCP client's declared
	// file:// roots changed (RootsV2 list_changed notification).
	EventSecurityRootsChanged EventType = "security_roots_changed"

	// EventSecurityRootsAccessDenied signals a denied mcp_roots_read_file
	// request (path outside declared roots or write-flag injection).
	EventSecurityRootsAccessDenied EventType = "security_roots_access_denied"
)

// Severity classifies the operational impact of an alert.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Alert is a structured outbound alert event.
//
// Labels carry machine-routable identifiers (alertname, severity, source).
// Annotations carry human-readable context (summary, description).
// Timestamp is set by the publisher; producers should leave it zero.
type Alert struct {
	Type        EventType
	Severity    Severity
	Source      string
	Message     string
	Labels      map[string]string
	Annotations map[string]string
	Timestamp   time.Time
}

// Publisher emits Alert-level events. Implementations must be safe for
// concurrent use; Publish should not panic on nil/empty Alert.
type Publisher interface {
	Publish(ctx context.Context, alert Alert) error
}

// NoopPublisher discards every alert. It is the default Publisher used
// when alerting has not been configured, and it is also useful in tests
// that need a Publisher but do not care about output.
type NoopPublisher struct{}

// Publish implements Publisher. It always returns nil.
func (NoopPublisher) Publish(_ context.Context, _ Alert) error {
	return nil
}

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

// AnomalyPublisher accepts anomaly events and dispatches them to an alert
// sink (Alertmanager webhook, Slack, PagerDuty, log-only, etc.).
// Implementations MUST be safe for concurrent use.
//
// The contract: PublishAnomaly is fire-and-forget on the happy path but
// returns an error so callers can decide whether to retry. A nil return value
// means the sink acknowledged receipt.
//
// Note: AnomalyPublisher is the rename applied during the pr866 rebase to
// coexist with main's Alert-level Publisher interface. Originally the PR
// introduced this as `Publisher`; renamed here so both surfaces share the
// package without a name collision.
type AnomalyPublisher interface {
	PublishAnomaly(ctx context.Context, ev AnomalyEvent) error
}

// NoopAnomalyPublisher is the default AnomalyPublisher when alert
// integration is disabled (no webhook configured, feature flag off,
// tests). It satisfies the AnomalyPublisher contract without performing
// any I/O.
//
// Note: original PR name was `NoOpPublisher`; renamed to `NoopAnomalyPublisher`
// to align with main's capitalization (`NoopPublisher`) and to encode scope.
type NoopAnomalyPublisher struct{}

// PublishAnomaly returns nil unconditionally and discards the event.
func (n *NoopAnomalyPublisher) PublishAnomaly(_ context.Context, _ AnomalyEvent) error {
	return nil
}
