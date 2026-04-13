package janus

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/prism"
)

// RegimeClassification is the emergent regime signal derived from cohort weight deltas
type RegimeClassification string

const (
	NovelRegime      RegimeClassification = "NOVEL_REGIME"
	HistoricalRegime RegimeClassification = "HISTORICAL_REGIME"
	MixedRegime      RegimeClassification = "MIXED"
)

// PerformanceWindow defines supported lookback windows for cohort accuracy tracking
type PerformanceWindow string

const (
	WindowShort  PerformanceWindow = "short"  // ~5 trading days
	WindowMedium PerformanceWindow = "medium" // ~20 trading days
	WindowLong   PerformanceWindow = "long"   // ~60 trading days
)

// CohortSnapshot captures a single performance observation for a PRISM cohort
type CohortSnapshot struct {
	Regime      prism.RegimeType
	SharpeRatio float64
	HitRate     float64
	TotalReturn float64
	Signals     int
	RecordedAt  time.Time
}

// CohortPerformance holds rolling-window aggregates for one cohort
type CohortPerformance struct {
	Regime      prism.RegimeType
	ShortWindow *WindowPerformance
	MedWindow   *WindowPerformance
	LongWindow  *WindowPerformance
	LastUpdated time.Time
}

// WindowPerformance holds aggregated metrics for a specific lookback window
type WindowPerformance struct {
	Window       PerformanceWindow
	SharpeRatio  float64
	HitRate      float64
	TotalReturn  float64
	Observations int
}

// CohortWeight is the computed JANUS weight for a single cohort
type CohortWeight struct {
	Regime      prism.RegimeType
	Weight      float64
	ShortScore  float64
	MedScore    float64
	LongScore   float64
	LastUpdated time.Time
}

// JANUSConfig holds tunable parameters for the meta-layer
type JANUSConfig struct {
	// MinWeight is the floor for any cohort weight (prevents total elimination)
	MinWeight float64
	// MaxWeight is the ceiling for any cohort weight
	MaxWeight float64
	// NovelThreshold triggers NOVEL_REGIME when short-window best cohort
	// weight exceeds long-window best cohort weight by this margin
	NovelThreshold float64
	// HistoricalThreshold triggers HISTORICAL_REGIME when long-window
	// best cohort weight exceeds short-window best by this margin
	HistoricalThreshold float64
	// EpsilonWeight assigned to cohorts with negative Sharpe when others are positive
	EpsilonWeight float64
}

// DefaultJANUSConfig returns sensible defaults aligned with Atlas-GIC style dynamics
func DefaultJANUSConfig() JANUSConfig {
	return JANUSConfig{
		MinWeight:           0.05,
		MaxWeight:           0.60,
		NovelThreshold:      0.15,
		HistoricalThreshold: 0.15,
		EpsilonWeight:       0.02,
	}
}
