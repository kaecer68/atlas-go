package domain

import (
	"encoding/json"
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

func (r *RecommendationOutcome) UnmarshalJSON(data []byte) error {
	type alias RecommendationOutcome
	var current alias
	if err := json.Unmarshal(data, &current); err == nil && current.AgentID != "" {
		*r = RecommendationOutcome(current)
		return nil
	}

	type legacyRecommendationOutcome struct {
		AgentID             string               `json:"AgentID"`
		Skill               string               `json:"Skill"`
		Layer               AgentLayer           `json:"Layer"`
		Symbol              string               `json:"Symbol"`
		Side                Side                 `json:"Side"`
		Conviction          int                  `json:"Conviction"`
		TargetPrice         float64              `json:"TargetPrice"`
		StopLossPrice       float64              `json:"StopLossPrice"`
		Window              string               `json:"Window"`
		ForwardReturn       float64              `json:"ForwardReturn"`
		BenchmarkDelta      float64              `json:"BenchmarkDelta"`
		Hit                 bool                 `json:"Hit"`
		Reason              string               `json:"Reason"`
		Price               float64              `json:"Price"`
		PassedGuards        bool                 `json:"PassedGuards"`
		GuardReason         string               `json:"GuardReason"`
		RecordedAt          time.Time            `json:"RecordedAt"`
		FactorScores        FactorScores         `json:"factor_scores,omitempty"`
		ConvictionBreakdown *ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
	}

	var legacy legacyRecommendationOutcome
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	*r = RecommendationOutcome{
		AgentID:             legacy.AgentID,
		Skill:               legacy.Skill,
		Layer:               legacy.Layer,
		Symbol:              legacy.Symbol,
		Side:                legacy.Side,
		Conviction:          legacy.Conviction,
		TargetPrice:         legacy.TargetPrice,
		StopLossPrice:       legacy.StopLossPrice,
		Window:              legacy.Window,
		ForwardReturn:       legacy.ForwardReturn,
		BenchmarkDelta:      legacy.BenchmarkDelta,
		Hit:                 legacy.Hit,
		Reason:              legacy.Reason,
		Price:               legacy.Price,
		PassedGuards:        legacy.PassedGuards,
		GuardReason:         legacy.GuardReason,
		RecordedAt:          legacy.RecordedAt,
		FactorScores:        legacy.FactorScores,
		ConvictionBreakdown: legacy.ConvictionBreakdown,
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

func (r *ExperimentRecord) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	isCanonical := false
	for key := range raw {
		if key == "id" {
			isCanonical = true
			break
		}
		for i := 0; i < len(key); i++ {
			if key[i] == '_' {
				isCanonical = true
				break
			}
		}
		if isCanonical {
			break
		}
	}

	if isCanonical {
		type alias ExperimentRecord
		var current alias
		if err := json.Unmarshal(data, &current); err == nil {
			*r = ExperimentRecord(current)
			return nil
		}
	}

	type legacyExperimentRecord struct {
		ID                string           `json:"ID"`
		ProposalID        string           `json:"ProposalID"`
		CommitID          string           `json:"CommitID"`
		ApprovalID        string           `json:"ApprovalID"`
		TargetAgentID     string           `json:"TargetAgentID"`
		Skill             string           `json:"Skill"`
		Hypothesis        string           `json:"Hypothesis"`
		PromptVersionFrom string           `json:"PromptVersionFrom"`
		PromptVersionTo   string           `json:"PromptVersionTo"`
		MutationType      string           `json:"MutationType"`
		AcceptanceGates   []string         `json:"AcceptanceGates"`
		WindowStart       time.Time        `json:"WindowStart"`
		WindowEnd         time.Time        `json:"WindowEnd"`
		AcceptanceMetric  string           `json:"AcceptanceMetric"`
		BaselineValue     float64          `json:"BaselineValue"`
		CandidateValue    float64          `json:"CandidateValue"`
		Status            ExperimentStatus `json:"Status"`
		RevertReason      string           `json:"RevertReason"`
	}

	var legacy legacyExperimentRecord
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	*r = ExperimentRecord{
		ID:                legacy.ID,
		ProposalID:        legacy.ProposalID,
		CommitID:          legacy.CommitID,
		ApprovalID:        legacy.ApprovalID,
		TargetAgentID:     legacy.TargetAgentID,
		Skill:             legacy.Skill,
		Hypothesis:        legacy.Hypothesis,
		PromptVersionFrom: legacy.PromptVersionFrom,
		PromptVersionTo:   legacy.PromptVersionTo,
		MutationType:      legacy.MutationType,
		AcceptanceGates:   legacy.AcceptanceGates,
		WindowStart:       legacy.WindowStart,
		WindowEnd:         legacy.WindowEnd,
		AcceptanceMetric:  legacy.AcceptanceMetric,
		BaselineValue:     legacy.BaselineValue,
		CandidateValue:    legacy.CandidateValue,
		Status:            legacy.Status,
		RevertReason:      legacy.RevertReason,
	}
	return nil
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

func (b *MutationBrief) UnmarshalJSON(data []byte) error {
	type alias MutationBrief
	var current alias
	if err := json.Unmarshal(data, &current); err == nil && current.WindowID != "" {
		*b = MutationBrief(current)
		return nil
	}

	type legacyMutationBrief struct {
		ContractVersion     int        `json:"ContractVersion"`
		ProposalID          string     `json:"ProposalID"`
		WindowID            string     `json:"WindowID"`
		TargetAgentID       string     `json:"TargetAgentID"`
		TargetSkill         string     `json:"TargetSkill"`
		TargetLayer         AgentLayer `json:"TargetLayer"`
		PromptFile          string     `json:"PromptFile"`
		MutationType        string     `json:"MutationType"`
		FailurePattern      string     `json:"FailurePattern"`
		Hypothesis          string     `json:"Hypothesis"`
		AcceptanceMetric    string     `json:"AcceptanceMetric"`
		AcceptanceGates     []string   `json:"AcceptanceGates"`
		ForbiddenActions    []string   `json:"ForbiddenActions"`
		RequiredSkills      []string   `json:"RequiredSkills"`
		ObservedWindowCount int        `json:"ObservedWindowCount"`
		MaturityLevel       string     `json:"MaturityLevel"`
		IterationGuidance   []string   `json:"IterationGuidance"`
		RecommendedWindow   string     `json:"RecommendedWindow"`
		GeneratedAt         time.Time  `json:"GeneratedAt"`
	}

	var legacy legacyMutationBrief
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	*b = MutationBrief{
		ContractVersion:     legacy.ContractVersion,
		ProposalID:          legacy.ProposalID,
		WindowID:            legacy.WindowID,
		TargetAgentID:       legacy.TargetAgentID,
		TargetSkill:         legacy.TargetSkill,
		TargetLayer:         legacy.TargetLayer,
		PromptFile:          legacy.PromptFile,
		MutationType:        legacy.MutationType,
		FailurePattern:      legacy.FailurePattern,
		Hypothesis:          legacy.Hypothesis,
		AcceptanceMetric:    legacy.AcceptanceMetric,
		AcceptanceGates:     legacy.AcceptanceGates,
		ForbiddenActions:    legacy.ForbiddenActions,
		RequiredSkills:      legacy.RequiredSkills,
		ObservedWindowCount: legacy.ObservedWindowCount,
		MaturityLevel:       legacy.MaturityLevel,
		IterationGuidance:   legacy.IterationGuidance,
		RecommendedWindow:   legacy.RecommendedWindow,
		GeneratedAt:         legacy.GeneratedAt,
	}
	return nil
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

func (m *ReplayDataMetadata) UnmarshalJSON(data []byte) error {
	type alias ReplayDataMetadata
	var current alias
	if err := json.Unmarshal(data, &current); err == nil && current.SourcePath != "" {
		*m = ReplayDataMetadata(current)
		return nil
	}

	type legacyReplayDataMetadata struct {
		SourcePath     string    `json:"SourcePath"`
		DateRangeStart time.Time `json:"DateRangeStart"`
		DateRangeEnd   time.Time `json:"DateRangeEnd"`
		DaysDelayed    int       `json:"DaysDelayed"`
		CoversWindow   bool      `json:"CoversWindow"`
		LastModified   time.Time `json:"LastModified"`
		RecordCount    int       `json:"RecordCount"`
	}

	var legacy legacyReplayDataMetadata
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	*m = ReplayDataMetadata{
		SourcePath:     legacy.SourcePath,
		DateRangeStart: legacy.DateRangeStart,
		DateRangeEnd:   legacy.DateRangeEnd,
		DaysDelayed:    legacy.DaysDelayed,
		CoversWindow:   legacy.CoversWindow,
		LastModified:   legacy.LastModified,
		RecordCount:    legacy.RecordCount,
	}
	return nil
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

func (r *OOSResult) UnmarshalJSON(data []byte) error {
	type alias OOSResult
	var current alias
	if err := json.Unmarshal(data, &current); err == nil {
		*r = OOSResult(current)
		return nil
	}

	type legacyOOSResult struct {
		Passed         bool      `json:"Passed"`
		BaselineScore  float64   `json:"BaselineScore"`
		CandidateScore float64   `json:"CandidateScore"`
		Improvement    float64   `json:"Improvement"`
		Observations   int       `json:"Observations"`
		OOSWindowStart time.Time `json:"OOSWindowStart"`
		OOSWindowEnd   time.Time `json:"OOSWindowEnd"`
		UsedFallback   bool      `json:"UsedFallback"`
		ValidationAt   time.Time `json:"ValidationAt"`
		Reason         string    `json:"Reason"`
	}

	var legacy legacyOOSResult
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}

	*r = OOSResult{
		Passed:         legacy.Passed,
		BaselineScore:  legacy.BaselineScore,
		CandidateScore: legacy.CandidateScore,
		Improvement:    legacy.Improvement,
		Observations:   legacy.Observations,
		OOSWindowStart: legacy.OOSWindowStart,
		OOSWindowEnd:   legacy.OOSWindowEnd,
		UsedFallback:   legacy.UsedFallback,
		ValidationAt:   legacy.ValidationAt,
		Reason:         legacy.Reason,
	}
	return nil
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

func (r *PromptExperimentResult) UnmarshalJSON(data []byte) error {
	type alias PromptExperimentResult

	var current alias
	_ = json.Unmarshal(data, &current)

	type legacyPromptExperimentResult struct {
		Experiment             ExperimentRecord    `json:"Experiment"`
		Brief                  MutationBrief       `json:"Brief"`
		CandidatePrompt        string              `json:"CandidatePrompt"`
		EvaluationMode         string              `json:"EvaluationMode"`
		PolicyChecks           []string            `json:"PolicyChecks"`
		Notes                  []string            `json:"Notes"`
		JudgeChecks            []string            `json:"JudgeChecks"`
		BaselineObservations   int                 `json:"BaselineObservations"`
		CandidateObservations  int                 `json:"CandidateObservations"`
		UsedFallbackWindow     bool                `json:"UsedFallbackWindow"`
		RecordedAt             time.Time           `json:"RecordedAt"`
		DataMetadata           *ReplayDataMetadata `json:"DataMetadata,omitempty"`
		OOSResult              *OOSResult          `json:"OOSResult,omitempty"`
		BaselineReturns        []float64           `json:"BaselineReturns,omitempty"`
		CandidateReturns       []float64           `json:"CandidateReturns,omitempty"`
		ParameterSnapshotID    string              `json:"ParameterSnapshotID,omitempty"`
		BaselineFallbackCount  int                 `json:"BaselineFallbackCount,omitempty"`
		CandidateFallbackCount int                 `json:"CandidateFallbackCount,omitempty"`
		BaselineFactorCount    int                 `json:"BaselineFactorCount,omitempty"`
		CandidateFactorCount   int                 `json:"CandidateFactorCount,omitempty"`
	}

	var legacy legacyPromptExperimentResult
	_ = json.Unmarshal(data, &legacy)

	result := PromptExperimentResult(current)

	if result.Experiment.ID == "" {
		result.Experiment = legacy.Experiment
	}
	if result.Brief.WindowID == "" {
		result.Brief = legacy.Brief
	}
	if result.CandidatePrompt == "" {
		result.CandidatePrompt = legacy.CandidatePrompt
	}
	if result.EvaluationMode == "" {
		result.EvaluationMode = legacy.EvaluationMode
	}
	if len(result.PolicyChecks) == 0 {
		result.PolicyChecks = legacy.PolicyChecks
	}
	if len(result.Notes) == 0 {
		result.Notes = legacy.Notes
	}
	if len(result.JudgeChecks) == 0 {
		result.JudgeChecks = legacy.JudgeChecks
	}
	if result.BaselineObservations == 0 {
		result.BaselineObservations = legacy.BaselineObservations
	}
	if result.CandidateObservations == 0 {
		result.CandidateObservations = legacy.CandidateObservations
	}
	if !result.UsedFallbackWindow {
		result.UsedFallbackWindow = legacy.UsedFallbackWindow
	}
	if result.RecordedAt.IsZero() {
		result.RecordedAt = legacy.RecordedAt
	}
	if result.DataMetadata == nil {
		result.DataMetadata = legacy.DataMetadata
	}
	if result.OOSResult == nil {
		result.OOSResult = legacy.OOSResult
	}
	if len(result.BaselineReturns) == 0 {
		result.BaselineReturns = legacy.BaselineReturns
	}
	if len(result.CandidateReturns) == 0 {
		result.CandidateReturns = legacy.CandidateReturns
	}
	if result.ParameterSnapshotID == "" {
		result.ParameterSnapshotID = legacy.ParameterSnapshotID
	}
	if result.BaselineFallbackCount == 0 {
		result.BaselineFallbackCount = legacy.BaselineFallbackCount
	}
	if result.CandidateFallbackCount == 0 {
		result.CandidateFallbackCount = legacy.CandidateFallbackCount
	}
	if result.BaselineFactorCount == 0 {
		result.BaselineFactorCount = legacy.BaselineFactorCount
	}
	if result.CandidateFactorCount == 0 {
		result.CandidateFactorCount = legacy.CandidateFactorCount
	}

	*r = result
	return nil
}
