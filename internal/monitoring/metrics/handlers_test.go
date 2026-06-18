package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleDegraded_EmptySnapshot(t *testing.T) {
	dm := NewDegradedMetrics()
	handler := HandleDegraded(dm)

	req := httptest.NewRequest(http.MethodGet, "/degraded", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var got struct {
		Timestamp           time.Time        `json:"timestamp"`
		DegradedActivations []map[string]any `json:"degraded_activations"`
		ProviderErrors      []map[string]any `json:"provider_errors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if got.Timestamp.IsZero() {
		t.Fatalf("expected non-zero top-level timestamp, got %v", got.Timestamp)
	}
	if len(got.DegradedActivations) != 0 {
		t.Fatalf("expected empty degraded_activations, got %v", got.DegradedActivations)
	}
	if len(got.ProviderErrors) != 0 {
		t.Fatalf("expected empty provider_errors, got %v", got.ProviderErrors)
	}
}

func TestHandleDegraded_WithIncrements(t *testing.T) {
	dm := NewDegradedMetrics()
	dm.DegradedActivations.WithLabelValues("svc-a", "timeout").Inc()
	dm.DegradedActivations.WithLabelValues("svc-a", "timeout").Inc()
	dm.ProviderErrors.WithLabelValues("svc-b", "network").Inc()

	handler := HandleDegraded(dm)
	req := httptest.NewRequest(http.MethodGet, "/degraded", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var got struct {
		Timestamp           time.Time        `json:"timestamp"`
		DegradedActivations []map[string]any `json:"degraded_activations"`
		ProviderErrors      []map[string]any `json:"provider_errors"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if got.Timestamp.IsZero() {
		t.Fatalf("expected non-zero top-level timestamp, got %v", got.Timestamp)
	}

	activations := got.DegradedActivations
	if len(activations) != 1 {
		t.Fatalf("expected 1 degraded_activations entry, got %d", len(activations))
	}
	if activations[0]["service"] != "svc-a" || activations[0]["reason"] != "timeout" || activations[0]["value"] != float64(2) {
		t.Fatalf("unexpected degraded_activations entry: %v", activations[0])
	}
	if tsStr, ok := activations[0]["timestamp"].(string); !ok || tsStr == "" {
		t.Fatalf("expected non-empty per-sample timestamp string, got %v (%T)", activations[0]["timestamp"], activations[0]["timestamp"])
	} else if ts, err := time.Parse(time.RFC3339Nano, tsStr); err != nil || ts.IsZero() {
		t.Fatalf("expected parseable RFC3339 timestamp, got %q (err=%v)", tsStr, err)
	}

	errors := got.ProviderErrors
	if len(errors) != 1 {
		t.Fatalf("expected 1 provider_errors entry, got %d", len(errors))
	}
	if errors[0]["service"] != "svc-b" || errors[0]["error_type"] != "network" || errors[0]["value"] != float64(1) {
		t.Fatalf("unexpected provider_errors entry: %v", errors[0])
	}
	if tsStr, ok := errors[0]["timestamp"].(string); !ok || tsStr == "" {
		t.Fatalf("expected non-empty per-sample timestamp on error, got %v (%T)", errors[0]["timestamp"], errors[0]["timestamp"])
	} else if ts, err := time.Parse(time.RFC3339Nano, tsStr); err != nil || ts.IsZero() {
		t.Fatalf("expected parseable RFC3339 timestamp on error, got %q (err=%v)", tsStr, err)
	}
}

func TestHandleDegraded_ContentType(t *testing.T) {
	dm := NewDegradedMetrics()
	handler := HandleDegraded(dm)

	req := httptest.NewRequest(http.MethodGet, "/degraded", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
}
