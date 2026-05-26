package system

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// HealthHandlers provides the /health endpoint.
type HealthHandlers struct{}

func (h *HealthHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /health", shared.Get(h.HandleHealth))
}

func (h *HealthHandlers) HandleHealth(r *http.Request) (int, any) {
	return http.StatusOK, map[string]string{"status": "ok"}
}
