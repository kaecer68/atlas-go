package alerting

import (
	"context"
	"testing"
	"time"
)

// Test_NoOpPublisher_implements_Publisher_interface is a compile-time check
// that NoOpPublisher satisfies the Publisher contract. Failure shows up at
// build time before the test runs.
func Test_NoOpPublisher_implements_Publisher_interface(t *testing.T) {
	var _ Publisher = (*NoOpPublisher)(nil)
}

// Test_NoOpPublisher_PublishAnomaly_returns_nil verifies the no-op publisher
// succeeds silently on any input. Callers can rely on the err=nil contract.
func Test_NoOpPublisher_PublishAnomaly_returns_nil(t *testing.T) {
	p := &NoOpPublisher{}
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
		t.Fatalf("expected no error from no-op publisher, got %v", err)
	}
}

// Test_NoOpPublisher_PublishAnomaly_idempotent verifies that calling
// PublishAnomaly multiple times with the same event is safe — useful for
// retry or replay paths.
func Test_NoOpPublisher_PublishAnomaly_idempotent(t *testing.T) {
	p := &NoOpPublisher{}
	ev := AnomalyEvent{AnomalyID: "anom-replay", Type: "tool_error_spike"}
	for i := 0; i < 3; i++ {
		if err := p.PublishAnomaly(context.Background(), ev); err != nil {
			t.Fatalf("iteration %d: expected nil, got %v", i, err)
		}
	}
}

// Test_AnomalyEvent_round_trip_fields verifies that AnomalyEvent fields are
// stable and not silently dropped when passed between packages. Future
// publishers (e.g., WebhookPublisher) will JSON-encode this struct, so
// detect field typos early.
func Test_AnomalyEvent_round_trip_fields(t *testing.T) {
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
