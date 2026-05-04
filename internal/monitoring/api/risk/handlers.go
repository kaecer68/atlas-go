package risk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// Handlers provides risk metrics API endpoints.
type Handlers struct {
	LedgerDir string
}

// NewHandlers creates a new risk Handlers.
func NewHandlers(ledgerDir string) *Handlers {
	return &Handlers{LedgerDir: ledgerDir}
}

// RegisterRoutes mounts risk endpoints on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/risk", h.HandleRiskMetrics)
}

// HandleRiskMetrics handles GET /api/dashboard/risk.
func (h *Handlers) HandleRiskMetrics(w http.ResponseWriter, r *http.Request) {
	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			shared.WriteJSON(w, http.StatusOK, map[string]any{"message": "no sessions available"})
			return
		}
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read sessions: %v", err))
		return
	}

	type sessionEntry struct {
		name  string
		value float64
	}
	sessions := make([]sessionEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			continue
		}
		sessions = append(sessions, sessionEntry{name: entry.Name(), value: summary.PortfolioValue})
	}

	slices.SortFunc(sessions, func(a, b sessionEntry) int {
		return strings.Compare(a.name, b.name)
	})

	portfolioValues := make([]float64, len(sessions))
	for i, s := range sessions {
		portfolioValues[i] = s.value
	}

	dailyReturns := make([]float64, 0, len(portfolioValues)-1)
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			dailyReturns = append(dailyReturns, (portfolioValues[i]-portfolioValues[i-1])/portfolioValues[i-1])
		}
	}

	var snap map[string]float64
	if len(dailyReturns) >= 30 {
		computed := risk.ComputeRiskSnapshot(dailyReturns, portfolioValues)
		snap = map[string]float64{
			"var_95":           computed.VaR95,
			"var_99":           computed.VaR99,
			"cvar_95":          computed.CVaR95,
			"max_drawdown_pct": computed.MaxDrawdownPct,
			"data_points":      float64(len(dailyReturns)),
		}
	} else {
		snap = map[string]float64{
			"var_95":            0,
			"var_99":            0,
			"cvar_95":           0,
			"max_drawdown_pct":  0,
			"data_points":       float64(len(dailyReturns)),
			"insufficient_data": 1,
		}
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"risk_snapshot": snap,
		"session_count": len(portfolioValues),
	})
}
