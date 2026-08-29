package eval

import "math"

// EvalResult bundles key out-of-sample evaluation metrics.
type EvalResult struct {
	R2OOS     float64 `json:"r2_oos"`
	Sharpe    float64 `json:"sharpe"`
	CumReturn float64 `json:"cum_return"`
	MaxDD     float64 `json:"max_dd"`
}

// OOSR2 computes the out-of-sample R-squared: 1 - Σ(y-ŷ)² / Σy².
// Returns 0 for empty inputs, length mismatches, or when yTrue is all zeros.
func OOSR2(yTrue, yPred []float64) float64 {
	n := len(yTrue)
	if n == 0 || len(yPred) != n {
		return 0
	}
	var ssRes, ssTot float64
	for i := range n {
		diff := yTrue[i] - yPred[i]
		ssRes += diff * diff
		ssTot += yTrue[i] * yTrue[i]
	}
	if ssTot == 0 {
		return 0
	}
	return 1 - ssRes/ssTot
}

// SharpeRatio computes the annualized Sharpe ratio from daily returns.
// Uses sqrt(252) for annualization. Returns 0 when std dev is effectively zero
// or input is empty.
func SharpeRatio(returns []float64, riskFreeRate float64) float64 {
	n := len(returns)
	if n == 0 {
		return 0
	}
	// Compute mean excess return
	var sum float64
	for _, r := range returns {
		sum += r - riskFreeRate/252
	}
	mean := sum / float64(n)

	// Compute standard deviation
	var sqSum float64
	for _, r := range returns {
		dev := r - (mean + riskFreeRate/252)
		sqSum += dev * dev
	}
	// Use sample std dev when n > 1
	var stdDev float64
	if n > 1 {
		stdDev = math.Sqrt(sqSum / float64(n-1))
	} else {
		stdDev = math.Sqrt(sqSum / float64(n))
	}

	if stdDev < 1e-15 {
		return 0
	}
	return mean * math.Sqrt(252) / stdDev
}

// CumulativeReturn computes the cumulative return: ∏(1+r) - 1.
// Returns 0 for empty input.
func CumulativeReturn(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	prod := 1.0
	for _, r := range returns {
		prod *= (1 + r)
	}
	return prod - 1
}

// MaxDrawdown computes the maximum peak-to-trough drawdown.
// Returns a positive value (e.g., 0.15 means 15% drawdown). Returns 0 for empty input.
func MaxDrawdown(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	cum := 1.0
	peak := 1.0
	maxDD := 0.0
	for _, r := range returns {
		cum *= (1 + r)
		if cum > peak {
			peak = cum
		}
		dd := (peak - cum) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}
