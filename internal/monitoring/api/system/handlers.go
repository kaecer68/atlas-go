package system

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
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
	mux.HandleFunc("/api/dashboard/clamping-events", h.HandleClampingEvents)
	mux.HandleFunc("/api/dashboard/conviction-clamping-events", h.HandleConvictionClampingEvents)
}

// HandlePhase3Status returns Phase 3 metrics (Swarm, PRISM, Spawning, Reflexivity, Adversarial).
func (h *Handlers) HandlePhase3Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metrics, err := h.Svc.LoadPhase3Status()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load phase3 metrics: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, metrics)
}

// HandleSystemHealth returns system health including baseline version, replay data status,
// last window, crowding warnings, regime, and data channel status.
func (h *Handlers) HandleSystemHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	health, err := h.Svc.LoadSystemHealth()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load system health: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, health)
}

func (h *Handlers) HandleClampingEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000
			}
			limit = parsed
		}
	}

	events, err := h.Svc.LoadClampingEvents(limit)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load clamping events: %v", err))
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	})
}

func (h *Handlers) HandleConvictionClampingEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000
			}
			limit = parsed
		}
	}

	events, err := h.Svc.LoadConvictionClampingEvents(limit)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load conviction clamping events: %v", err))
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	})
}
