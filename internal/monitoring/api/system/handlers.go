package system

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Svc *service.SystemService
}

func NewHandlers(svc *service.SystemService) *Handlers {
	return &Handlers{Svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/phase3-status", h.HandlePhase3Status)
	mux.HandleFunc("/api/dashboard/system-health", h.HandleSystemHealth)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// HandlePhase3Status returns Phase 3 metrics (Swarm, PRISM, Spawning, Reflexivity, Adversarial).
func (h *Handlers) HandlePhase3Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metrics, err := h.Svc.LoadPhase3Status()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load phase3 metrics: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

// HandleSystemHealth returns system health including baseline version, replay data status,
// last window, crowding warnings, regime, and data channel status.
func (h *Handlers) HandleSystemHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	health, err := h.Svc.LoadSystemHealth()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load system health: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, health)
}
