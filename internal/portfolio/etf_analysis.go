package portfolio

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ETFAnalysis holds per-ETF analytics data.
type ETFAnalysis struct {
	Symbol              string
	NAV                 float64 // Net Asset Value per share
	MarketPrice         float64 // current market price
	PremiumDiscount     float64 // (MarketPrice - NAV) / NAV as fraction
	ExpenseRatio        float64 // annual expense ratio as fraction (e.g. 0.0032 = 0.32%)
	TrackingErrorAnnual float64 // annualized tracking error vs benchmark
	TrackingError20d    float64 // 20-day realized tracking error
	Benchmark           string  // benchmark index name
}

// ETFAnalyzer computes ETF-specific factor scores.
type ETFAnalyzer struct {
	metadata map[string]ETFMetadata // per-symbol metadata
	hp       *HistoricalPrices      // for tracking error calc
	mu       sync.RWMutex
}

// ETFMetadata holds static per-ETF configuration.
type ETFMetadata struct {
	Name         string
	NAV          float64
	ExpenseRatio float64
	Benchmark    string
}

// NewETFAnalyzer creates a new ETF analyzer with optional metadata and price history.
func NewETFAnalyzer() *ETFAnalyzer {
	return &ETFAnalyzer{metadata: make(map[string]ETFMetadata)}
}

// WithHistoricalPrices attaches historical prices for tracking error computation.
func (ea *ETFAnalyzer) WithHistoricalPrices(hp *HistoricalPrices) *ETFAnalyzer {
	ea.mu.Lock()
	defer ea.mu.Unlock()
	ea.hp = hp
	return ea
}

// LoadMetadata loads ETF metadata from a map (populated from JSON config).
func (ea *ETFAnalyzer) LoadMetadata(data map[string]ETFMetadata) {
	ea.mu.Lock()
	defer ea.mu.Unlock()
	ea.metadata = data
}

// AddMetadata adds a single ETF metadata entry.
func (ea *ETFAnalyzer) AddMetadata(symbol string, m ETFMetadata) {
	ea.mu.Lock()
	defer ea.mu.Unlock()
	ea.metadata[symbol] = m
}

// CalculateETFScore returns a composite ETF factor score with breakdown.
// Uses: PremiumDiscountScore (40%), TrackingErrorScore (30%), ExpenseRatioScore (30%).
func (ea *ETFAnalyzer) CalculateETFScore(symbol string, quote domain.Quote) domain.FactorScoreItem {
	ea.mu.RLock()
	meta, hasMeta := ea.metadata[symbol]
	hp := ea.hp
	ea.mu.RUnlock()

	if !hasMeta {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "etf_score: no metadata available",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}

	if meta.NAV <= 0 || quote.Last <= 0 {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "etf_score: no NAV or price data",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}

	// 1. Premium/Discount Score (40%)
	premDisc := (quote.Last - meta.NAV) / meta.NAV
	pdScore := -premDisc / 0.02 // normalize to [-1,1]; 2% premium → -1, 2% discount → +1
	if pdScore > 1.0 {
		pdScore = 1.0
	}
	if pdScore < -1.0 {
		pdScore = -1.0
	}

	// 2. Tracking Error Score (30%)
	teScore := 0.5 // neutral default
	teAnnual := 0.0
	if hp != nil {
		vol20 := hp.Volatility(symbol, 20)
		if vol20 > 0 {
			// Annualized tracking error proxy: sqrt(252) * 20d vol
			teAnnual = vol20 * math.Sqrt(252)
			teScore = 1.0 - teAnnual/0.03 // 3% = 0 score, 0% = 1 score
			if teScore > 1.0 {
				teScore = 1.0
			}
			if teScore < -1.0 {
				teScore = -1.0
			}
		}
	}

	// 3. Expense Ratio Score (30%)
	erScore := 1.0 - meta.ExpenseRatio/0.015 // 1.5% = 0, 0% = 1
	if erScore > 1.0 {
		erScore = 1.0
	}
	if erScore < -1.0 {
		erScore = -1.0
	}

	// Composite
	composite := 0.40*pdScore + 0.30*teScore + 0.30*erScore

	return domain.FactorScoreItem{
		Score:   composite,
		Formula: fmt.Sprintf("0.40*PremiumDiscountScore(%.3f) + 0.30*TrackingErrorScore(%.3f) + 0.30*ExpenseRatioScore(%.3f)", pdScore, teScore, erScore),
		RawInputs: map[string]float64{
			"nav":               meta.NAV,
			"market_price":      quote.Last,
			"premium_discount":  premDisc,
			"pd_score":          pdScore,
			"tracking_error_an": teAnnual,
			"te_score":          teScore,
			"expense_ratio":     meta.ExpenseRatio,
			"er_score":          erScore,
			"composite":         composite,
		},
	}
}

