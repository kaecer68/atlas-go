package domain

import "time"

// ScreeningCriteria defines declarative thresholds for stock screening.
// When a field is nil or absent, that filter is skipped (pass-through).
type ScreeningCriteria struct {
	// Fundamental filters
	PE            *RangeFilter `json:"pe,omitempty"`
	PB            *RangeFilter `json:"pb,omitempty"`
	DividendYield *RangeFilter `json:"dividend_yield,omitempty"`

	// Technical filters
	Momentum20Day   *RangeFilter `json:"momentum_20d,omitempty"`
	Volatility20Day *RangeFilter `json:"volatility_20d,omitempty"`
	VolumeIntraday  *MinFilter   `json:"volume_intraday,omitempty"`

	// Composite filters
	MinTotalFactorScore *float64 `json:"min_total_factor_score,omitempty"`
	RequiredFactors     []string `json:"required_factors,omitempty"`
}

// RangeFilter defines an inclusive numeric range [Min, Max].
// A nil bound means unbounded on that side.
type RangeFilter struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// MinFilter defines a minimum threshold.
type MinFilter struct {
	Min *int64 `json:"min,omitempty"`
}

// HasFilters returns true if any screening condition is set.
func (sc ScreeningCriteria) HasFilters() bool {
	return sc.PE != nil ||
		sc.PB != nil ||
		sc.DividendYield != nil ||
		sc.Momentum20Day != nil ||
		sc.Volatility20Day != nil ||
		sc.VolumeIntraday != nil ||
		sc.MinTotalFactorScore != nil ||
		len(sc.RequiredFactors) > 0
}

// ScreeningReject records a single symbol-agent screening failure for audit.
type ScreeningReject struct {
	SessionID      string       `json:"session_id"`
	Symbol         string       `json:"symbol"`
	AgentID        string       `json:"agent_id"`
	Skill          string       `json:"skill"`
	Criterion      string       `json:"criterion"`
	CriterionLabel string       `json:"criterion_label"`
	Threshold      string       `json:"threshold"`
	ActualValue    string       `json:"actual_value"`
	FactorScores   FactorScores `json:"factor_scores,omitempty"`
	RecordedAt     time.Time    `json:"recorded_at"`
}
