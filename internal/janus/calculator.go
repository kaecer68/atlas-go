package janus

import (
	"math"

	"github.com/kaecer68/atlas-go/internal/prism"
)

// CohortWeightCalculator converts windowed cohort performance into dynamic weights.
type CohortWeightCalculator struct {
	config JANUSConfig
}

// NewCohortWeightCalculator creates a calculator with the given config.
func NewCohortWeightCalculator(config JANUSConfig) *CohortWeightCalculator {
	return &CohortWeightCalculator{config: config}
}

// CalculateWeights computes JANUS weights for all provided cohort performances.
// Weights sum to 1.0 and respect MinWeight / MaxWeight bounds.
func (c *CohortWeightCalculator) CalculateWeights(
	performances map[prism.RegimeType]*CohortPerformance,
) map[prism.RegimeType]CohortWeight {
	if len(performances) == 0 {
		return nil
	}

	// Use a blended score: 50% short + 30% medium + 20% long, all based on Sharpe.
	// This emphasizes recent accuracy while retaining long-term memory.
	rawScores := make(map[prism.RegimeType]float64)
	for regime, perf := range performances {
		if perf == nil {
			continue
		}
		score := c.blendedSharpeScore(perf)
		rawScores[regime] = score
	}

	// Handle edge case where all scores are negative or zero.
	allNegativeOrZero := true
	positiveSum := 0.0
	for _, score := range rawScores {
		if score > 0 {
			allNegativeOrZero = false
			positiveSum += score
		}
	}

	weights := make(map[prism.RegimeType]CohortWeight)
	if allNegativeOrZero {
		// Equal weight fallback when no cohort has positive Sharpe.
		equalWeight := 1.0 / float64(len(rawScores))
		for regime := range rawScores {
			weights[regime] = CohortWeight{
				Regime: regime,
				Weight: c.clamp(equalWeight),
			}
		}
		return weights
	}

	// Positive-score cohorts get proportionally higher weight;
	// negative-score cohorts get epsilon.
	for regime, score := range rawScores {
		var w float64
		if score > 0 {
			w = score / positiveSum
			// Scale so that total mass reserved for positive cohorts is
			// (1 - epsilon*negCount). For simplicity we assign epsilon to
			// negatives and re-normalize afterwards.
		} else {
			w = c.config.EpsilonWeight
		}
		weights[regime] = CohortWeight{
			Regime: regime,
			Weight: c.clamp(w),
		}
	}

	// Normalize to sum 1.0 after clamping.
	weights = c.normalize(weights)
	return weights
}

// CalculateWindowWeights computes weights using *only* the specified lookback window.
// Useful for the RegimeDetector's short-vs-long comparison.
func (c *CohortWeightCalculator) CalculateWindowWeights(
	performances map[prism.RegimeType]*CohortPerformance,
	window PerformanceWindow,
) map[prism.RegimeType]CohortWeight {
	if len(performances) == 0 {
		return nil
	}

	rawScores := make(map[prism.RegimeType]float64)
	for regime, perf := range performances {
		if perf == nil {
			continue
		}
		score := c.windowSharpeScore(perf, window)
		rawScores[regime] = score
	}

	allNegativeOrZero := true
	positiveSum := 0.0
	for _, score := range rawScores {
		if score > 0 {
			allNegativeOrZero = false
			positiveSum += score
		}
	}

	weights := make(map[prism.RegimeType]CohortWeight)
	if allNegativeOrZero {
		equalWeight := 1.0 / float64(len(rawScores))
		for regime := range rawScores {
			weights[regime] = CohortWeight{
				Regime: regime,
				Weight: c.clamp(equalWeight),
			}
		}
		return weights
	}

	for regime, score := range rawScores {
		var w float64
		if score > 0 {
			w = score / positiveSum
		} else {
			w = c.config.EpsilonWeight
		}
		weights[regime] = CohortWeight{
			Regime: regime,
			Weight: c.clamp(w),
		}
	}

	weights = c.normalize(weights)
	return weights
}

// blendedSharpeScore mixes short/medium/long Sharpe into a single scalar.
func (c *CohortWeightCalculator) blendedSharpeScore(perf *CohortPerformance) float64 {
	short := c.safeSharpe(perf.ShortWindow)
	med := c.safeSharpe(perf.MedWindow)
	long := c.safeSharpe(perf.LongWindow)
	return short*0.5 + med*0.3 + long*0.2
}

// windowSharpeScore extracts Sharpe for a specific window.
func (c *CohortWeightCalculator) windowSharpeScore(perf *CohortPerformance, window PerformanceWindow) float64 {
	switch window {
	case WindowShort:
		return c.safeSharpe(perf.ShortWindow)
	case WindowMedium:
		return c.safeSharpe(perf.MedWindow)
	case WindowLong:
		return c.safeSharpe(perf.LongWindow)
	default:
		return c.blendedSharpeScore(perf)
	}
}

func (c *CohortWeightCalculator) safeSharpe(wp *WindowPerformance) float64 {
	if wp == nil {
		return 0
	}
	return wp.SharpeRatio
}

func (c *CohortWeightCalculator) clamp(w float64) float64 {
	if w < c.config.MinWeight {
		return c.config.MinWeight
	}
	if w > c.config.MaxWeight {
		return c.config.MaxWeight
	}
	return w
}

func (c *CohortWeightCalculator) normalize(weights map[prism.RegimeType]CohortWeight) map[prism.RegimeType]CohortWeight {
	total := 0.0
	for _, cw := range weights {
		total += cw.Weight
	}
	if total == 0 || math.Abs(total-1.0) < 1e-9 {
		return weights
	}

	normalized := make(map[prism.RegimeType]CohortWeight, len(weights))
	for regime, cw := range weights {
		cw.Weight = cw.Weight / total
		normalized[regime] = cw
	}
	return normalized
}
