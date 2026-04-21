package risk

import (
	"math"
	"sort"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CalculateVaR computes historical VaR and CVaR from a series of daily returns.
// confidence should be 0.95 or 0.99.
func CalculateVaR(dailyReturns []float64, confidence float64) float64 {
	if len(dailyReturns) == 0 {
		return 0.0
	}
	sorted := make([]float64, len(dailyReturns))
	copy(sorted, dailyReturns)
	sort.Float64s(sorted)

	index := int(math.Floor((1.0 - confidence) * float64(len(sorted))))
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// CalculateCVaR computes Conditional VaR (Expected Shortfall) as the average
// of returns worse than the VaR threshold.
func CalculateCVaR(dailyReturns []float64, confidence float64) float64 {
	if len(dailyReturns) == 0 {
		return 0.0
	}
	varThreshold := CalculateVaR(dailyReturns, confidence)
	var sum float64
	var count int
	for _, r := range dailyReturns {
		if r <= varThreshold {
			sum += r
			count++
		}
	}
	if count == 0 {
		return varThreshold
	}
	return sum / float64(count)
}

// CalculateMaxDrawdown computes the maximum peak-to-trough decline from a
// series of portfolio values.
func CalculateMaxDrawdown(values []float64) float64 {
	if len(values) == 0 {
		return 0.0
	}
	maxValue := values[0]
	maxDrawdown := 0.0
	for _, v := range values {
		if v > maxValue {
			maxValue = v
		}
		decline := (maxValue - v) / maxValue
		if decline > maxDrawdown {
			maxDrawdown = decline
		}
	}
	return maxDrawdown
}

// ComputeRiskSnapshot calculates all risk metrics from historical daily returns
// and portfolio value history.
func ComputeRiskSnapshot(dailyReturns []float64, portfolioValues []float64) domain.RiskSnapshot {
	return domain.RiskSnapshot{
		VaR95:          CalculateVaR(dailyReturns, 0.95),
		VaR99:          CalculateVaR(dailyReturns, 0.99),
		CVaR95:         CalculateCVaR(dailyReturns, 0.95),
		MaxDrawdownPct: CalculateMaxDrawdown(portfolioValues),
	}
}
