package shared

import "math"

// SortinoConfig configures ComputeSortino. Use Frequency to select the
// annualization convention; MinSamples guards against statistically
// meaningless estimates. RiskFreeRate is the per-period threshold below
// which a return contributes to the downside deviation (typically 0 for
// simple excess-return Sortino, or the risk-free rate for an alternative-
// free Sortino).
type SortinoConfig struct {
	Frequency    Frequency
	RiskFreeRate float64
	MinSamples   int
}

// ComputeSortino returns the annualized Sortino ratio for the given return
// series. Unlike Sharpe, only returns strictly below RiskFreeRate contribute
// to the downside deviation, so positive volatility is not penalized.
//
// Returns 0 when:
//   - the series has fewer than MinSamples entries
//   - the series is empty
//   - no return falls below RiskFreeRate (no downside observations)
//
// The mean uses an arithmetic (N) denominator; the downside deviation uses
// the population (N) denominator of squared negative excesses — this matches
// the legacy behavior of internal/reporting.calculateSortinoRatio that this
// function replaces, so callers see identical numeric output.
//
// Formula: (mean_return - risk_free_rate) / downside_dev * sqrt(annualization_factor)
// where downside_dev = sqrt(mean of squared (r - risk_free_rate) for r < risk_free_rate)
func ComputeSortino(returns []float64, cfg SortinoConfig) float64 {
	if len(returns) < cfg.MinSamples {
		return 0
	}

	var excessSum, downsideSum float64
	for _, r := range returns {
		excess := r - cfg.RiskFreeRate
		excessSum += excess
		if r < cfg.RiskFreeRate {
			downsideSum += excess * excess
		}
	}
	meanExcess := excessSum / float64(len(returns))

	downsideDev := 0.0
	if downsideSum > 0 {
		downsideDev = math.Sqrt(downsideSum / float64(len(returns)))
	}
	if downsideDev == 0 {
		return 0
	}

	sortino := (meanExcess / downsideDev)
	switch cfg.Frequency {
	case FrequencyPerDay:
		sortino *= math.Sqrt(252)
	case FrequencyTWSE:
		sortino *= math.Sqrt(243)
	}
	return sortino
}
