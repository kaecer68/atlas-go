package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

	var got map[string][]map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if len(got["degraded_activations"]) != 0 {
		t.Fatalf("expected empty degraded_activations, got %v", got["degraded_activations"])
	}
	if len(got["provider_errors"]) != 0 {
		t.Fatalf("expected empty provider_errors, got %v", got["provider_errors"])
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

	var got map[string][]map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	activations := got["degraded_activations"]
	if len(activations) != 1 {
		t.Fatalf("expected 1 degraded_activations entry, got %d", len(activations))
	}
	if activations[0]["service"] != "svc-a" || activations[0]["reason"] != "timeout" || activations[0]["value"] != float64(2) {
		t.Fatalf("unexpected degraded_activations entry: %v", activations[0])
	}

	errors := got["provider_errors"]
	if len(errors) != 1 {
		t.Fatalf("expected 1 provider_errors entry, got %d", len(errors))
	}
	if errors[0]["service"] != "svc-b" || errors[0]["error_type"] != "network" || errors[0]["value"] != float64(1) {
		t.Fatalf("unexpected provider_errors entry: %v", errors[0])
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
