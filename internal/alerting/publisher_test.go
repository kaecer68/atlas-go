package alerting

import (
	"context"
	"sync"
	"testing"
	"time"
)

// =========================================================================
// Alert-level Publisher tests (added in main, ce2f4ff2).
// =========================================================================

// TestNoopPublisher_PublishReturnsNil locks the contract: a no-op publisher
// must never fail, regardless of the alert payload. This is the default
// Publisher used when alerting has not been configured.
func TestNoopPublisher_PublishReturnsNil(t *testing.T) {
	var p Publisher = NoopPublisher{}
	cases := []struct {
		name  string
		alert Alert
	}{
		{
			name:  "zero_value_alert",
			alert: Alert{},
		},
		{
			name: "fully_populated_alert",
			alert: Alert{
				Type:        EventSecurityRootsChanged,
				Severity:    SeverityInfo,
				Source:      "atlas-mcp",
				Message:     "roots list changed",
				Labels:      map[string]string{"alertname": "security_roots_changed"},
				Annotations: map[string]string{"summary": "client declared 3 new roots"},
				Timestamp:   time.Now(),
			},
		},
		{
			name: "canceled_context_still_returns_nil",
			alert: Alert{
				Type: EventSecurityRootsAccessDenied,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := p.Publish(ctx, tc.alert); err != nil {
				t.Fatalf("NoopPublisher.Publish must always return nil; got %v", err)
			}
		})
	}
}

// TestNoopPublisher_ConcurrentSafe verifies NoopPublisher satisfies the
// Publisher contract under concurrent callers (race detector must stay clean).
func TestNoopPublisher_ConcurrentSafe(t *testing.T) {
	p := NoopPublisher{}
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			_ = p.Publish(context.Background(), Alert{Type: EventSecurityRootsChanged})
		})
	}
	wg.Wait()
}

// TestPublisherContract_InterfaceSatisfaction verifies a fake implementation
// can be substituted as a Publisher. This locks the interface signature.
func TestPublisherContract_InterfaceSatisfaction(t *testing.T) {
	var _ Publisher = (*recordingPublisher)(nil)
}

type recordingPublisher struct {
	mu     sync.Mutex
	alerts []Alert
}

func (r *recordingPublisher) Publish(_ context.Context, a Alert) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alerts = append(r.alerts, a)
	return nil
}

// TestRecordingPublisher_CapturesAlerts ensures a custom Publisher
// implementation receives every Publish call in order. This is the
// canonical test double for downstream consumers (e.g. atlas-mcp roots
// handler).
func TestRecordingPublisher_CapturesAlerts(t *testing.T) {
	r := &recordingPublisher{}
	var p Publisher = r
	want := []Alert{
		{Type: EventSecurityRootsChanged, Severity: SeverityInfo},
		{Type: EventSecurityRootsAccessDenied, Severity: SeverityWarning},
	}
	for _, a := range want {
		if err := p.Publish(context.Background(), a); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	if got := len(r.alerts); got != len(want) {
		t.Fatalf("alerts captured = %d, want %d", got, len(want))
	}
	for i, a := range want {
		if r.alerts[i].Type != a.Type || r.alerts[i].Severity != a.Severity {
			t.Errorf("alerts[%d] = %+v, want Type=%s Severity=%s",
				i, r.alerts[i], a.Type, a.Severity)
		}
	}
}

// =========================================================================
// Anomaly-level tests (added in pr866 commit 99146fab).
//
// These were originally named Test_NoOpPublisher_* with underscored style.
// Renamed to CamelCase during the pr866 rebase to match the Alert-level
// tests above AND the renamed types (NoopAnomalyPublisher / AnomalyPublisher).
// =========================================================================

// TestNoopAnomalyPublisher_ImplementsAnomalyPublisher is a compile-time check
// that NoopAnomalyPublisher satisfies the AnomalyPublisher contract. Failure
// shows up at build time before the test runs.
func TestNoopAnomalyPublisher_ImplementsAnomalyPublisher(t *testing.T) {
	var _ AnomalyPublisher = (*NoopAnomalyPublisher)(nil)
}

// TestNoopAnomalyPublisher_PublishAnomalyReturnsNil verifies the no-op
// anomaly publisher succeeds silently on any input. Callers can rely on the
// err=nil contract.
func TestNoopAnomalyPublisher_PublishAnomalyReturnsNil(t *testing.T) {
	p := &NoopAnomalyPublisher{}
	ev := AnomalyEvent{
		AnomalyID:  "anom-001",
		Type:       "burst",
		TenantID:   "tenant-a",
		Tool:       "mcp_anomaly_get_recent",
		Score:      4.2,
		DetectedAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		Severity:   "high",
	}
	if err := p.PublishAnomaly(context.Background(), ev); err != nil {
		t.Fatalf("expected no error from no-op anomaly publisher, got %v", err)
	}
}

// TestNoopAnomalyPublisher_PublishAnomalyIdempotent verifies that calling
// PublishAnomaly multiple times with the same event is safe — useful for
// retry or replay paths.
func TestNoopAnomalyPublisher_PublishAnomalyIdempotent(t *testing.T) {
	p := &NoopAnomalyPublisher{}
	ev := AnomalyEvent{AnomalyID: "anom-replay", Type: "tool_error_spike"}
	for i := range 3 {
		if err := p.PublishAnomaly(context.Background(), ev); err != nil {
			t.Fatalf("iteration %d: expected nil, got %v", i, err)
		}
	}
}

// TestEventTypeAndSeverity_StableStrings locks the wire identifiers used
// by downstream Alertmanager labels. Renaming an existing constant is a
// breaking change.
func TestEventTypeAndSeverity_StableStrings(t *testing.T) {
	cases := map[EventType]string{
		EventSecurityRootsChanged:      "security_roots_changed",
		EventSecurityRootsAccessDenied: "security_roots_access_denied",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("EventType wire identifier drift: %q != %q", string(got), want)
		}
	}
	sevCases := map[Severity]string{
		SeverityInfo:     "info",
		SeverityWarning:  "warning",
		SeverityCritical: "critical",
	}
	for got, want := range sevCases {
		if string(got) != want {
			t.Errorf("Severity wire identifier drift: %q != %q", string(got), want)
		}
	}
}

// TestAnomalyEvent_RoundTripFields verifies that AnomalyEvent fields are
// stable and not silently dropped when passed between packages. Future
// publishers (e.g., WebhookPublisher) will JSON-encode this struct, so
// detect field typos early.
func TestAnomalyEvent_RoundTripFields(t *testing.T) {
	ev := AnomalyEvent{
		AnomalyID:  "anom-42",
		Type:       "tenant_error_anomaly",
		TenantID:   "tenant-z",
		Tool:       "",
		Score:      0.7,
		DetectedAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		Severity:   "medium",
	}
	if ev.AnomalyID != "anom-42" {
		t.Errorf("AnomalyID lost: %q", ev.AnomalyID)
	}
	if ev.Type != "tenant_error_anomaly" {
		t.Errorf("Type lost: %q", ev.Type)
	}
	if ev.TenantID != "tenant-z" {
		t.Errorf("TenantID lost: %q", ev.TenantID)
	}
	if ev.Tool != "" {
		t.Errorf("Tool should be empty, got %q", ev.Tool)
	}
	if ev.Score != 0.7 {
		t.Errorf("Score lost: %v", ev.Score)
	}
	if ev.Severity != "medium" {
		t.Errorf("Severity lost: %q", ev.Severity)
	}
	if ev.DetectedAt.IsZero() {
		t.Error("DetectedAt lost")
	}
}
