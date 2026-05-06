package tax

import (
	"encoding/json"
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

	// Find the latest daily session that has tax data
	type candidate struct {
		name string
		date time.Time
	}
	var candidates []candidate
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
		candidates = append(candidates, candidate{name: e.Name(), date: parsed})
	}
	// Sort by date descending
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].date.After(candidates[i].date) {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// Find the first session with tax data
	for _, c := range candidates {
		summaryPath := filepath.Join(sessionsDir, c.name, "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			continue
		}
		if len(summary.TaxSnapshots) > 0 || summary.TotalTaxPaid != 0 {
			returnTaxSnapshot(w, summary)
			return
		}
	}

	// No session with tax data found
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"snapshots":      []domain.TaxSnapshot{},
		"before_tax_pnl": 0,
		"after_tax_pnl":  0,
		"total_tax_paid": 0,
		"is_simulated":   true,
		"note":           "no sessions with tax data — run a new simulation to populate",
	})
}

func returnTaxSnapshot(w http.ResponseWriter, summary domain.SessionSummary) {
	snapshots := summary.TaxSnapshots
	if snapshots == nil {
		snapshots = []domain.TaxSnapshot{}
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"session_id":     summary.SessionID,
		"snapshots":      snapshots,
		"before_tax_pnl": summary.PortfolioValue,
		"after_tax_pnl":  summary.AfterTaxPnL,
		"total_tax_paid": summary.TotalTaxPaid,
		"is_simulated":   true,
	})
}
