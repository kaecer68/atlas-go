package robustness

import (
	"math"
	"sort"

	"github.com/kaecer68/atlas-go/internal/eval"
)

// ExclusionReport contains the results of penny stock exclusion analysis.
type ExclusionReport struct {
	Baseline            *eval.EvalResult
	Excluded            *eval.EvalResult
	ExcludedSymbols     []string
	PercentileThreshold float64
	DegradationPct      float64 // positive = degradation, negative = improvement
}

// ExcludePennyStocks evaluates the impact of removing low-price stocks from a strategy.
//
// Prices is a map of symbol → price. percentileThreshold determines the cutoff
// (e.g., 0.20 excludes the bottom 20% by price). strategyFunc returns daily returns
// for a given symbol.
//
// Baseline eval uses all symbols. Excluded eval uses only symbols above the threshold.
// DegradationPct measures how much cumulative return is lost by excluding penny stocks:
//
//	((baseline.CumReturn - excluded.CumReturn) / |baseline.CumReturn|) * 100
func ExcludePennyStocks(prices map[string]float64, percentileThreshold float64, strategyFunc func(symbol string) []float64) ExclusionReport {
	// Collect all symbols and prices
	type symPrice struct {
		symbol string
		price  float64
	}
	var allSymbols []symPrice
	for sym, price := range prices {
		allSymbols = append(allSymbols, symPrice{symbol: sym, price: price})
	}

	if len(allSymbols) == 0 {
		return ExclusionReport{
			Baseline:            &eval.EvalResult{},
			Excluded:            &eval.EvalResult{},
			PercentileThreshold: percentileThreshold,
		}
	}

	// Sort by price ascending
	sort.Slice(allSymbols, func(i, j int) bool {
		return allSymbols[i].price < allSymbols[j].price
	})

	// Compute how many symbols to exclude: bottom (percentileThreshold * N) symbols
	excludeCount := int(math.Floor(float64(len(allSymbols)) * percentileThreshold))
	if excludeCount < 0 {
		excludeCount = 0
	}
	if excludeCount > len(allSymbols) {
		excludeCount = len(allSymbols)
	}

	// Split into excluded (cheapest n) and kept (rest)
	var excludedSymbols []string
	var keptSymbols []symPrice
	for i, sp := range allSymbols {
		if i < excludeCount {
			excludedSymbols = append(excludedSymbols, sp.symbol)
		} else {
			keptSymbols = append(keptSymbols, sp)
		}
	}

	// Collect returns for baseline (all symbols) and excluded (kept only)
	var allReturns []float64
	for _, sp := range allSymbols {
		returns := strategyFunc(sp.symbol)
		allReturns = append(allReturns, returns...)
	}

	var keptReturns []float64
	for _, sp := range keptSymbols {
		returns := strategyFunc(sp.symbol)
		keptReturns = append(keptReturns, returns...)
	}

	baseline := computeEvalFromReturns(allReturns)
	excluded := computeEvalFromReturns(keptReturns)

	// Calculate degradation percentage
	var degradationPct float64
	if math.Abs(baseline.CumReturn) > 1e-15 {
		degradationPct = ((baseline.CumReturn - excluded.CumReturn) / math.Abs(baseline.CumReturn)) * 100
	}

	return ExclusionReport{
		Baseline:            baseline,
		Excluded:            excluded,
		ExcludedSymbols:     excludedSymbols,
		PercentileThreshold: percentileThreshold,
		DegradationPct:      degradationPct,
	}
}
