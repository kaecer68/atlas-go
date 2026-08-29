package recommendation

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// Recommendation is the core output of a research agent.
type Recommendation struct {
	Agent               string
	Skill               string
	Layer               shared.AgentLayer
	Symbol              string
	Side                shared.Side
	Conviction          int
	TargetPrice         float64
	StopLossPrice       float64
	Reason              string
	ReasoningChain      []string                    `json:"reasoning_chain,omitempty"`
	SupportingEvents    []string                    `json:"supporting_events,omitempty"`
	FactorScores        shared.FactorScores         `json:"factor_scores"`
	ConvictionBreakdown *shared.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
}

// AgentSpec defines a registered agent's metadata.
type AgentSpec struct {
	ID                string
	Name              string
	Layer             shared.AgentLayer
	Skill             string
	PromptFile        string
	Enabled           bool
	Universe          []string
	PrimaryMetrics    []string
	RequiredSkills    []string
	ForbiddenActions  []string
	OperatingNotes    []string
	DarwinianWeight   float64           `json:"darwinian_weight,omitempty"`
	ScreeningCriteria ScreeningCriteria `json:"screening_criteria"`
}

// AgentRegistry holds the set of registered agents.
type AgentRegistry struct {
	Version int
	Agents  []AgentSpec
}

// RecommendationOutcome records the realized result of a recommendation.
type RecommendationOutcome struct {
	AgentID             string                      `json:"agent_id"`
	Skill               string                      `json:"skill"`
	Layer               shared.AgentLayer           `json:"layer"`
	Symbol              string                      `json:"symbol"`
	Side                shared.Side                 `json:"side"`
	Conviction          int                         `json:"conviction"`
	TargetPrice         float64                     `json:"target_price"`
	StopLossPrice       float64                     `json:"stop_loss_price"`
	Window              string                      `json:"window"`
	ForwardReturn       float64                     `json:"forward_return"`
	BenchmarkDelta      float64                     `json:"benchmark_delta"`
	Hit                 bool                        `json:"hit"`
	HitRate             float64                     `json:"hit_rate"`
	Reason              string                      `json:"reason"`
	Price               float64                     `json:"price"`
	PassedGuards        bool                        `json:"passed_guards"`
	GuardReason         string                      `json:"guard_reason"`
	RecordedAt          time.Time                   `json:"recorded_at"`
	FactorScores        shared.FactorScores         `json:"factor_scores"`
	ConvictionBreakdown *shared.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
	SupportingEvents    []string                    `json:"supporting_events,omitempty"`
	ParameterSnapshot   *shared.ParameterSnapshot   `json:"parameter_snapshot,omitempty"`
	IsSynthetic         bool                        `json:"is_synthetic"`
	Regime              string                      `json:"regime,omitempty"`
}

func (r *RecommendationOutcome) Validate() error {
	if r.AgentID == "" {
		return fmt.Errorf("RecommendationOutcome.Validate: missing AgentID")
	}
	if r.Symbol == "" {
		return fmt.Errorf("RecommendationOutcome.Validate: missing Symbol")
	}
	if r.Side == "" {
		return fmt.Errorf("RecommendationOutcome.Validate: missing Side")
	}
	if r.Window == "" {
		return fmt.Errorf("RecommendationOutcome.Validate: missing Window")
	}
	if r.Conviction <= 0 {
		return fmt.Errorf("RecommendationOutcome.Validate: missing Conviction")
	}
	return nil
}

// HumanIntervention records a manual operator action.
type HumanIntervention struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	TargetAgentID string    `json:"target_agent_id,omitempty"`
	TargetModelID string    `json:"target_model_id,omitempty"`
	TargetSector  string    `json:"target_sector,omitempty"`
	TargetSymbol  string    `json:"target_symbol,omitempty"`
	Value         float64   `json:"value,omitempty"`
	Reason        string    `json:"reason"`
	Operator      string    `json:"operator"`
	SessionID     string    `json:"session_id,omitempty"`
	RecordedAt    time.Time `json:"recorded_at"`
	ExpiresAt     time.Time `json:"expires_at,omitzero"`
	TTLHours      int       `json:"ttl_hours,omitempty"`
}

// IsExpired checks whether this intervention has passed its expiry time.
func (h HumanIntervention) IsExpired() bool {
	if h.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(h.ExpiresAt)
}

