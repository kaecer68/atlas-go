package swarm

import (
	"fmt"
	"math/rand"
)

// PredictionRule defines a fish's prediction strategy parameters.
// Each fish starts with a randomly-initialized rule and rules
// evolve across generations through mutation and selection.
type PredictionRule struct {
	LookbackWindow     int     `json:"lookback_window"`      // steps to look back for trend (3–20)
	TrendUpThreshold   float64 `json:"trend_up_threshold"`   // price ratio above which → "up" (1.005–1.10)
	TrendDownThreshold float64 `json:"trend_down_threshold"` // price ratio below which → "down" (0.90–0.995)
	UseSentiment       bool    `json:"use_sentiment"`        // whether to factor sentiment into predictions
	ContrarianBias     float64 `json:"contrarian_bias"`      // -1.0 (full contrarian) to 1.0 (full trend-follow)
}

// RandomPredictionRule generates a random rule for initial fish population.
func RandomPredictionRule() PredictionRule {
	return PredictionRule{
		LookbackWindow:     rand.Intn(18) + 3,
		TrendUpThreshold:   1.005 + rand.Float64()*0.095,
		TrendDownThreshold: 0.90 + rand.Float64()*0.095,
		UseSentiment:       rand.Float64() < 0.5,
		ContrarianBias:     rand.Float64()*2.0 - 1.0,
	}
}

// DefaultPredictionRule returns a neutral rule matching the original hardcoded strategy.
// Used for backward compatibility.
func DefaultPredictionRule() PredictionRule {
	return PredictionRule{
		LookbackWindow:     5,
		TrendUpThreshold:   1.02,
		TrendDownThreshold: 0.98,
		UseSentiment:       false,
		ContrarianBias:     0.0,
	}
}

// MutateRule creates a mutated copy of the rule.
// Each field has an independent mutation chance (mutationRate).
// Returns a new PredictionRule (original is unchanged).
func MutateRule(parent PredictionRule, mutationRate float64) PredictionRule {
	if mutationRate <= 0 {
		mutationRate = 0.15
	}

	child := parent

	if rand.Float64() < mutationRate {
		delta := rand.Intn(5) - 2 // -2 to +2
		child.LookbackWindow = clampInt(parent.LookbackWindow+delta, 3, 20)
	}
	if rand.Float64() < mutationRate {
		delta := rand.Float64()*0.04 - 0.02
		child.TrendUpThreshold = clampFloat(parent.TrendUpThreshold+delta, 1.005, 1.10)
	}
	if rand.Float64() < mutationRate {
		delta := rand.Float64()*0.04 - 0.02
		child.TrendDownThreshold = clampFloat(parent.TrendDownThreshold+delta, 0.90, 0.995)
	}
	if rand.Float64() < mutationRate {
		child.UseSentiment = !parent.UseSentiment
	}
	if rand.Float64() < mutationRate {
		delta := rand.Float64()*0.6 - 0.3
		child.ContrarianBias = clampFloat(parent.ContrarianBias+delta, -1.0, 1.0)
	}

	return child
}

// CrossoverRules creates a child rule by randomly selecting parameters from two parents.
func CrossoverRules(parent1, parent2 PredictionRule) PredictionRule {
	child := PredictionRule{}
	if rand.Float64() < 0.5 {
		child.LookbackWindow = parent1.LookbackWindow
	} else {
		child.LookbackWindow = parent2.LookbackWindow
	}
	if rand.Float64() < 0.5 {
		child.TrendUpThreshold = parent1.TrendUpThreshold
	} else {
		child.TrendUpThreshold = parent2.TrendUpThreshold
	}
	if rand.Float64() < 0.5 {
		child.TrendDownThreshold = parent1.TrendDownThreshold
	} else {
		child.TrendDownThreshold = parent2.TrendDownThreshold
	}
	if rand.Float64() < 0.5 {
		child.UseSentiment = parent1.UseSentiment
	} else {
		child.UseSentiment = parent2.UseSentiment
	}
	// Blend contrarian bias
	child.ContrarianBias = clampFloat((parent1.ContrarianBias+parent2.ContrarianBias)/2.0, -1.0, 1.0)
	return child
}

// RuleSummary returns a human-readable summary of the rule.
func (r PredictionRule) RuleSummary() string {
	return fmt.Sprintf("LB=%d,Up=%.4f,Down=%.4f,Sent=%v,Contr=%.2f",
		r.LookbackWindow, r.TrendUpThreshold, r.TrendDownThreshold, r.UseSentiment, r.ContrarianBias)
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
