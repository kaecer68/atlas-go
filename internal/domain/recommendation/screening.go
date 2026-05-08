package recommendation

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// ScreeningCriteria defines declarative thresholds for stock screening.
type ScreeningCriteria struct {
	PE                  *RangeFilter `json:"pe,omitempty"`
	PB                  *RangeFilter `json:"pb,omitempty"`
	DividendYield       *RangeFilter `json:"dividend_yield,omitempty"`
	Momentum20Day       *RangeFilter `json:"momentum_20d,omitempty"`
	Volatility20Day     *RangeFilter `json:"volatility_20d,omitempty"`
	VolumeIntraday      *MinFilter   `json:"volume_intraday,omitempty"`
	MinTotalFactorScore *float64     `json:"min_total_factor_score,omitempty"`
	RequiredFactors     []string     `json:"required_factors,omitempty"`
}

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

// RangeFilter defines an inclusive numeric range [Min, Max].
type RangeFilter struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

// MinFilter defines a minimum threshold.
type MinFilter struct {
	Min *int64 `json:"min,omitempty"`
}

// ScreeningReject records a single symbol-agent screening failure for audit.
type ScreeningReject struct {
	SessionID      string              `json:"session_id"`
	Symbol         string              `json:"symbol"`
	AgentID        string              `json:"agent_id"`
	Skill          string              `json:"skill"`
	Criterion      string              `json:"criterion"`
	CriterionLabel string              `json:"criterion_label"`
	Threshold      string              `json:"threshold"`
	ActualValue    string              `json:"actual_value"`
	FactorScores   shared.FactorScores `json:"factor_scores"`
	RecordedAt     time.Time           `json:"recorded_at"`
}
