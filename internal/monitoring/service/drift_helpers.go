package service

import (
	"context"
	"time"
)

// === Drift Constants ===
const (
	DriftMaxConcentrationThreshold = 0.25
	DriftTurnoverThreshold         = 0.15
	DriftTargetWeightThreshold     = 0.10
	DriftCheckInterval             = 5 * time.Minute
	DriftEventSchemaVerV1          = 1
	DriftEventSchemaVer            = 2
)

// === Drift Reason Constants ===
const (
	ReasonConcentration = "concentration"
	ReasonTurnover      = "turnover"
	ReasonTargetDrift   = "target_drift"
)

// DriftDetector is the public interface for the drift detection service.
type DriftDetector interface {
	Start(ctx context.Context) error
	Stop() error
}

// TargetWeightsProvider returns target portfolio weights keyed by symbol for a given regime.
// Returning nil or an empty map means "no target tracking available" — detector will
// preserve v1 behavior (no target_drift reason emitted).
type TargetWeightsProvider interface {
	GetTargetWeights(regime string) map[string]float64
}

// driftSnapshot captures the current market value and last update time for a symbol.
type driftSnapshot struct {
	value     float64
	updatedAt time.Time
}

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
