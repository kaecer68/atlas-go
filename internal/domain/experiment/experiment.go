package experiment

import (
	"encoding/json"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/eval"
)

// MutationBriefContractVersion is the current contract version for mutation briefs.
const MutationBriefContractVersion = 1

// ExperimentStatus tracks the lifecycle state of an experiment.
type ExperimentStatus string

const (
	ExperimentPlanned  ExperimentStatus = "planned"
	ExperimentRunning  ExperimentStatus = "running"
	ExperimentAccepted ExperimentStatus = "accepted"
	ExperimentRejected ExperimentStatus = "rejected"
	ExperimentExpired  ExperimentStatus = "expired"
)

// ExperimentRecord captures the execution state of a single experiment.
type ExperimentRecord struct {
	ID                   string           `json:"id"`
	ProposalID           string           `json:"proposal_id"`
	CommitID             string           `json:"commit_id"`
	ApprovalID           string           `json:"approval_id"`
	TargetAgentID        string           `json:"target_agent_id"`
	Skill                string           `json:"skill"`
	Hypothesis           string           `json:"hypothesis"`
	PromptVersionFrom    string           `json:"prompt_version_from"`
	PromptVersionTo      string           `json:"prompt_version_to"`
	MutationType         string           `json:"mutation_type"`
	AcceptanceGates      []string         `json:"acceptance_gates"`
	WindowStart          time.Time        `json:"window_start"`
	WindowEnd            time.Time        `json:"window_end"`
	AcceptanceMetric     string           `json:"acceptance_metric"`
	BaselineValue        float64          `json:"baseline_value"`
	CandidateValue       float64          `json:"candidate_value"`
	BaselineMonetaryNTD  float64          `json:"baseline_monetary_ntd,omitempty"`
	CandidateMonetaryNTD float64          `json:"candidate_monetary_ntd,omitempty"`
	Status               ExperimentStatus `json:"status"`
	RevertReason         string           `json:"revert_reason"`
}

// MutationBrief defines the mutation specification for an experiment.
type MutationBrief struct {
	ContractVersion     int               `json:"contract_version,omitempty"`
	ProposalID          string            `json:"proposal_id,omitempty"`
	WindowID            string            `json:"window_id"`
	TargetAgentID       string            `json:"target_agent_id"`
	TargetSkill         string            `json:"target_skill"`
	TargetLayer         shared.AgentLayer `json:"target_layer"`
	PromptFile          string            `json:"prompt_file"`
	MutationType        string            `json:"mutation_type"`
	FailurePattern      string            `json:"failure_pattern"`
	Hypothesis          string            `json:"hypothesis"`
	AcceptanceMetric    string            `json:"acceptance_metric"`
	AcceptanceGates     []string          `json:"acceptance_gates"`
	ForbiddenActions    []string          `json:"forbidden_actions"`
	RequiredSkills      []string          `json:"required_skills"`
	ObservedWindowCount int               `json:"observed_window_count"`
	MaturityLevel       string            `json:"maturity_level"`
	IterationGuidance   []string          `json:"iteration_guidance"`
	RecommendedWindow   string            `json:"recommended_window"`
	RSITwScore          float64           `json:"rsi_tw_score,omitempty"`
	GeneratedAt         time.Time         `json:"generated_at"`
	// Architecture mutation fields (mutation_type: pipeline_stage_toggle)
	PipelineStage  string `json:"pipeline_stage,omitempty"`
	PipelineAction string `json:"pipeline_action,omitempty"` // "enable" or "disable"
}

// ReplayDataMetadata describes a replay dataset for experiment validation.
type ReplayDataMetadata struct {
	SourcePath     string    `json:"source_path"`
	DateRangeStart time.Time `json:"date_range_start"`
	DateRangeEnd   time.Time `json:"date_range_end"`
	DaysDelayed    int       `json:"days_delayed"`
	CoversWindow   bool      `json:"covers_window"`
	LastModified   time.Time `json:"last_modified"`
	RecordCount    int       `json:"record_count"`
}

// OOSResult holds the out-of-sample validation result for an experiment.
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

// PromptExperimentResult holds the full result of a prompt experiment execution.
type PromptExperimentResult struct {
	Experiment             ExperimentRecord       `json:"experiment"`
	Brief                  MutationBrief          `json:"brief"`
	CandidatePrompt        string                 `json:"candidate_prompt"`
	EvaluationMode         string                 `json:"evaluation_mode"`
	PolicyChecks           []string               `json:"policy_checks"`
	Notes                  []string               `json:"notes"`
	JudgeChecks            []string               `json:"judge_checks"`
	BaselineObservations   int                    `json:"baseline_observations"`
	CandidateObservations  int                    `json:"candidate_observations"`
	UsedFallbackWindow     bool                   `json:"used_fallback_window"`
	RecordedAt             time.Time              `json:"recorded_at"`
	DataMetadata           *ReplayDataMetadata    `json:"data_metadata,omitempty"`
	OOSResult              *OOSResult             `json:"oos_result,omitempty"`
	BaselineReturns        []float64              `json:"baseline_returns,omitempty"`
	CandidateReturns       []float64              `json:"candidate_returns,omitempty"`
	ParameterSnapshotID    string                 `json:"parameter_snapshot_id,omitempty"`
	BaselineFallbackCount  int                    `json:"baseline_fallback_count,omitempty"`
	CandidateFallbackCount int                    `json:"candidate_fallback_count,omitempty"`
	BaselineFactorCount    int                    `json:"baseline_factor_count,omitempty"`
	CandidateFactorCount   int                    `json:"candidate_factor_count,omitempty"`
	BaselineMonetaryNTD    float64                `json:"baseline_monetary_ntd,omitempty"`
	CandidateMonetaryNTD   float64                `json:"candidate_monetary_ntd,omitempty"`
	EvalMetrics            *eval.EvalResult       `json:"eval_metrics,omitempty"`
	ImportanceResult       *eval.ImportanceResult `json:"importance_result,omitempty"`
	RegimeCounts           map[string]int         `json:"regime_counts,omitempty"`
	RegimeTotalDays        int                    `json:"regime_total_days,omitempty"`
}

// UnmarshalJSON decodes a prompt experiment result file with tolerance for
// the legacy "notes" schema drift (audit A2): early production files wrote
// notes as a single JSON string, the current schema uses []string. The field
// itself stays []string (JSON tag unchanged) while this decoder normalizes a
// legacy string into a one-element slice before the regular field decoding,
// so monitoring readers stop failing with parse_experiment_file_failed.
func (r *PromptExperimentResult) UnmarshalJSON(data []byte) error {
	normalized, err := normalizeNotesField(data)
	if err != nil {
		return err
	}
	// Decode into an alias type to avoid recursing into this method.
	type rawResult PromptExperimentResult
	var out rawResult
	if err := json.Unmarshal(normalized, &out); err != nil {
		return err
	}
	*r = PromptExperimentResult(out)
	return nil
}

// normalizeNotesField rewrites a legacy string "notes" value into the
// []string shape. Non-legacy inputs are returned unchanged.
func normalizeNotesField(data []byte) ([]byte, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	notes, ok := raw["notes"]
	if !ok || len(notes) == 0 {
		return data, nil
	}
	if notes[0] == '"' {
		var s string
		if err := json.Unmarshal(notes, &s); err != nil {
			return nil, err
		}
		arr, err := json.Marshal([]string{s})
		if err != nil {
			return nil, err
		}
		raw["notes"] = arr
		normalized, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		return normalized, nil
	}
	return data, nil
}
