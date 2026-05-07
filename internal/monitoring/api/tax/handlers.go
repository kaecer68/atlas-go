package tax

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// Handlers provides tax-related API endpoints.
type Handlers struct{}

// NewHandlers creates a new tax Handlers.
func NewHandlers() *Handlers {
	return &Handlers{}
}

// RegisterRoutes mounts tax endpoints on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/tax-snapshot", h.HandleTaxSnapshot)
}

// HandleTaxSnapshot handles GET /api/dashboard/tax-snapshot.
func (h *Handlers) HandleTaxSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"snapshots":      []domain.TaxSnapshot{},
		"before_tax_pnl": 0,
		"after_tax_pnl":  0,
		"total_tax_paid": 0,
		"is_simulated":   true,
		"note":           "no trading sessions recorded — tax snapshots computed from ledger outcomes",
	})
}
