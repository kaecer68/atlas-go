package janus

import (
	"math"

	"github.com/kaecer68/atlas-go/internal/prism"
)

// RegimeDetector compares short-window and long-window cohort weight distributions
// to produce an emergent regime classification.
type RegimeDetector struct {
	config JANUSConfig
}

// NewRegimeDetector creates a detector with the supplied configuration.
func NewRegimeDetector(config JANUSConfig) *RegimeDetector {
	return &RegimeDetector{config: config}
}

// Detect compares short-window cohort weights against long-window weights.
//
// Algorithm:
//   1. Find the best-performing cohort in the short window (top weight).
//   2. Find the best-performing cohort in the long window (top weight).
//   3. Measure the delta between short and long weights for the short winner.
//   4. If the short winner has a meaningfully higher weight in the short window
//      than in the long window => NOVEL_REGIME.
//   5. If the long winner dominates in the long window and the short window
//      looks similar => HISTORICAL_REGIME.
//   6. Otherwise => MIXED.
func (d *RegimeDetector) Detect(
	shortWeights map[prism.RegimeType]CohortWeight,
	longWeights map[prism.RegimeType]CohortWeight,
) RegimeClassification {
	if len(shortWeights) == 0 || len(longWeights) == 0 {
		return MixedRegime
	}

	shortBest, shortBestWeight := d.findBest(shortWeights)
	longBest, longBestWeight := d.findBest(longWeights)

	// Edge case: identical empty sets or unable to resolve best.
	if shortBest == -1 || longBest == -1 {
		return MixedRegime
	}

	// Retrieve the short-best cohort's weight in the long window.
	longWeightForShortBest := 0.0
	if cw, ok := longWeights[shortBest]; ok {
		longWeightForShortBest = cw.Weight
	}

	// Retrieve the long-best cohort's weight in the short window.
	shortWeightForLongBest := 0.0
	if cw, ok := shortWeights[longBest]; ok {
		shortWeightForLongBest = cw.Weight
	}

	// Compute deltas.
	shortDelta := shortBestWeight - longWeightForShortBest
	longDelta := longBestWeight - shortWeightForLongBest

	// Novel regime: short-window winner surges relative to its long-window standing.
	// This implies recent accuracy is coming from a cohort that was not historically dominant.
	if shortDelta > d.config.NovelThreshold && shortBest != longBest {
		return NovelRegime
	}

	// Historical regime: long-window winner maintains a strong, stable lead.
	// We also require the long winner to be the same in both windows or at least
	// the long window to be much more concentrated than the short window.
	if longDelta > d.config.HistoricalThreshold && longBestWeight >= shortBestWeight {
		return HistoricalRegime
	}

	// If both top cohorts are the same and deltas are small, the regime is stable/mixed.
	if shortBest == longBest && math.Abs(shortDelta) < d.config.NovelThreshold {
		return HistoricalRegime
	}

	return MixedRegime
}

// findBest returns the regime with the highest weight and that weight value.
func (d *RegimeDetector) findBest(weights map[prism.RegimeType]CohortWeight) (prism.RegimeType, float64) {
	var bestRegime prism.RegimeType = -1
	bestWeight := -1.0

	for regime, cw := range weights {
		if cw.Weight > bestWeight {
			bestWeight = cw.Weight
			bestRegime = regime
		}
	}

	return bestRegime, bestWeight
}
