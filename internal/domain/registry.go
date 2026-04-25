package domain

import "time"

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
	AgentID             string
	Skill               string
	Layer               AgentLayer
	Symbol              string
	Side                Side
	Conviction          int
	TargetPrice         float64
	StopLossPrice       float64
	Window              string
	ForwardReturn       float64
	BenchmarkDelta      float64
	Hit                 bool
	Reason              string
	Price               float64
	PassedGuards        bool
	GuardReason         string
	RecordedAt          time.Time
	FactorScores        FactorScores         `json:"factor_scores,omitempty"`
	ConvictionBreakdown *ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
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
	ID                string
	ProposalID        string
	CommitID          string
	ApprovalID        string
	TargetAgentID     string
	Skill             string
	Hypothesis        string
	PromptVersionFrom string
	PromptVersionTo   string
	MutationType      string
	AcceptanceGates   []string
	WindowStart       time.Time
	WindowEnd         time.Time
	AcceptanceMetric  string
	BaselineValue     float64
	CandidateValue    float64
	Status            ExperimentStatus
	RevertReason      string
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
	Experiment            ExperimentRecord
	Brief                 MutationBrief
	CandidatePrompt       string
	EvaluationMode        string
	PolicyChecks          []string
	Notes                 []string
	JudgeChecks           []string
	BaselineObservations  int
	CandidateObservations int
	UsedFallbackWindow    bool
	RecordedAt            time.Time
	DataMetadata          *ReplayDataMetadata `json:"data_metadata,omitempty"`
	OOSResult             *OOSResult          `json:"oos_result,omitempty"`
	BaselineReturns       []float64           `json:"baseline_returns,omitempty"`
	CandidateReturns      []float64           `json:"candidate_returns,omitempty"`
}
