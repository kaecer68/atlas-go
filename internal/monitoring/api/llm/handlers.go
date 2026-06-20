// Package llm provides the HTTP handler for the LLM health monitoring endpoint.
package llm

import (
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// Handler serves the LLM health monitoring endpoint.
type Handler struct {
	router llm.Router
}

// NewHandler creates a Handler backed by the given Router.
// Returns nil if router is nil.
func NewHandler(router llm.Router) *Handler {
	if router == nil {
		return nil
	}
	return &Handler{router: router}
}

// RegisterRoutes registers LLM monitoring routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/llm/health", shared.Get(h.HandleGetHealth))
}

// providerHealthJSON is the per-provider health representation in the response.
type providerHealthJSON struct {
	Provider    string    `json:"provider"`
	Healthy     bool      `json:"healthy"`
	LastError   string    `json:"last_error"`
	LastSuccess time.Time `json:"last_success"`
	BreakerOpen bool      `json:"breaker_open"`
}

// healthResponse is the top-level JSON response for GET /api/llm/health.
type healthResponse struct {
	Providers     map[string]providerHealthJSON `json:"providers"`
	RouterVersion string                        `json:"router_version"`
}

// HandleGetHealth returns the health status of all registered LLM providers.
func (h *Handler) HandleGetHealth(r *http.Request) (int, any) {
	statusMap := h.router.Health()
	providers := make(map[string]providerHealthJSON, len(statusMap))
	for _, s := range statusMap {
		providers[string(s.Provider)] = providerHealthJSON{
			Provider:    string(s.Provider),
			Healthy:     s.Healthy,
			LastError:   s.LastError,
			LastSuccess: s.LastSuccess,
			BreakerOpen: s.BreakerOpen,
		}
	}
	return http.StatusOK, healthResponse{
		Providers:     providers,
		RouterVersion: "v2.1",
	}
}
