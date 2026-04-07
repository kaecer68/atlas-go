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
	ID               string
	Name             string
	Layer            AgentLayer
	Skill            string
	PromptFile       string
	Enabled          bool
	Universe         []string
	PrimaryMetrics   []string
	RequiredSkills   []string
	ForbiddenActions []string
	OperatingNotes   []string
	DarwinianWeight  float64 `json:"darwinian_weight,omitempty"` // Starting weight for Darwinian system
}

type AgentRegistry struct {
	Version int
	Agents  []AgentSpec
}

type RecommendationOutcome struct {
	AgentID        string
	Skill          string
	Symbol         string
	Window         string
	ForwardReturn  float64
	BenchmarkDelta float64
	Hit            bool
	RecordedAt     time.Time
}

type Scorecard struct {
	AgentID               string
	Skill                 string
	Layer                 AgentLayer
	Observations          int
	WindowCount           int
	HitRate               float64
	AverageReturn         float64
	SharpeLike            float64
	MaxDrawdown           float64
	ConcentrationWarnings int
	LastUpdatedAt         time.Time
}

type ExperimentStatus string

const (
	ExperimentPlanned  ExperimentStatus = "planned"
	ExperimentRunning  ExperimentStatus = "running"
	ExperimentAccepted ExperimentStatus = "accepted"
	ExperimentRejected ExperimentStatus = "rejected"
)

type ExperimentRecord struct {
	ID                string
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

type PromptExperimentResult struct {
	Experiment      ExperimentRecord
	Brief           MutationBrief
	CandidatePrompt string
	EvaluationMode  string
	PolicyChecks    []string
	Notes           []string
	JudgeChecks     []string
	RecordedAt      time.Time
}
