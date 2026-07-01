package alerting

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Test_WebhookPublisher_PublishAnomaly_posts_alertmanager_payload verifies
// the wire format matches the existing AlertWebhookHandler schema (version,
// status, receiver, alerts[]) so a single Alertmanager receiver can ingest
// both push (webhook_publisher) and pull (alertmanager) sources.
func Test_WebhookPublisher_PublishAnomaly_posts_alertmanager_payload(t *testing.T) {
	var captured AlertmanagerPayload
	captured.Alerts = nil
	var hits int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pub := NewWebhookPublisher(WebhookPublisherConfig{URL: srv.URL, HTTPTimeout: 2 * time.Second})
	ev := AnomalyEvent{
		AnomalyID:  "anom-001",
		Type:       "burst",
		TenantID:   "tenant-a",
		Tool:       "",
		Score:      4.2,
		DetectedAt: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		Severity:   "high",
	}

	if err := pub.PublishAnomaly(context.Background(), ev); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected 1 POST, got %d", hits)
	}
	if captured.Version != "4" {
		t.Errorf("expected version=4, got %q", captured.Version)
	}
	if captured.Status != "firing" {
		t.Errorf("expected status=firing, got %q", captured.Status)
	}
	if captured.Receiver != "atlas-mcp" {
		t.Errorf("expected receiver=atlas-mcp, got %q", captured.Receiver)
	}
	if len(captured.Alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(captured.Alerts))
	}
	a := captured.Alerts[0]
	if a.Status != "firing" {
		t.Errorf("alert status=%q", a.Status)
	}
	if a.Labels["alertname"] != "mcp_anomaly_detected" {
		t.Errorf("alertname=%q", a.Labels["alertname"])
	}
	if a.Labels["tenant_id"] != "tenant-a" {
		t.Errorf("tenant_id label=%q", a.Labels["tenant_id"])
	}
	if a.Labels["anomaly_type"] != "burst" {
		t.Errorf("anomaly_type label=%q", a.Labels["anomaly_type"])
	}
	if a.Labels["severity"] != "high" {
		t.Errorf("severity label=%q", a.Labels["severity"])
	}
	if a.Annotations["anomaly_id"] != "anom-001" {
		t.Errorf("anomaly_id annotation=%q", a.Annotations["anomaly_id"])
	}
	if a.Annotations["score"] == "" {
		t.Error("score annotation empty")
	}
	if !a.StartsAt.Equal(ev.DetectedAt) {
		t.Errorf("startsAt mismatch: got %v want %v", a.StartsAt, ev.DetectedAt)
	}
}

// Test_WebhookPublisher_PublishAnomaly_includes_tool_label_when_set
// verifies that the optional Tool field is propagated as a Prometheus label
// (not an annotation) so Alertmanager can group/filter by tool.
func Test_WebhookPublisher_PublishAnomaly_includes_tool_label_when_set(t *testing.T) {
	var captured AlertmanagerPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pub := NewWebhookPublisher(WebhookPublisherConfig{URL: srv.URL, HTTPTimeout: time.Second})
	if err := pub.PublishAnomaly(context.Background(), AnomalyEvent{
		AnomalyID: "x", Type: "tool_error_spike", TenantID: "t", Tool: "mcp_anomaly_get_recent",
		Score: 0.9, DetectedAt: time.Now().UTC(), Severity: "medium",
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if captured.Alerts[0].Labels["tool"] != "mcp_anomaly_get_recent" {
		t.Errorf("expected tool label, got %q", captured.Alerts[0].Labels["tool"])
	}
}

// Test_WebhookPublisher_PublishAnomaly_surfaces_5xx verifies that an upstream
// HTTP error becomes a non-nil return so the caller can decide whether to
// retry. The detector must not block on alert failures (see ST5 design
// notes) but it MUST see the error.
func Test_WebhookPublisher_PublishAnomaly_surfaces_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream sad"))
	}))
	defer srv.Close()

	pub := NewWebhookPublisher(WebhookPublisherConfig{URL: srv.URL, HTTPTimeout: time.Second})
	err := pub.PublishAnomaly(context.Background(), AnomalyEvent{AnomalyID: "a", Type: "burst", TenantID: "t"})
	if err == nil {
		t.Fatal("expected error on 5xx, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention 500, got %q", err.Error())
	}
}

// Test_WebhookPublisher_PublishAnomaly_respects_context_cancel verifies that
// a cancelled context short-circuits the POST. The detector should never
// hang waiting for Alertmanager.
func Test_WebhookPublisher_PublishAnomaly_respects_context_cancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Hang until client gives up.
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pub := NewWebhookPublisher(WebhookPublisherConfig{URL: srv.URL, HTTPTimeout: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before call

	if err := pub.PublishAnomaly(ctx, AnomalyEvent{AnomalyID: "a", Type: "burst", TenantID: "t"}); err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// Test_WebhookPublisher_PublishAnomaly_requires_url verifies the constructor
// rejects a blank URL. Misconfiguration should fail fast at boot, not on the
// first anomaly.
func Test_WebhookPublisher_PublishAnomaly_requires_url(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on empty URL")
		}
	}()
	_ = NewWebhookPublisher(WebhookPublisherConfig{URL: ""})
}
