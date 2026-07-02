package macroflow

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// Engine computes macro→factor-weight adjustments from market data + risk level.
type Engine struct {
	maxStaleAge time.Duration // max age of snapshot data before it's stale
	nowFn       func() time.Time
}

// NewEngine creates a macroflow engine. maxStaleAge defaults to 7 days when ≤ 0.
func NewEngine(maxStaleAge time.Duration) *Engine {
	if maxStaleAge <= 0 {
		maxStaleAge = 7 * 24 * time.Hour
	}
	return &Engine{
		maxStaleAge: maxStaleAge,
		nowFn:       time.Now,
	}
}

// Compute runs the macroflow rules and returns the combined adjustment.
// Returns nil result if data is stale, snapshot is nil, or level is unknown.
func (e *Engine) Compute(snapshot *marketdata.MacroDataSnapshot, level RiskLevel) *AdjustmentResult {
	if snapshot == nil {
		return nil
	}
	if e.isStale(snapshot) {
		return nil
	}

	stress := e.isStressful(snapshot)
	rules := evaluateRules(level, stress)
	if len(rules) == 0 {
		return nil
	}
	adj := combineAdjustments(rules)
	reasoning := make([]string, len(rules))
	for i, r := range rules {
		reasoning[i] = r.Reason
	}

	return &AdjustmentResult{
		RiskLevel:  level,
		IsStress:   stress,
		Adjustment: adj,
		Reasoning:  reasoning,
	}
}

// isStale checks if snapshot RecordedAt is older than maxStaleAge.
func (e *Engine) isStale(snapshot *marketdata.MacroDataSnapshot) bool {
	recorded := time.Unix(snapshot.RecordedAt, 0)
	return e.nowFn().Sub(recorded) > e.maxStaleAge
}

// isStressful returns true when VIX >= 35.
func (e *Engine) isStressful(snapshot *marketdata.MacroDataSnapshot) bool {
	return snapshot.VIX.Value >= 35
}