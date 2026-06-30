// Package deployment provides the deployment dashboard handler for the admin API.
// It exposes the fubon-proxy ProcessManager status at GET /api/admin/live/deployment/dashboard.
package deployment

import (
	"encoding/json"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/fubonproxy"
)

// Handlers holds dependencies for the deployment dashboard endpoint.
type Handlers struct {
	pm *fubonproxy.ProcessManager
}

// NewHandlers creates a Handlers wired to the given ProcessManager.
// pm may be nil if the fubon-proxy manager is not initialized; in that case
// HandleDeploymentDashboard returns 503 Service Unavailable.
func NewHandlers(pm *fubonproxy.ProcessManager) *Handlers {
	return &Handlers{pm: pm}
}

// HandleDeploymentDashboard serves the fubon-proxy deployment status as JSON.
// Returns 503 when pm is nil (manager not initialized).
func (h *Handlers) HandleDeploymentDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.pm == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "fubon-proxy process manager not available",
		})
		return
	}

	status := h.pm.Status()
	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
