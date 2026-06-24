package portfolio

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// RefreshETFNAV refreshes ETF NAV values for all tracked ETFs using
// the given QuoteProvider to fetch market quotes. Returns the number of
// symbols whose NAV was updated. If the provider is nil or no
// ETFAnalyzer is attached, returns 0.
//
// Public method — invoked from cmd/atlas bootstrap and screener paths
// to keep ETF NAV synchronized with the latest market quotes. Errors
// from GetQuotes are logged but non-fatal (returns 0).
func (fe *FactorEngine) RefreshETFNAV(ctx context.Context, provider QuoteProvider) int {
	fe.mu.RLock()
	ea := fe.etfAnalyzer
	fe.mu.RUnlock()
	if ea == nil || provider == nil {
		return 0
	}

	symbols := ea.AllSymbols()
	if len(symbols) == 0 {
		return 0
	}

	quotes, err := provider.GetQuotes(ctx, time.Now(), symbols)
	if err != nil {
		logging.Warn("factor_engine", "refresh_etf_nav_failed", logging.Err(err))
		return 0
	}

	return ea.UpdateNAVFromQuotes(quotes)
}

// CalculateETFScore returns an ETF-specific composite factor score.
// Delegates to the attached ETFAnalyzer. Returns a zero-score fallback if no analyzer is attached.
func (fe *FactorEngine) CalculateETFScore(symbol string, quote domain.Quote) domain.FactorScoreItem {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	if fe.etfAnalyzer == nil {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "etf_score: no analyzer attached",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	return fe.etfAnalyzer.CalculateETFScore(symbol, quote)
}
