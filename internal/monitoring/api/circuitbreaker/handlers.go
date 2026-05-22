package circuitbreaker

import (
	"encoding/json"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	Svc *service.CircuitBreakerService
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/circuit-breaker", shared.Get(h.HandleGetState))
	mux.Handle("POST /api/dashboard/circuit-breaker/reset", shared.AdminPost(h.HandleReset))
}

func (h *Handlers) HandleGetState(r *http.Request) (int, any) {
	state, err := h.Svc.GetCircuitBreakerState()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, state
}

func (h *Handlers) HandleReset(r *http.Request) (int, any) {
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		return http.StatusBadRequest, map[string]string{"error": "invalid request body"}
	}
	if err := h.Svc.ResetCircuitBreaker(req.Reason); err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, map[string]string{"status": "reset"}
}
