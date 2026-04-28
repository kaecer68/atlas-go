package tax

import (
	"encoding/json"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/domain"
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// HandleTaxSnapshot handles GET /api/dashboard/tax-snapshot.
func (h *Handlers) HandleTaxSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"snapshots":      []domain.TaxSnapshot{},
		"before_tax_pnl": 0,
		"after_tax_pnl":  0,
		"total_tax_paid": 0,
		"is_simulated":   true,
		"note":           "no trading sessions recorded — tax snapshots computed from ledger outcomes",
	})
}
