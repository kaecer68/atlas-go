// Package prism provides HTTP handlers for the PRISM training-results API.
package prism

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/prism"
)

// Handlers serves PRISM-related endpoints.
type Handlers struct {
	pm *prism.PRISMManager
}

// NewHandlers creates PRISM handlers with the given manager.
func NewHandlers(pm *prism.PRISMManager) *Handlers {
	return &Handlers{pm: pm}
}

// RegisterRoutes attaches all PRISM endpoints to the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/prism/training-results", shared.Get(h.HandleTrainingResults))
}

// HandleTrainingResults returns all completed training results with their LLM explanations.
func (h *Handlers) HandleTrainingResults(r *http.Request) (int, any) {
	if h.pm == nil {
		return http.StatusOK, []prism.CompletedTrainingResult{}
	}
	results := h.pm.GetCompletedResults()
	if results == nil {
		return http.StatusOK, []prism.CompletedTrainingResult{}
	}
	return http.StatusOK, results
}