// IsETF checks if a symbol has ETF metadata loaded.
func (ea *ETFAnalyzer) IsETF(symbol string) bool {
	ea.mu.RLock()
	defer ea.mu.RUnlock()
	_, ok := ea.metadata[symbol]
	return ok
}

// GetNAV returns the current NAV for a symbol, or 0 if not found.
func (ea *ETFAnalyzer) GetNAV(symbol string) float64 {
	ea.mu.RLock()
	defer ea.mu.RUnlock()
	meta, ok := ea.metadata[symbol]
	if !ok {
		return 0
	}
	return meta.NAV
}

// SymbolsWithZeroNAV returns all ETF symbols whose NAV is zero (never populated).
func (ea *ETFAnalyzer) SymbolsWithZeroNAV() []string {
	ea.mu.RLock()
	defer ea.mu.RUnlock()
	var symbols []string
	for sym, meta := range ea.metadata {
		if meta.NAV <= 0 {
			symbols = append(symbols, sym)
		}
	}
	return symbols
}

// RefreshNAV updates the NAV for a single symbol. Thread-safe.
// Returns false if the symbol has no metadata.
func (ea *ETFAnalyzer) RefreshNAV(symbol string, nav float64) bool {
	ea.mu.Lock()
	defer ea.mu.Unlock()
	meta, ok := ea.metadata[symbol]
	if !ok {
		return false
	}
	meta.NAV = nav
	ea.metadata[symbol] = meta
	return true
}

// UpdateNAVFromQuotes updates ETF NAVs using market quotes (Last price)
// as a NAV proxy. This is a fallback when real NAV data is unavailable
// and is reasonable since most ETFs trade within 0.5% of NAV.
func (ea *ETFAnalyzer) UpdateNAVFromQuotes(quotes []domain.Quote) int {
	ea.mu.Lock()
	defer ea.mu.Unlock()

	updated := 0
	for _, q := range quotes {
		if q.Last <= 0 {
			continue
		}
		meta, ok := ea.metadata[q.Symbol]
		if !ok {
			continue
		}
		meta.NAV = q.Last
		ea.metadata[q.Symbol] = meta
		updated++
	}
	return updated
}

// ETFNAVFetcher fetches NAV for a given ETF symbol.
type ETFNAVFetcher interface {
	FetchNAV(ctx context.Context, symbol string) (float64, error)
}

// RefreshNAVFromFetcher fetches NAV data for all tracked symbols using a fetcher.
// Symbols with existing non-zero NAV are skipped by default; pass force=true
// to refresh all symbols. Returns the count of successfully updated symbols.
func (ea *ETFAnalyzer) RefreshNAVFromFetcher(ctx context.Context, fetcher ETFNAVFetcher, force bool) int {
	symbols := ea.AllSymbols()
	updated := 0

	for _, sym := range symbols {
		if !force && ea.GetNAV(sym) > 0 {
			continue
		}
		nav, err := fetcher.FetchNAV(ctx, sym)
		if err != nil {
			continue
		}
		if ea.RefreshNAV(sym, nav) {
			updated++
		}
	}
	return updated
}

// AllSymbols returns all ETF symbols tracked by this analyzer.
func (ea *ETFAnalyzer) AllSymbols() []string {
	ea.mu.RLock()
	defer ea.mu.RUnlock()
	symbols := make([]string, 0, len(ea.metadata))
	for sym := range ea.metadata {
		symbols = append(symbols, sym)
	}
	return symbols
}
