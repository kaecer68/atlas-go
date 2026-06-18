package metrics

import (
	"encoding/json"
	"net/http"
)

// HandleDegraded returns an HTTP handler that exposes the current degraded
// metrics snapshot as JSON.
func HandleDegraded(dm *DegradedMetrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := dm.Snapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	}
}
