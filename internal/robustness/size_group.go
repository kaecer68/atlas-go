package robustness

import (
	"sort"

	"github.com/kaecer68/atlas-go/internal/eval"
)

// SizeGroupData holds market cap and returns data for size group analysis.
type SizeGroupData struct {
	MarketCap map[string]float64   // symbol → market cap
	Returns   map[string][]float64 // symbol → daily returns (unused; strategyFunc provides returns)
}

// SizeGroupReport contains evaluation results for big-cap vs small-cap groups.
type SizeGroupReport struct {
	BigGroup     *eval.EvalResult
	SmallGroup   *eval.EvalResult
	BigSymbols   []string
	SmallSymbols []string
	SplitMethod  string
}

// SizeGroupRobustness splits symbols into big/small groups by market cap and
// evaluates strategy performance independently for each group.
//
// splitMethod: "median" (default) divides by median market cap.
// Unrecognized values fall back to "median".
//
// For each symbol, strategyFunc returns the daily returns for that symbol.
// Returns are aggregated across all symbols in a group and evaluated via
// CumulativeReturn, SharpeRatio, and MaxDrawdown.
func SizeGroupRobustness(data SizeGroupData, strategyFunc func(symbol string) []float64, splitMethod string) SizeGroupReport {
	if splitMethod != "median" {
		splitMethod = "median"
	}

	// Collect symbols and their market caps
	var symbols []symCap
	for sym, cap := range data.MarketCap {
		symbols = append(symbols, symCap{symbol: sym, cap: cap})
	}

	if len(symbols) == 0 {
		return SizeGroupReport{
			BigGroup:    &eval.EvalResult{},
			SmallGroup:  &eval.EvalResult{},
			SplitMethod: splitMethod,
		}
	}

	// Sort by market cap ascending
	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].cap < symbols[j].cap
	})

	// Compute median market cap
	var median float64
	n := len(symbols)
	if n%2 == 1 {
		median = symbols[n/2].cap
	} else {
		median = (symbols[n/2-1].cap + symbols[n/2].cap) / 2
	}

	// Split into big and small groups
	var bigCaps, smallCaps []symCap
	for _, sc := range symbols {
		if sc.cap >= median {
			bigCaps = append(bigCaps, sc)
		} else {
			smallCaps = append(smallCaps, sc)
		}
	}

	// Collect returns for each group
	bigReturns := collectReturns(bigCaps, strategyFunc)
	smallReturns := collectReturns(smallCaps, strategyFunc)

	// Build symbol lists
	bigSymbols := make([]string, len(bigCaps))
	for i, sc := range bigCaps {
		bigSymbols[i] = sc.symbol
	}
	smallSymbols := make([]string, len(smallCaps))
	for i, sc := range smallCaps {
		smallSymbols[i] = sc.symbol
	}

	// Compute eval metrics
	bigEval := computeEvalFromReturns(bigReturns)
	smallEval := computeEvalFromReturns(smallReturns)

	return SizeGroupReport{
		BigGroup:     bigEval,
		SmallGroup:   smallEval,
		BigSymbols:   bigSymbols,
		SmallSymbols: smallSymbols,
		SplitMethod:  splitMethod,
	}
}

// symCap pairs a symbol name with its market cap.
type symCap struct {
	symbol string
	cap    float64
}

// collectReturns aggregates daily returns across all symbols in a group.
func collectReturns(symbols []symCap, strategyFunc func(symbol string) []float64) []float64 {
	var allReturns []float64
	for _, sc := range symbols {
		returns := strategyFunc(sc.symbol)
		allReturns = append(allReturns, returns...)
	}
	return allReturns
}

// computeEvalFromReturns computes evaluation metrics from a slice of returns.
// R2OOS is set to 0 since returns-only data has no prediction vs actual comparison.
func computeEvalFromReturns(returns []float64) *eval.EvalResult {
	return &eval.EvalResult{
		R2OOS:     0,
		Sharpe:    eval.SharpeRatio(returns, 0),
		CumReturn: eval.CumulativeReturn(returns),
		MaxDD:     eval.MaxDrawdown(returns),
	}
}
