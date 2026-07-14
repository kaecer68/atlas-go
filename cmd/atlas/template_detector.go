// Package main — Stage 5 PR#4 Stage B template detector HTTP endpoints.
//
// Exposes two read-only HTTP endpoints that proxy the Stage 5 detector
// subsystem to external callers (notably cmd/atlas-mcp):
//
//	GET /api/detector/scan/status?limit=N   → recent ScanResultRow from ledger
//	GET /api/detector/registry/list          → 24 detectors + enable/disable
//
// Both endpoints are best-effort and unconditional — they must not block
// startup if the store or registry cannot be constructed.
package main

import (
	"encoding/json"
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
	if scanStore != nil {
		mux.HandleFunc("/api/detector/scan/status", handleDetectorScanStatus(scanStore))
	}
	if registry != nil {
		mux.HandleFunc("/api/detector/registry/list", handleDetectorRegistryList(registry))
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
