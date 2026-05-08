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
	FactorScores        shared.FactorScores         `json:"factor_scores,omitempty"`
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
	ScreeningCriteria ScreeningCriteria `json:"screening_criteria,omitempty"`
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
	Reason              string                      `json:"reason"`
	Price               float64                     `json:"price"`
	PassedGuards        bool                        `json:"passed_guards"`
	GuardReason         string                      `json:"guard_reason"`
	RecordedAt          time.Time                   `json:"recorded_at"`
	FactorScores        shared.FactorScores         `json:"factor_scores,omitempty"`
	ConvictionBreakdown *shared.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
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
}

// Scorecard aggregates an agent's historical performance.
type Scorecard struct {
	AgentID               string            `json:"agent_id"`
	Skill                 string            `json:"skill"`
	Layer                 shared.AgentLayer `json:"layer"`
	Observations          int               `json:"observations"`
	WindowCount           int               `json:"windows"`
	HitRate               float64           `json:"hit_rate"`
	AverageReturn         float64           `json:"average_return"`
	SharpeLike            float64           `json:"sharpe"`
	MaxDrawdown           float64           `json:"max_drawdown"`
	ConcentrationWarnings int               `json:"concentration_warnings"`
	LastUpdatedAt         time.Time         `json:"last_updated_at"`
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
