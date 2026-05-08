package tax

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/tax"
)

type Handlers struct {
	LedgerDir string
}

func NewHandlers(ledgerDir string) *Handlers {
	return &Handlers{LedgerDir: ledgerDir}
}

func (h *Handlers) loadPositions() ([]domain.Position, error) {
	livePath := filepath.Join(h.LedgerDir, "..", "live", "state", "positions_current.jsonl")
	data, err := os.ReadFile(livePath)
	if err == nil && len(data) > 2 {
		var positions []domain.Position
		if err := json.Unmarshal(data, &positions); err == nil && len(positions) > 0 {
			return positions, nil
		}
	}

	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, nil
	}

	var latest string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() > latest {
			latest = entry.Name()
		}
	}
	if latest == "" {
		return nil, nil
	}

	summaryPath := filepath.Join(sessionsDir, latest, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		return nil, nil
	}
	var summary struct {
		PositionCount int `json:"position_count"`
	}
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		return nil, nil
	}
	if summary.PositionCount == 0 {
		return nil, nil
	}

	return nil, nil
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/tax-snapshot", shared.Get(h.HandleTaxSnapshot))
}

func (h *Handlers) HandleTaxSnapshot(r *http.Request) (int, any) {
	positions, err := h.loadPositions()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load positions: %v", err)}
	}

	if len(positions) == 0 {
		return http.StatusOK, map[string]any{
			"snapshots":      []domain.TaxSnapshot{},
			"before_tax_pnl": 0,
			"after_tax_pnl":  0,
			"total_tax_paid": 0,
			"is_simulated":   true,
			"note":           "no positions — tax snapshots require active positions to compute",
		}
	}

	calc := tax.NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())

	sellPrices := make(map[string]float64)
	dividends := make(map[string]float64)
	for _, pos := range positions {
		sellPrices[pos.Symbol] = pos.CurrentPrice
	}

	snapshots := calc.CalculatePortfolioTax(positions, sellPrices, dividends)

	var beforeTaxPnL, afterTaxPnL, totalTaxPaid float64
	for _, snap := range snapshots {
		beforeTaxPnL += snap.AfterTaxPnL + snap.TotalTax
		afterTaxPnL += snap.AfterTaxPnL
		totalTaxPaid += snap.TotalTax
	}

	return http.StatusOK, map[string]any{
		"snapshots":      snapshots,
		"before_tax_pnl": beforeTaxPnL,
		"after_tax_pnl":  afterTaxPnL,
		"total_tax_paid": totalTaxPaid,
		"is_simulated":   false,
		"note":           "tax snapshots computed from live positions using TaiwanTaxCalculator",
	}
}
