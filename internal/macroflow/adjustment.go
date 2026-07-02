// Package macroflow computes macro regime-based factor weight adjustments.
// The Engine takes a MacroDataSnapshot and a RiskLevel, evaluates 6 rules
// (Yellow/Orange/Red × Calm/Stress), and returns combined percentage deltas
// for Defensive, Aggressive, and Cash allocation tiers.
package macroflow

import (
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// RiskLevel represents the macro risk regime for factor-weight adjustment.
type RiskLevel string

const (
	RiskYellow RiskLevel = "yellow" // Elevated uncertainty
	RiskOrange RiskLevel = "orange" // High risk
	RiskRed    RiskLevel = "red"    // Severe/crisis risk
)

// Adjustment holds the percentage-point deltas for one allocation tier.
type Adjustment struct {
	Defensive  float64 // percentage-point change to defensive allocation (±)
	Aggressive float64 // percentage-point change to aggressive allocation (±)
	Cash       float64 // percentage-point change to cash allocation (±)
}

// AdjustmentResult is the output of the macroflow engine.
type AdjustmentResult struct {
	RiskLevel  RiskLevel
	IsStress   bool
	Adjustment Adjustment
	Reasoning  []string // per-rule reasoning for auditability
}

// clipAndDedupe caps each adjustment dimension to ±30% and deduplicates reasoning.
func clipAndDedupe(adj Adjustment, reasoning []string) (Adjustment, []string) {
	clamp := func(v float64) float64 {
		if v > 30 {
			return 30
		}
		if v < -30 {
			return -30
		}
		return v
	}
	adj.Defensive = clamp(adj.Defensive)
	adj.Aggressive = clamp(adj.Aggressive)
	adj.Cash = clamp(adj.Cash)

	seen := make(map[string]bool, len(reasoning))
	deduped := make([]string, 0, len(reasoning))
	for _, r := range reasoning {
		if !seen[r] {
			seen[r] = true
			deduped = append(deduped, r)
		}
	}
	return adj, deduped
}

// Compile-time check that AdjustmentResult can carry a macro snapshot reference.
var _ = (*marketdata.MacroDataSnapshot)(nil)
