package tax

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// Handlers provides tax-related API endpoints.
type Handlers struct {
	LedgerDir string
}

// NewHandlers creates a new tax Handlers.
func NewHandlers(ledgerDir string) *Handlers {
	return &Handlers{LedgerDir: ledgerDir}
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

	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"snapshots":      []domain.TaxSnapshot{},
			"before_tax_pnl": 0,
			"after_tax_pnl":  0,
			"total_tax_paid": 0,
			"is_simulated":   true,
			"note":           "no sessions directory found",
		})
		return
	}

	// Find the latest daily session with tax data
	type candidate struct {
		name string
		date time.Time
	}
	var latest candidate
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), "-daily") {
			continue
		}
		dateStr := strings.TrimPrefix(e.Name(), "session-")
		dateStr = strings.TrimSuffix(dateStr, "-daily")
		parsed, err := time.Parse("20060102", dateStr)
		if err != nil {
			continue
		}
		if parsed.After(latest.date) {
			latest = candidate{name: e.Name(), date: parsed}
		}
	}
	if latest.name == "" {
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"snapshots":      []domain.TaxSnapshot{},
			"before_tax_pnl": 0,
			"after_tax_pnl":  0,
			"total_tax_paid": 0,
			"is_simulated":   true,
			"note":           "no daily sessions found",
		})
		return
	}

	summaryPath := filepath.Join(sessionsDir, latest.name, "summary.json")
	bytes, err := os.ReadFile(summaryPath)
	if err != nil {
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"snapshots":      []domain.TaxSnapshot{},
			"before_tax_pnl": 0,
			"after_tax_pnl":  0,
			"total_tax_paid": 0,
			"is_simulated":   true,
			"note":           "session summary not found: " + latest.name,
		})
		return
	}

	var summary domain.SessionSummary
	if err := json.Unmarshal(bytes, &summary); err != nil {
		log.Printf("[TaxHandler] warn: failed to parse summary for %s: %v", latest.name, err)
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"snapshots":      []domain.TaxSnapshot{},
			"before_tax_pnl": 0,
			"after_tax_pnl":  0,
			"total_tax_paid": 0,
			"is_simulated":   true,
			"note":           "failed to parse session summary",
		})
		return
	}

	snapshots := summary.TaxSnapshots
	if snapshots == nil {
		snapshots = []domain.TaxSnapshot{}
	}

	beforeTax := summary.PortfolioValue // approximate: portfolio value represents pre-tax state
	if summary.AfterTaxPnL != 0 || summary.TotalTaxPaid != 0 {
		// If tax was computed, use actual tax fields
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"session_id":     summary.SessionID,
			"snapshots":      snapshots,
			"before_tax_pnl": summary.PortfolioValue,
			"after_tax_pnl":  summary.AfterTaxPnL,
			"total_tax_paid": summary.TotalTaxPaid,
			"is_simulated":   true,
		})
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"session_id":     summary.SessionID,
		"snapshots":      snapshots,
		"before_tax_pnl": beforeTax,
		"after_tax_pnl":  summary.AfterTaxPnL,
		"total_tax_paid": summary.TotalTaxPaid,
		"is_simulated":   true,
		"note":           "tax data not yet recorded in this session — run a new simulation to populate",
	})
}
