package domain

import (
	"fmt"
	"time"
)

type AgentLayer string

const (
	LayerContext       AgentLayer = "context"
	LayerMacro         AgentLayer = "macro"
	LayerSector        AgentLayer = "sector"
	LayerStyle         AgentLayer = "style"
	LayerSuperinvestor AgentLayer = "superinvestor"
	LayerControl       AgentLayer = "control"
)

type AgentSpec struct {
	ID                string
	Name              string
	Layer             AgentLayer
	Skill             string
	PromptFile        string
	Enabled           bool
	Universe          []string
	PrimaryMetrics    []string
	RequiredSkills    []string
	ForbiddenActions  []string
	OperatingNotes    []string
	DarwinianWeight   float64           `json:"darwinian_weight,omitempty"` // Starting weight for Darwinian system
	ScreeningCriteria ScreeningCriteria `json:"screening_criteria,omitempty"`
}

type AgentRegistry struct {
	Version int
	Agents  []AgentSpec
}

type RecommendationOutcome struct {
	AgentID             string               `json:"agent_id"`
	Skill               string               `json:"skill"`
	Layer               AgentLayer           `json:"layer"`
	Symbol              string               `json:"symbol"`
	Side                Side                 `json:"side"`
	Conviction          int                  `json:"conviction"`
	TargetPrice         float64              `json:"target_price"`
	StopLossPrice       float64              `json:"stop_loss_price"`
	Window              string               `json:"window"`
	ForwardReturn       float64              `json:"forward_return"`
	BenchmarkDelta      float64              `json:"benchmark_delta"`
	Hit                 bool                 `json:"hit"`
	Reason              string               `json:"reason"`
	Price               float64              `json:"price"`
	PassedGuards        bool                 `json:"passed_guards"`
	GuardReason         string               `json:"guard_reason"`
	RecordedAt          time.Time            `json:"recorded_at"`
	FactorScores        FactorScores         `json:"factor_scores,omitempty"`
	ConvictionBreakdown *ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
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

type HumanIntervention struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"` // e.g., "pause_agent", "set_model_weight", "sector_ban", "approve_rec", "reject_rec"
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

type Scorecard struct {
	AgentID               string     `json:"agent_id"`
	Skill                 string     `json:"skill"`
	Layer                 AgentLayer `json:"layer"`
	Observations          int        `json:"observations"`
	WindowCount           int        `json:"windows"`
	HitRate               float64    `json:"hit_rate"`
	AverageReturn         float64    `json:"average_return"`
	SharpeLike            float64    `json:"sharpe"`
	MaxDrawdown           float64    `json:"max_drawdown"`
	ConcentrationWarnings int        `json:"concentration_warnings"`
	LastUpdatedAt         time.Time  `json:"last_updated_at"`
}

type ExperimentStatus string

const (
	ExperimentPlanned  ExperimentStatus = "planned"
	ExperimentRunning  ExperimentStatus = "running"
	ExperimentAccepted ExperimentStatus = "accepted"
	ExperimentRejected ExperimentStatus = "rejected"
	ExperimentExpired  ExperimentStatus = "expired"
)

type ExperimentRecord struct {
	ID                string           `json:"id"`
	ProposalID        string           `json:"proposal_id"`
	CommitID          string           `json:"commit_id"`
	ApprovalID        string           `json:"approval_id"`
	TargetAgentID     string           `json:"target_agent_id"`
	Skill             string           `json:"skill"`
	Hypothesis        string           `json:"hypothesis"`
	PromptVersionFrom string           `json:"prompt_version_from"`
	PromptVersionTo   string           `json:"prompt_version_to"`
	MutationType      string           `json:"mutation_type"`
	AcceptanceGates   []string         `json:"acceptance_gates"`
	WindowStart       time.Time        `json:"window_start"`
	WindowEnd         time.Time        `json:"window_end"`
	AcceptanceMetric  string           `json:"acceptance_metric"`
	BaselineValue     float64          `json:"baseline_value"`
	CandidateValue    float64          `json:"candidate_value"`
	Status            ExperimentStatus `json:"status"`
	RevertReason      string           `json:"revert_reason"`
}

type MutationBrief struct {
	ContractVersion     int        `json:"contract_version,omitempty"`
	ProposalID          string     `json:"proposal_id,omitempty"`
	WindowID            string     `json:"window_id"`
	TargetAgentID       string     `json:"target_agent_id"`
	TargetSkill         string     `json:"target_skill"`
	TargetLayer         AgentLayer `json:"target_layer"`
	PromptFile          string     `json:"prompt_file"`
	MutationType        string     `json:"mutation_type"`
	FailurePattern      string     `json:"failure_pattern"`
	Hypothesis          string     `json:"hypothesis"`
	AcceptanceMetric    string     `json:"acceptance_metric"`
	AcceptanceGates     []string   `json:"acceptance_gates"`
	ForbiddenActions    []string   `json:"forbidden_actions"`
	RequiredSkills      []string   `json:"required_skills"`
	ObservedWindowCount int        `json:"observed_window_count"`
	MaturityLevel       string     `json:"maturity_level"`
	IterationGuidance   []string   `json:"iteration_guidance"`
	RecommendedWindow   string     `json:"recommended_window"`
	GeneratedAt         time.Time  `json:"generated_at"`
}

type ReplayDataMetadata struct {
	SourcePath     string    `json:"source_path"`
	DateRangeStart time.Time `json:"date_range_start"`
	DateRangeEnd   time.Time `json:"date_range_end"`
	DaysDelayed    int       `json:"days_delayed"`
	CoversWindow   bool      `json:"covers_window"`
	LastModified   time.Time `json:"last_modified"`
	RecordCount    int       `json:"record_count"`
}

type OOSResult struct {
	Passed         bool      `json:"passed"`
	BaselineScore  float64   `json:"baseline_score"`
	CandidateScore float64   `json:"candidate_score"`
	Improvement    float64   `json:"improvement"`
	Observations   int       `json:"observations"`
	OOSWindowStart time.Time `json:"oos_window_start"`
	OOSWindowEnd   time.Time `json:"oos_window_end"`
	UsedFallback   bool      `json:"used_fallback"`
	ValidationAt   time.Time `json:"validation_at"`
	Reason         string    `json:"reason"`
}

type PromptExperimentResult struct {
	Experiment             ExperimentRecord    `json:"experiment"`
	Brief                  MutationBrief       `json:"brief"`
	CandidatePrompt        string              `json:"candidate_prompt"`
	EvaluationMode         string              `json:"evaluation_mode"`
	PolicyChecks           []string            `json:"policy_checks"`
	Notes                  []string            `json:"notes"`
	JudgeChecks            []string            `json:"judge_checks"`
	BaselineObservations   int                 `json:"baseline_observations"`
	CandidateObservations  int                 `json:"candidate_observations"`
	UsedFallbackWindow     bool                `json:"used_fallback_window"`
	RecordedAt             time.Time           `json:"recorded_at"`
	DataMetadata           *ReplayDataMetadata `json:"data_metadata,omitempty"`
	OOSResult              *OOSResult          `json:"oos_result,omitempty"`
	BaselineReturns        []float64           `json:"baseline_returns,omitempty"`
	CandidateReturns       []float64           `json:"candidate_returns,omitempty"`
	ParameterSnapshotID    string              `json:"parameter_snapshot_id,omitempty"`
	BaselineFallbackCount  int                 `json:"baseline_fallback_count,omitempty"`
	CandidateFallbackCount int                 `json:"candidate_fallback_count,omitempty"`
	BaselineFactorCount    int                 `json:"baseline_factor_count,omitempty"`
	CandidateFactorCount   int                 `json:"candidate_factor_count,omitempty"`
}
