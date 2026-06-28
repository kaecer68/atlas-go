package main

import (
	"io/fs"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// registerSimpleRoutes installs the API endpoints that have no
// orchestrator/scheduler state dependencies: Prometheus metrics,
// the liveness probe, and the static file server that serves the
// web dashboard UI from web.DistFS. It also serves the split
// admin_web and client_web SPAs under /admin/ and /client/.
//
// All routes are best-effort and unconditional — they must not block
// startup if any of them fails to install.
func registerSimpleRoutes(mux *http.ServeMux, collector *monitoring.MetricsCollector, subFS, adminFS, clientFS fs.FS) {
	mux.HandleFunc("/metrics", monitoring.PrometheusHandler(collector))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	handler := staticHandler(subFS)
	mux.Handle("/", handler)
	mux.Handle("/static/", http.StripPrefix("/static/", handler))

	// Admin and investor SPAs. Redirect bare /admin and /client so the
	// relative asset paths in index.html resolve correctly.
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/client", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/client/", http.StatusMovedPermanently)
	})
	mux.Handle("/admin/", http.StripPrefix("/admin/", staticHandler(adminFS)))
	mux.Handle("/client/", http.StripPrefix("/client/", staticHandler(clientFS)))
}
