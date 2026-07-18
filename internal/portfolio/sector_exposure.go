package portfolio

import (
	"fmt"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// L1SymbolResolver maps a stock symbol to a canonical L1 SectorID.
// Implementations must be deterministic and non-fuzzy.
type L1SymbolResolver interface {
	ResolveL1(symbol string) (industry.SectorID, bool)
}

// ExposureGap records a single gap in the exposure calculation.
type ExposureGap struct {
	Symbol      string  `json:"symbol"`
	MarketValue float64 `json:"market_value"`
	Reason      string  `json:"reason"`
}

// SectorExposure captures the simulation-closing sector weights calculated
// from positions, quotes, and a L1 symbol resolver.
//
// Invariant: Weights always has exactly 20 keys (one per canonical L1).
// When the portfolio is empty, all values are zero and Complete is true.
// When any position's symbol cannot be mapped to an L1, Complete is false.
type SectorExposure struct {
	AsOfTradingDate  string                        `json:"as_of_trading_date"`
	Weights          map[industry.SectorID]float64 `json:"weights"`
	TotalMarketValue float64                       `json:"total_market_value"`
	Complete         bool                          `json:"complete"`
	UnmappedSymbols  []string                      `json:"unmapped_symbols,omitempty"`
	UnmappedWeight   float64                       `json:"unmapped_weight,omitempty"`
	UnpricedSymbols  []string                      `json:"unpriced_symbols,omitempty"`
	Gaps             []ExposureGap                 `json:"gaps,omitempty"`
	PositionSource   string                        `json:"position_source"`
	PriceSource      string                        `json:"price_source"`
}

// SectorExposureCalculator computes the current sector exposure from simulation
// closing positions, T-day closing quotes, and a symbol→L1 mapper.
type SectorExposureCalculator struct{}

// Calculate produces a SectorExposure snapshot.
//
// Rules:
//   - SA-INV-10: Exposure = quantity × quote.Last (not AverageCost, not MarketValue).
//   - SA-INV-01: Exactly 20 L1 keys in Weights, every time.
//   - SA-INV-15: Unmapped positive-quantity positions → Complete=false.
//   - SA-INV-15: Missing T-price → Complete=false, symbol in UnpricedSymbols.
//   - SA-INV-19: Quote.AsOf != asOf → fail closed (Complete=false).
//   - SA-INV-15: Empty portfolio returns Complete=true, all-zeros weights.
func (c *SectorExposureCalculator) Calculate(
	positions []domain.Position,
	quotes []domain.Quote,
	asOf time.Time,
	resolver L1SymbolResolver,
) SectorExposure {
	// Initialize exactly 20 L1 keys.
	weights := make(map[industry.SectorID]float64, 20)
	for _, id := range industry.L1Sectors() {
		weights[id] = 0
	}

	// Build quote map; verify date alignment.
	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		dateOnly := q.AsOf.Truncate(24 * time.Hour)
		asOfOnly := asOf.Truncate(24 * time.Hour)
		if !dateOnly.Equal(asOfOnly) {
			return SectorExposure{
				AsOfTradingDate: asOf.Format("2006-01-02"),
				Weights:         weights,
				Complete:        false,
				Gaps: []ExposureGap{{
					Symbol:      q.Symbol,
					MarketValue: 0,
					Reason:      fmt.Sprintf("date_mismatch: quote_date=%s as_of=%s", dateOnly.Format("2006-01-02"), asOfOnly.Format("2006-01-02")),
				}},
				PositionSource: "simulation_closing_positions",
				PriceSource:    "simulation_session_quotes",
			}
		}
		quoteMap[q.Symbol] = q
	}

	totalValue := 0.0
	var gaps []ExposureGap
	var unmappedSyms []string
	unmappedValue := 0.0
	var unpricedSyms []string

	for _, p := range positions {
		if p.Quantity <= 0 {
			continue
		}
		var l1 industry.SectorID
		var ok bool
		if resolver != nil {
			l1, ok = resolver.ResolveL1(p.Symbol)
		}
		if !ok {
			// Use quantity * price for the gap market value.
			mv := float64(p.Quantity) * p.CurrentPrice
			if mv <= 0 {
				mv = float64(p.Quantity) * p.AverageCost
			}
			unmappedValue += mv
			unmappedSyms = append(unmappedSyms, p.Symbol)
			gaps = append(gaps, ExposureGap{Symbol: p.Symbol, MarketValue: mv, Reason: "unmapped_symbol"})
			continue
		}

		q, hasQuote := quoteMap[p.Symbol]
		if !hasQuote || q.Last <= 0 {
			unpricedSyms = append(unpricedSyms, p.Symbol)
			mv := float64(p.Quantity) * p.CurrentPrice
			if mv <= 0 {
				mv = float64(p.Quantity) * p.AverageCost
			}
			gaps = append(gaps, ExposureGap{Symbol: p.Symbol, MarketValue: mv, Reason: "missing_t_price"})
			continue
		}

		// SA-INV-10: quantity × T-day Last price
		v := float64(p.Quantity) * q.Last
		weights[l1] += v
		totalValue += v
	}

	// Normalize weights.
	denominator := totalValue + unmappedValue
	// Complete only when denominator==0 AND no gaps (truly empty portfolio).
	hasGaps := len(unmappedSyms) > 0 || len(unpricedSyms) > 0
	if denominator == 0 {
		sort.Strings(unmappedSyms)
		sort.Strings(unpricedSyms)
		return SectorExposure{
			AsOfTradingDate:  asOf.Format("2006-01-02"),
			Weights:          weights,
			TotalMarketValue: 0,
			Complete:         !hasGaps,
			UnmappedSymbols:  unmappedSyms,
			UnpricedSymbols:  unpricedSyms,
			Gaps:             gaps,
			PositionSource:   "simulation_closing_positions",
			PriceSource:      "simulation_session_quotes",
		}
	}

	for id := range weights {
		weights[id] = weights[id] / denominator
	}

	unmappedWeight := unmappedValue / denominator
	sort.Strings(unmappedSyms)
	sort.Strings(unpricedSyms)

	complete := unmappedValue == 0 && len(unpricedSyms) == 0

	return SectorExposure{
		AsOfTradingDate:  asOf.Format("2006-01-02"),
		Weights:          weights,
		TotalMarketValue: denominator,
		Complete:         complete,
		UnmappedSymbols:  unmappedSyms,
		UnmappedWeight:   unmappedWeight,
		UnpricedSymbols:  unpricedSyms,
		Gaps:             gaps,
		PositionSource:   "simulation_closing_positions",
		PriceSource:      "simulation_session_quotes",
	}
}
