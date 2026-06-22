package main

import (
	"io/fs"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// registerSimpleRoutes installs the API endpoints that have no
// orchestrator/scheduler state dependencies: Prometheus metrics,
// the liveness probe, and the static file server that serves the
// web dashboard UI from web.DistFS.
//
// All routes are best-effort and unconditional — they must not block
// startup if any of them fails to install.
func registerSimpleRoutes(mux *http.ServeMux, collector *monitoring.MetricsCollector, subFS fs.FS) {
	mux.HandleFunc("/metrics", monitoring.PrometheusHandler(collector))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	handler := staticHandler(subFS)
	mux.Handle("/", handler)
	mux.Handle("/static/", http.StripPrefix("/static/", handler))
}
