package tax

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/tax"
)

type DividendProvider interface {
	GetLatestDividend(ctx context.Context, symbol string) (*domain.DividendRecord, error)
}

type Handlers struct {
	LedgerDir        string
	DividendProvider DividendProvider
}

func NewHandlers(ledgerDir string, dividendProvider DividendProvider) *Handlers {
	return &Handlers{
		LedgerDir:        ledgerDir,
		DividendProvider: dividendProvider,
	}
}

func readPositionsFromFile(path string) []domain.Position {
	data, err := os.ReadFile(path)
	if err != nil || len(data) <= 2 {
		return nil
	}
	var positions []domain.Position
	if err := json.Unmarshal(data, &positions); err != nil || len(positions) == 0 {
		return nil
	}
	return positions
}

func (h *Handlers) loadPositions() []domain.Position {
	ledgerDir := h.LedgerDir
	if !filepath.IsAbs(ledgerDir) {
		if wd, err := os.Getwd(); err == nil {
			ledgerDir = filepath.Join(wd, ledgerDir)
		}
	}

	livePath := filepath.Join(ledgerDir, "live", "state", "positions_current.json")
	if positions := readPositionsFromFile(livePath); positions != nil {
		return positions
	}

	livePathJSONL := filepath.Join(h.LedgerDir, "live", "state", "positions_current.jsonl")
	if positions := readPositionsFromFile(livePathJSONL); positions != nil {
		return positions
	}

	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		logging.Warn("tax_handler", "read_sessions_dir_failed", logging.Err(err))
		return nil
	}

	var latest string
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() > latest {
			latest = entry.Name()
		}
	}
	if latest == "" {
		return nil
	}

	summaryPath := filepath.Join(sessionsDir, latest, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		logging.Warn("tax_handler", "read_summary_failed", logging.Err(err))
		return nil
	}
	var summary struct {
		PositionCount int `json:"position_count"`
	}
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		return nil
	}
	if summary.PositionCount == 0 {
		return nil
	}

	positionsPath := filepath.Join(sessionsDir, latest, "positions.json")
	positionsData, err := os.ReadFile(positionsPath)
	if err != nil {
		logging.Warn("tax_handler", "read_positions_failed", logging.Err(err))
		return nil
	}
	var positions []domain.Position
	if err := json.Unmarshal(positionsData, &positions); err != nil {
		logging.Warn("tax_handler", "parse_positions_failed", logging.Err(err))
		return nil
	}
	if len(positions) > 0 {
		return positions
	}

	return nil
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/tax-snapshot", shared.Get(h.HandleTaxSnapshot))
}

func (h *Handlers) HandleTaxSnapshot(r *http.Request) (int, any) {
	positions := h.loadPositions()

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
		if h.DividendProvider != nil {
			symbolWithoutSuffix := strings.TrimSuffix(pos.Symbol, ".TW")
			record, err := h.DividendProvider.GetLatestDividend(r.Context(), symbolWithoutSuffix)
			if err == nil && record != nil {
				dividends[pos.Symbol] = record.CashDividend * float64(pos.Quantity)
			}
		}
	}

	snapshots := calc.CalculatePortfolioTax(positions, sellPrices, dividends)

	var beforeTaxPnL, afterTaxPnL, totalTaxPaid, totalDividendTax float64
	for _, snap := range snapshots {
		beforeTaxPnL += snap.AfterTaxPnL + snap.TotalTax
		afterTaxPnL += snap.AfterTaxPnL
		totalTaxPaid += snap.TotalTax
		totalDividendTax += snap.DividendTax
	}

	note := "tax snapshots computed from live positions using TaiwanTaxCalculator"
	if h.DividendProvider == nil {
		note += "; dividend tax is 0 because dividend data provider is not configured"
	} else if totalDividendTax == 0 {
		note += "; no dividend tax accrued — holdings may not have paid dividends in the current period"
	}

	return http.StatusOK, map[string]any{
		"snapshots":          snapshots,
		"before_tax_pnl":     beforeTaxPnL,
		"after_tax_pnl":      afterTaxPnL,
		"total_tax_paid":     totalTaxPaid,
		"total_dividend_tax": totalDividendTax,
		"is_simulated":       false,
		"note":               note,
	}
}
