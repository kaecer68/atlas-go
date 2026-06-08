package portfolio

import "math"

// Frequency selects the annualization convention for Sharpe calculation.
type Frequency string

const (
	// FrequencyPerOutcome treats each entry as a single return observation
	// with no annualization. Used by Scorecard where each entry is a
	// per-outcome forward return.
	FrequencyPerOutcome Frequency = "per_outcome"
	// FrequencyPerDay treats each entry as one trading day and annualizes
	// by sqrt(252). Used by DarwinianWeightManager where DailyReturns is a
	// calendar-aligned daily series.
	FrequencyPerDay Frequency = "per_day"
)

// SharpeConfig configures ComputeSharpe. Use Frequency to select the
// annualization convention; MinSamples guards against statistically
// meaningless estimates; StdDevMeanRatioThreshold enables the IEEE 754
// precision guard for near-constant input series.
type SharpeConfig struct {
	Frequency                Frequency
	MinSamples               int
	StdDevMeanRatioThreshold float64
}

// ComputeSharpe returns the Sharpe ratio for the given return series.
// Negative Sharpe is meaningful (below risk-free rate) and is returned as-is.
// Returns 0 when:
//   - the series has fewer than MinSamples entries
//   - the series is empty
//   - the standard deviation is zero or below StdDevMeanRatioThreshold
//     (IEEE 754 precision edge case: identical values like 0.02 produce
//     non-zero stdDev of ~1e-17 due to float64 representation)
//
// The mean and stdDev use sample (N-1) denominators to match the prior
// internal/ledger and internal/portfolio implementations and to keep all
// downstream T-stat / CI calculations consistent.
func ComputeSharpe(returns []float64, cfg SharpeConfig) float64 {
	if len(returns) < cfg.MinSamples {
		return 0
	}
	mean, stdDev := meanSampleVariance(returns)
	if cfg.StdDevMeanRatioThreshold > 0 && mean != 0 &&
		math.Abs(stdDev/mean) < cfg.StdDevMeanRatioThreshold {
		return 0
	}
	if stdDev == 0 {
		return 0
	}
	sharpe := mean / stdDev
	if cfg.Frequency == FrequencyPerDay {
		sharpe *= math.Sqrt(252)
	}
	return sharpe
}

// meanSampleVariance returns the arithmetic mean and the sample standard
// deviation (N-1 denominator) of the given series. Returns 0,0 for empty
// input so callers can safely divide without an extra length check.
func meanSampleVariance(values []float64) (float64, float64) {
	n := len(values)
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(n)
	if n < 2 {
		return mean, 0
	}
	var sq float64
	for _, v := range values {
		d := v - mean
		sq += d * d
	}
	stdDev := math.Sqrt(sq / float64(n-1))
	return mean, stdDev
}
