package main

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/portprobe"
)

// portHealthReport describes one component's port occupancy state at the
// moment /health was queried. State is a JSON-friendly version of
// portprobe.State: "free" | "healthy" | "foreign" | "unknown".
type portHealthReport struct {
	Addr    string `json:"addr"`
	State   string `json:"state"`
	PID     int    `json:"pid,omitempty"`
	Command string `json:"command,omitempty"`
	Error   string `json:"error,omitempty"`
}

// healthResponse is the /health JSON body. Status preserves the legacy
// "ok" string so existing consumers (Docker healthcheck, metrics scrapers)
// keep working; Ports is additive.
type healthResponse struct {
	Status string                      `json:"status"`
	Ports  map[string]portHealthReport `json:"ports"`
}

// healthConfig parameterises newHealthHandler. Probe is the function used
// to inspect each port; tests override it to avoid binding real sockets.
type healthConfig struct {
	Probe func(addr string) portHealthReport
}

func reportPort(addr string) portHealthReport {
	state, occ, err := portprobe.Probe(addr)
	if err != nil {
		return portHealthReport{Addr: addr, State: "unknown", Error: err.Error()}
	}
	switch state {
	case portprobe.StateFree:
		return portHealthReport{Addr: addr, State: "free"}
	case portprobe.StateHealthy:
		return portHealthReport{Addr: addr, State: "healthy", PID: occ.PID, Command: occ.Command}
	case portprobe.StateForeign:
		return portHealthReport{Addr: addr, State: "foreign", PID: occ.PID, Command: occ.Command}
	}
	return portHealthReport{Addr: addr, State: "unknown"}
}

// newHealthHandler returns the JSON /health endpoint that mirrors the
// legacy "ok" payload while also reporting occupancy of atlas_http (8080)
// and fubon_proxy (8081) via portprobe.Probe. Oracle 反駁 final plan PR 2.
func newHealthHandler(cfg healthConfig) http.Handler {
	probe := cfg.Probe
	if probe == nil {
		probe = reportPort
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := healthResponse{
			Status: "ok",
			Ports: map[string]portHealthReport{
				"atlas_http":  probe("127.0.0.1:8080"),
				"fubon_proxy": probe("127.0.0.1:8081"),
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(body)
	})
}

// registerSimpleRoutes installs the API endpoints that have no
// orchestrator/scheduler state dependencies: Prometheus metrics,
// the liveness probe, and the static file servers that serve the
// admin_web and client_web SPAs under /admin/ and /client/. The root
// path / redirects to /client/ so general users land on the investor
// interface by default.
//
// All routes are best-effort and unconditional — they must not block
// startup if any of them fails to install.
func registerSimpleRoutes(mux *http.ServeMux, collector *monitoring.MetricsCollector, adminFS, clientFS fs.FS, rc readyChecker) {
	mux.HandleFunc("/metrics", monitoring.PrometheusHandler(collector))
	mux.Handle("/health", newHealthHandler(healthConfig{}))
	mux.Handle("/ready", newReadyHandler(rc))

	// Redirect root to the client-facing UI. Bare /admin and /client are
	// also redirected so the trailing-slash relative asset paths in
	// index.html resolve correctly.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/client/", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/client", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/client/", http.StatusMovedPermanently)
	})
	mux.Handle("/admin/", http.StripPrefix("/admin/", staticHandler(adminFS)))
	mux.Handle("/client/", http.StripPrefix("/client/", staticHandler(clientFS)))
}

// readyResponse is the GET /ready JSON body. Unlike /health (liveness),
// /ready checks whether the system is ready to serve traffic: database
// connectivity, replay data availability, and Gateway channel readiness.
type readyResponse struct {
	Status string            `json:"status"` // "ready" or "not_ready"
	Checks map[string]string `json:"checks"` // check name → "ok" | "failed: reason"
}

// readyChecker holds the state needed by newReadyHandler. Fields are set
// by run() during startup and remain immutable after the server starts.
type readyChecker struct {
	dbDSN       string
	replayPath  string
	gatewayChan int // number of registered Gateway channels (0 = not ready)
	// skipGateway bypasses the gateway_channels check. Set to true in
	// modes (e.g. live trading) that do not initialize an apigateway.Gateway,
	// where a 0 channel count is expected and should not mark the system
	// as not_ready.
	skipGateway bool
}

func newReadyHandler(rc readyChecker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := readyResponse{
			Status: "ready",
			Checks: make(map[string]string, 3),
		}

		// Check 1: PostgreSQL connectivity
		if rc.dbDSN != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			conn, err := pgx.Connect(ctx, rc.dbDSN)
			if err != nil {
				resp.Checks["postgres"] = "failed: " + err.Error()
				resp.Status = "not_ready"
			} else {
				_ = conn.Close(ctx)
				resp.Checks["postgres"] = "ok"
			}
		} else {
			resp.Checks["postgres"] = "skipped (DATABASE_URL unset)"
		}

		// Check 2: Replay data availability
		if rc.replayPath != "" {
			if _, err := os.Stat(rc.replayPath); os.IsNotExist(err) {
				resp.Checks["replay_data"] = "failed: file not found at " + rc.replayPath
				resp.Status = "not_ready"
			} else {
				resp.Checks["replay_data"] = "ok"
			}
		}

		// Check 3: Gateway channel count (skipped in modes without Gateway)
		if rc.skipGateway {
			resp.Checks["gateway_channels"] = "skipped (mode does not initialize Gateway)"
		} else if rc.gatewayChan > 0 {
			resp.Checks["gateway_channels"] = "ok"
		} else {
			resp.Checks["gateway_channels"] = "failed: 0 channels registered"
			resp.Status = "not_ready"
		}

		w.Header().Set("Content-Type", "application/json")
		if resp.Status == "ready" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
}