// Scorecard aggregates an agent's historical performance.
//
// Phase 1 (Observatory Agent Capability Framework) — see internal/domain/AGENTS.md
// for zero-value semantics. Pointer-typed optional fields use omitempty to
// distinguish "no data" from zero.
type Scorecard struct {
	AgentID                  string            `json:"agent_id"`
	Skill                    string            `json:"skill"`
	Layer                    shared.AgentLayer `json:"layer"`
	Observations             int               `json:"observations"`
	WindowCount              int               `json:"windows"`
	HitRate                  float64           `json:"hit_rate"`
	AverageReturn            float64           `json:"average_return"`
	SharpeLike               float64           `json:"sharpe"`
	MaxDrawdown              float64           `json:"max_drawdown"`
	TStat                    float64           `json:"t_stat"`
	HitRateTStat             float64           `json:"hit_rate_t_stat"`
	ConfidenceLow            float64           `json:"confidence_low"`
	ConfidenceHigh           float64           `json:"confidence_high"`
	StatisticallySignificant bool              `json:"statistically_significant"`
	ConcentrationWarnings    int               `json:"concentration_warnings"`
	LastUpdatedAt            time.Time         `json:"last_updated_at"`
	// DarwinianWeight is the dynamic weight from DarwinianWeightManager,
	// clamped to [0.3, 2.5]. Sourced from data/state/darwinian_weights.json.
	DarwinianWeight float64 `json:"darwinian_weight"`
	// DarwinianSharpe is the per-day rolling Sharpe (mean/stdDev*sqrt(252)).
	// Pointer to distinguish "agent not tracked" (nil) from "sharpe=0".
	DarwinianSharpe *float64 `json:"darwinian_sharpe,omitempty"`
	// RegimeBreakdown is the per-agent stratification of session outcomes
	// by regime. Nil when no regime data is available.
	RegimeBreakdown *RegimeBreakdown `json:"regime_breakdown,omitempty"`
	// RegimeStability is stddev of per-regime AvgReturn. Nil when fewer
	// than 2 regimes have data (statistically insufficient).
	RegimeStability *float64 `json:"regime_stability,omitempty"`
	// DataConsistencyWarning is set when scorecard.sharpe (per-outcome
	// formula) diverges from darwinian.sharpe (per-day formula).
	DataConsistencyWarning string `json:"data_consistency_warning,omitempty"`

	// IsSharpe uses the first 80% of outcomes chronologically (per-outcome
	// frequency, no annualization). Same formula as SharpeLike but on a
	// restricted sample, making the comparison with OosSharpe meaningful.
	IsSharpe float64 `json:"is_sharpe"`
	// OosSharpe uses the last 20% of outcomes chronologically.
	OosSharpe float64 `json:"oos_sharpe"`
	// IsOosRatio is |IsSharpe| / max(|OosSharpe|, 0.01). Zero when OOS
	// is the denominator. 0 when OosSharpe is zero (avoid div-by-zero).
	IsOosRatio float64 `json:"is_oos_ratio"`
	// OverfitWarning is true when IS/OOS diverges (ratio > 2.0 OR IS>0+OOS<=0).
	OverfitWarning bool `json:"overfit_warning"`
	// OverfitReason gives the human-readable reason for the warning.
	OverfitReason string `json:"overfit_reason,omitempty"`
	// AfterTaxPnL is the after-tax profit/loss attributable to this agent.
	// Nil when per-agent tax allocation has not been computed.
	// TODO(per-agent-tax): allocate portfolio-level tax proportionally to
	// each agent's contribution; see sim.Engine.computeTaxAdjustedResults.
	AfterTaxPnL *float64 `json:"after_tax_pnl,omitempty"`
	// RollingSharpeTrend is the linear regression slope of per-window
	// Sharpe across the chronological order. Positive = improving,
	// negative = degrading. 0 when fewer than 2 windows.
	RollingSharpeTrend float64 `json:"rolling_sharpe_trend"`
	// OosSampleWarning is set when train or test split has insufficient
	// samples (e.g. "insufficient_test_samples: 3 < 5").
	OosSampleWarning string `json:"oos_sample_warning,omitempty"`
}

// RegimeBreakdown is the per-agent stratification of performance metrics
// by market regime. The map key is the regime label (RISK_ON / RISK_OFF /
// NEUTRAL / unknown) as recorded in SessionSummary.Regime. Canonical
// definition lives in domain (not reporting) to avoid the package-private
// trap of internal/reporting.calculateRegimeBreakdown.
type RegimeBreakdown struct {
	Regimes map[string]RegimePerformance `json:"regimes"`
}

// RegimePerformance is the per-regime aggregate of session outcomes for
// a single agent.
type RegimePerformance struct {
	Regime       string  `json:"regime"`
	SessionCount int     `json:"session_count"`
	TotalReturn  float64 `json:"total_return"`
	WinRate      float64 `json:"win_rate"`
	AvgReturn    float64 `json:"avg_return"`
}

// GuardSeverity classifies a control guard's enforcement level.
type GuardSeverity string

const (
	GuardSeveritySoft GuardSeverity = "soft"
	GuardSeverityHard GuardSeverity = "hard"
)

// GuardOutcome records the result of a control guard's evaluation.
type GuardOutcome struct {
	GuardID     string        `json:"guard_id"`
	GuardSkill  string        `json:"guard_skill"`
	Passed      bool          `json:"passed"`
	Reason      string        `json:"reason"`
	InputCount  int           `json:"input_count"`
	OutputCount int           `json:"output_count"`
	Severity    GuardSeverity `json:"severity"`
}

// ExecutionInput holds the request context passed into an executor.
type ExecutionInput struct {
	Regime               shared.Regime
	RawRecommendations   []Recommendation
	FinalRecommendations []Recommendation
	GuardOutcomes        []GuardOutcome
	DeterminedBy         string
}
