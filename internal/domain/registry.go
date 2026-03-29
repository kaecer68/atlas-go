package domain

import "time"

type AgentLayer string

const (
	LayerContext AgentLayer = "context"
	LayerSector  AgentLayer = "sector"
	LayerStyle   AgentLayer = "style"
	LayerControl AgentLayer = "control"
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
	WindowID            string
	TargetAgentID       string
	TargetSkill         string
	TargetLayer         AgentLayer
	PromptFile          string
	MutationType        string
	FailurePattern      string
	Hypothesis          string
	AcceptanceMetric    string
	AcceptanceGates     []string
	ForbiddenActions    []string
	RequiredSkills      []string
	ObservedWindowCount int
	MaturityLevel       string
	IterationGuidance   []string
	RecommendedWindow   string
	GeneratedAt         time.Time
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
