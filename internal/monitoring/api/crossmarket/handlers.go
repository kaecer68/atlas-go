package crossmarket

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// Handlers provides HTTP handlers for cross-market monitoring endpoints.
type Handlers struct {
	Svc *service.CrossMarketService
}

// RegisterRoutes registers cross-market routes on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/cross-market/status", shared.Get(h.HandleStatus))
	mux.Handle("GET /api/cross-market/correlation", shared.Get(h.HandleCorrelation))
	mux.Handle("GET /api/dashboard/us-indices", shared.Get(h.HandleUSIndices))
}

// HandleStatus returns the full US-TW cross-market status snapshot.
func (h *Handlers) HandleStatus(r *http.Request) (int, any) {
	status, err := h.Svc.GetStatus(r.Context())
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, status
}

// HandleCorrelation returns the current SPX-TWSE correlation estimate.
func (h *Handlers) HandleCorrelation(r *http.Request) (int, any) {
	corr, err := h.Svc.GetCorrelation()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, corr
}

// HandleUSIndices returns current US market index and tech stock values.
func (h *Handlers) HandleUSIndices(r *http.Request) (int, any) {
	indices, err := h.Svc.GetUSIndices(r.Context())
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, indices
}
