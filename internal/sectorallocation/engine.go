package sectorallocation

import (
	"context"
	"time"
)

// WeightEngine is the core interface for computing sector weights.
// It combines multiple input factors to derive final industry weights.
type WeightEngine interface {
	// ComputeWeights computes weights for all industries.
	ComputeWeights(ctx context.Context, now time.Time) ([]SectorWeight, error)
	// ComputeWeight computes weight for a single industry.
	ComputeWeight(ctx context.Context, industryID string, now time.Time) (*SectorWeight, error)
}

// CycleInputProvider provides cycle-based weight multipliers.
type CycleInputProvider interface {
	GetCycleMultiplier(ctx context.Context, industryID string) (float64, error)
}

// SeasonalInputProvider provides seasonal-based weight multipliers.
type SeasonalInputProvider interface {
	GetSeasonalMultiplier(ctx context.Context, industryID string, now time.Time) (float64, error)
}

// LinkageInputProvider provides supply chain linkage multipliers.
type LinkageInputProvider interface {
	GetLinkageMultiplier(ctx context.Context, industryID string) (float64, error)
}

// NarrativeInputProvider provides narrative-based multipliers.
type NarrativeInputProvider interface {
	GetNarrativeMultiplier(ctx context.Context, industryID string) (float64, error)
}

// MacroInputProvider provides macro-level tilt adjustments.
type MacroInputProvider interface {
	GetMacroTilt(ctx context.Context, industryID, macroLevel, primaryFlow string) (float64, error)
}

// FactorInputProvider provides factor-based tilt adjustments.
type FactorInputProvider interface {
	GetFactorTilt(ctx context.Context, industryID string) (float64, error)
}
