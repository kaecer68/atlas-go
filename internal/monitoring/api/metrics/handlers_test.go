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
