// Package alerting receives Alertmanager webhooks and emits outbound
// security/lifecycle events for atlas-go.
//
// # Publisher
//
// Publisher is the outbound event sink for code paths that detect
// significant state changes (e.g. MCP Roots list changed, MCP Elicitation
// session opened). Implementations must be safe for concurrent use and
// must not block the caller indefinitely. The default implementation
// NoopPublisher discards all events and is the safe choice when alerting
// has not been configured.
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

// Publisher emits alert events. Implementations must be safe for
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
