package metrics_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/metrics"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// mockStorageReporter implements metrics.StorageReporter for testing.
type mockStorageReporter struct {
	report any
}

func (m *mockStorageReporter) LastReport() any {
	return m.report
}

func TestHandleStorage(t *testing.T) {
	mockReport := map[string]any{
		"total_deleted": 5,
		"total_kept":    6,
		"policies": []map[string]any{
			{"dir": "macro", "deleted": 1, "kept": 2},
		},
	}

	svc := service.NewMetricsService(
		&service.MetricsCollectorAdapter{},
		&service.MetricsHistoryAdapter{},
	)
	handlers := metrics.NewHandlers(svc).WithStorageReporter(
		&mockStorageReporter{report: mockReport},
	)
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/storage", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got["total_deleted"] != float64(5) {
		t.Fatalf("expected total_deleted=5, got %v", got["total_deleted"])
	}
	if got["total_kept"] != float64(6) {
		t.Fatalf("expected total_kept=6, got %v", got["total_kept"])
	}
}

func TestHandleStorageWithoutReporter(t *testing.T) {
	svc := service.NewMetricsService(
		&service.MetricsCollectorAdapter{},
		&service.MetricsHistoryAdapter{},
	)
	handlers := metrics.NewHandlers(svc) // no WithStorageReporter
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/storage", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Route should not be registered → 404
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 when no reporter attached, got %d", w.Code)
	}
}

func TestHandleThresholds(t *testing.T) {
	violations := []service.ThresholdViolation{
		{Metric: "screening_rate", Current: 0.05, Threshold: 0.1, Severity: "warning", Message: "篩選率過低"},
		{Metric: "alert_trigger_rate", Current: 150, Threshold: 100, Severity: "critical", Message: "警報觸發率過高"},
	}
	collector := &service.MetricsCollectorAdapter{
		CheckThresholdsFunc: func(t service.AlertThreshold) []service.ThresholdViolation {
			return violations
		},
	}
	svc := service.NewMetricsService(collector, &service.MetricsHistoryAdapter{})
	handlers := metrics.NewHandlers(svc)
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/metrics/thresholds", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got["count"] != float64(2) {
		t.Errorf("count = %v, want 2", got["count"])
	}
	v, ok := got["violations"].([]any)
	if !ok || len(v) != 2 {
		t.Fatalf("violations = %v (%T), want 2-element array", got["violations"], got["violations"])
	}
	first := v[0].(map[string]any)
	if first["metric"] != "screening_rate" {
		t.Errorf("violation[0].metric = %v, want screening_rate", first["metric"])
	}
	if first["severity"] != "warning" {
		t.Errorf("violation[0].severity = %v, want warning", first["severity"])
	}
	if first["current"] != 0.05 {
		t.Errorf("violation[0].current = %v, want 0.05", first["current"])
	}
	if got["checked_at"] == nil || got["checked_at"] == "" {
		t.Error("checked_at should be present and non-empty")
	}
	threshold, ok := got["threshold"].(map[string]any)
	if !ok {
		t.Fatalf("threshold = %v, want map", got["threshold"])
	}
	if threshold["min_screening_rate"] != 0.1 {
		t.Errorf("threshold.min_screening_rate = %v, want 0.1", threshold["min_screening_rate"])
	}
	if threshold["max_alert_trigger_rate"] != 100.0 {
		t.Errorf("threshold.max_alert_trigger_rate = %v, want 100", threshold["max_alert_trigger_rate"])
	}
	if threshold["max_unacknowledged_alerts"] != 10.0 {
		t.Errorf("threshold.max_unacknowledged_alerts = %v, want 10", threshold["max_unacknowledged_alerts"])
	}
}

func TestHandleThresholds_NoViolations(t *testing.T) {
	collector := &service.MetricsCollectorAdapter{
		CheckThresholdsFunc: func(t service.AlertThreshold) []service.ThresholdViolation {
			return nil
		},
	}
	svc := service.NewMetricsService(collector, &service.MetricsHistoryAdapter{})
	handlers := metrics.NewHandlers(svc)
	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/metrics/thresholds", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got["count"] != float64(0) {
		t.Errorf("count = %v, want 0", got["count"])
	}
	v, ok := got["violations"].([]any)
	if !ok {
		t.Fatalf("violations should be a non-nil array, got %T: %v", got["violations"], got["violations"])
	}
	if len(v) != 0 {
		t.Errorf("violations = %v, want empty array", v)
	}
}
