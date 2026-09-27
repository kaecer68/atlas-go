// Package main — Stage 5 PR#4 Stage B template detector HTTP endpoints.
//
// Exposes two read-only HTTP endpoints that proxy the Stage 5 detector
// subsystem to external callers (notably cmd/atlas-mcp):
//
//	GET /api/detector/scan/status?limit=N   → recent ScanResultRow from ledger
//	GET /api/detector/registry/list          → 29 detectors + enable/disable
//	                                            (count is registry-driven — never hardcode;
//	                                             guarded by internal/narrative/detector_count_gate_test.go)
//
// Both endpoints are best-effort and unconditional — they must not block
// startup if the store or registry cannot be constructed.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// RegisterTemplateDetectorRoutes installs the two detector HTTP handlers.
// Callers should pass a non-nil registry (NewDefaultDetectorRegistry()) and
// a non-nil store (ledger.NewDetectorScanStore(cfg)). Nil deps are silently
// ignored — the corresponding endpoint is skipped rather than registered with
// a nil dependency.
func RegisterTemplateDetectorRoutes(
	mux *http.ServeMux,
	registry *narrative.DetectorRegistry,
	scanStore ledger.DetectorScanStore,
) {
	if mux == nil {
		return
	}
	mux.Handle("GET /api/detector/scan/status", handleDetectorScanStatus(scanStore))
	if registry != nil {
		mux.Handle("GET /api/detector/registry/list", handleDetectorRegistryList(registry))
	}
}

// handleDetectorScanStatus returns up to ?limit=N (default 100) most recent
// detector scan results, newest first.
func handleDetectorScanStatus(store ledger.DetectorScanStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "detector scan store unavailable (sqlite backend required, see Stage 5 PR#4 contract)", http.StatusServiceUnavailable)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit < 0 {
			limit = 0
		}
		rows, err := store.LoadRecentScans(r.Context(), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rows)
	}
}

// handleDetectorRegistryList returns the list of registered detectors with
// their current enable/disable state. Used by the MCP detector_registry_list
// tool and the admin dashboard.
func handleDetectorRegistryList(registry *narrative.DetectorRegistry) http.HandlerFunc {
	type detectorView struct {
		Theme   string `json:"theme"`
		Enabled bool   `json:"enabled"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		themes := registry.Themes()
		out := make([]detectorView, 0, len(themes))
		for _, theme := range themes {
			d, ok := registry.Get(theme)
			if !ok {
				continue
			}
			out = append(out, detectorView{Theme: d.Theme(), Enabled: d.Enabled()})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// templateDetectorRouteLog builds the startup log line for /api/detector/*.
//
// The detector count is read from the registry at runtime and never written
// down: an earlier version of this line hardcoded a count and kept printing it
// long after the registry had grown, which leaked a false number into
// downstream audit output. Never reintroduce a literal count here — the size is
// whatever the registry registered.
//
// Guarded by:
//   - cmd/atlas/template_detector_count_log_test.go (this message follows the registry)
//   - internal/narrative/detector_count_gate_test.go (doc/comment count claims)
func templateDetectorRouteLog(reg *narrative.DetectorRegistry, scanStoreAvailable bool) string {
	return fmt.Sprintf(
		"[TemplateDetector] registered /api/detector/* routes (%d detectors + scan store=%v)",
		len(reg.List()), scanStoreAvailable,
	)
}
