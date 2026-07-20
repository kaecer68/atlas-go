package main_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/storage"
)

// TestStorageRouteRegistration verifies that /api/metrics/storage is registered
// when SetStorageReporter is called before RegisterRoutes.
func TestStorageRouteRegistration(t *testing.T) {
	d := monitoring.NewDashboardAPI("", "", nil) //lint:ignore SA1019 test stub doesn't need DataFetcher wiring
	mgr := storage.NewLifecycleManager(t.TempDir())
	d.SetStorageReporter(mgr)

	mux := http.NewServeMux()
	d.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/storage", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should not be 404 — route must exist even if report is empty.
	if w.Code == http.StatusNotFound {
		t.Fatal("/api/metrics/storage route not registered — SetStorageReporter must be called before RegisterRoutes")
	}
}
