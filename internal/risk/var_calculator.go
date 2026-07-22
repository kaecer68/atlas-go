package risk

import (
	"math"
	"sort"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// MinObservationsForVaR is the minimum number of daily return observations
// required for a statistically meaningful VaR calculation (1 trading year).
const MinObservationsForVaR = 252

type VaRCalculator struct {
	primaryConfidence   float64
	secondaryConfidence float64
}

func NewVaRCalculator() *VaRCalculator {
	cfg := config.GetParametersConfig()
	return &VaRCalculator{
		primaryConfidence:   cfg.Risk.VaRConfidenceLevel.Value,
		secondaryConfidence: cfg.Risk.VaRSecondaryConfidence.Value,
	}
}

func (c *VaRCalculator) ComputeRiskSnapshot(dailyReturns []float64, portfolioValues []float64) domain.RiskSnapshot {
	return domain.RiskSnapshot{
		VaR95:          CalculateVaR(dailyReturns, c.primaryConfidence),
		VaR99:          CalculateVaR(dailyReturns, c.secondaryConfidence),
		CVaR95:         CalculateCVaR(dailyReturns, c.primaryConfidence),
		MaxDrawdownPct: CalculateMaxDrawdown(portfolioValues),
	}
}

// ComputeComponentVaR decomposes portfolio VaR into per-asset contributions
// using the calculator's primary confidence level.
func (c *VaRCalculator) ComputeComponentVaR(returns map[string][]float64, weights map[string]float64) []ComponentVaRItem {
	return CalculateComponentVaR(returns, weights, c.primaryConfidence)
}

// CalculateVaR computes historical VaR from a series of daily returns.
// confidence should be 0.95 or 0.99. Returns 0.0 when the series has fewer
// than MinObservationsForVaR entries (252 = 1 trading year).
func CalculateVaR(dailyReturns []float64, confidence float64) float64 {
	if len(dailyReturns) < MinObservationsForVaR {
		return 0.0
	}
	return CalculateVaRPercentile(dailyReturns, confidence)
}

// CalculateVaRPercentile computes historical VaR as the (1-confidence)
// percentile of daily returns WITHOUT a minimum-observation guard.
// Callers must enforce their own sample-size threshold. Use this when
// the canonical 252-day gate is inappropriate (e.g., backtest signals
// with shorter windows — see #1265 canonical metric source).
func CalculateVaRPercentile(dailyReturns []float64, confidence float64) float64 {
	sorted := make([]float64, len(dailyReturns))
	copy(sorted, dailyReturns)
	sort.Float64s(sorted)
	index := max(int(math.Floor((1.0-confidence)*float64(len(sorted)))), 0)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// CalculateCVaR computes Conditional VaR (Expected Shortfall) as the average
// of returns worse than the VaR threshold.
func CalculateCVaR(dailyReturns []float64, confidence float64) float64 {
	if len(dailyReturns) < MinObservationsForVaR {
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
// series of portfolio values. Returns a positive magnitude (e.g., 0.20 = 20% drawdown).
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
// and portfolio value history using configurable VaR confidence levels.
func ComputeRiskSnapshot(dailyReturns []float64, portfolioValues []float64) domain.RiskSnapshot {
	calculator := NewVaRCalculator()
	return calculator.ComputeRiskSnapshot(dailyReturns, portfolioValues)
}

// ComponentVaRItem holds the component VaR breakdown for a single asset.
type ComponentVaRItem struct {
	Symbol          string  `json:"symbol"`
	Weight          float64 `json:"weight"`
	StandaloneVaR   float64 `json:"standalone_var"`
	ComponentVaR    float64 `json:"component_var"`
	PctContribution float64 `json:"pct_contribution"`
}

// CalculateComponentVaR decomposes portfolio VaR into per-asset contributions
// via historical simulation: CVaR_i = w_i × E[R_i | R_p ≤ VaR_p].
//
// returns maps symbol → daily return series.
// weights maps symbol → portfolio weight (should sum to ~1.0).
// confidence is the VaR confidence level (e.g., 0.95 or 0.99).
func CalculateComponentVaR(returns map[string][]float64, weights map[string]float64, confidence float64) []ComponentVaRItem {
	if len(returns) == 0 || len(weights) == 0 {
		return nil
	}

	minLen := -1
	for _, r := range returns {
		if minLen == -1 || len(r) < minLen {
			minLen = len(r)
		}
	}
	if minLen < MinObservationsForVaR {
		return nil
	}

	symbols := make([]string, 0, len(returns))
	for sym := range returns {
		if _, ok := weights[sym]; ok {
			symbols = append(symbols, sym)
		}
	}

	portReturns := make([]float64, minLen)
	for t := 0; t < minLen; t++ {
		var rp float64
		for _, sym := range symbols {
			rp += weights[sym] * returns[sym][t]
		}
		portReturns[t] = rp
	}

	sorted := make([]float64, len(portReturns))
	copy(sorted, portReturns)
	sort.Float64s(sorted)

	varIndex := max(int(math.Floor((1.0-confidence)*float64(len(sorted)))), 0)
	if varIndex >= len(sorted) {
		varIndex = len(sorted) - 1
	}
	varThreshold := sorted[varIndex]

	items := make([]ComponentVaRItem, 0, len(symbols))
	for _, sym := range symbols {
		w := weights[sym]
		assetReturns := returns[sym][:minLen]
		standaloneVaR := CalculateVaR(assetReturns, confidence)

		var marginalSum float64
		var marginalCount int
		for t := 0; t < minLen; t++ {
			if portReturns[t] <= varThreshold {
				marginalSum += assetReturns[t]
				marginalCount++
			}
		}

		var componentVaR float64
		if marginalCount > 0 {
			componentVaR = w * marginalSum / float64(marginalCount)
		}

		items = append(items, ComponentVaRItem{
			Symbol:        sym,
			Weight:        w,
			StandaloneVaR: standaloneVaR,
			ComponentVaR:  componentVaR,
		})
	}

	absSum := 0.0
	for _, item := range items {
		absSum += math.Abs(item.ComponentVaR)
	}
	if absSum > 0 {
		for i := range items {
			items[i].PctContribution = math.Abs(items[i].ComponentVaR) / absSum
		}
	}

	return items
}
