package domain

import "time"

type TaskType string

const (
	TaskTypeRunExperiment    TaskType = "run_experiment"
	TaskTypeJudgeExperiment   TaskType = "judge_experiment"
	TaskTypePromoteBaseline   TaskType = "promote_baseline"
	TaskTypeBacktestWindow    TaskType = "backtest_window"
)

type TaskStatus string

const (
	TaskStatusQueued          TaskStatus = "queued"
	TaskStatusRunning         TaskStatus = "running"
	TaskStatusSucceeded       TaskStatus = "succeeded"
	TaskStatusFailed          TaskStatus = "failed"
	TaskStatusCancelRequested TaskStatus = "cancel_requested"
	TaskStatusCancelled       TaskStatus = "cancelled"
)

type TaskEventType string

const (
	TaskEventStatus   TaskEventType = "status"
	TaskEventProgress TaskEventType = "progress"
	TaskEventStdout   TaskEventType = "stdout"
	TaskEventStderr   TaskEventType = "stderr"
	TaskEventSummary  TaskEventType = "summary"
	TaskEventDone     TaskEventType = "done"
)

type TaskExecution struct {
	ID                    string     `json:"id"`
	TaskType              TaskType   `json:"task_type"`
	CommandName           string     `json:"command_name"`
	CommandArgs           []string   `json:"command_args,omitempty"`
	RequestPayload        []byte     `json:"request_payload,omitempty"`
	Status                TaskStatus `json:"status"`
	Actor                 string     `json:"actor"`
	ActorSource           string     `json:"actor_source"`
	IdempotencyKey        string     `json:"idempotency_key,omitempty"`
	RetryOf               string     `json:"retry_of,omitempty"`
	ParentExecutionID     string     `json:"parent_execution_id,omitempty"`
	ExperimentID          string     `json:"experiment_id,omitempty"`
	ResultPath            string     `json:"result_path,omitempty"`
	BaselineVersionBefore *int       `json:"baseline_version_before,omitempty"`
	BaselineVersionAfter  *int       `json:"baseline_version_after,omitempty"`
	RequiresConfirmation  bool       `json:"requires_confirmation"`
	ConfirmedAt           *time.Time `json:"confirmed_at,omitempty"`
	CancelRequestedAt     *time.Time `json:"cancel_requested_at,omitempty"`
	SubmittedAt           time.Time  `json:"submitted_at"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	FinishedAt            *time.Time `json:"finished_at,omitempty"`
	ExitCode              *int       `json:"exit_code,omitempty"`
	ErrorMessage          string     `json:"error_message,omitempty"`
	Summary               []byte     `json:"summary,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type TaskExecutionEvent struct {
	ExecutionID string        `json:"execution_id"`
	Sequence    int64         `json:"sequence"`
	EventType   TaskEventType `json:"event_type"`
	Stream      string        `json:"stream"`
	Level       string        `json:"level,omitempty"`
	Message     string        `json:"message"`
	Payload     []byte        `json:"payload,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}

type ExperimentLineageRecord struct {
	ExperimentID       string     `json:"experiment_id"`
	ExecutionID        string     `json:"execution_id"`
	ParentExperimentID string     `json:"parent_experiment_id,omitempty"`
	RootExperimentID   string     `json:"root_experiment_id"`
	LineageDepth       int        `json:"lineage_depth"`
	TargetAgentID      string     `json:"target_agent_id"`
	TargetSkill        string     `json:"target_skill"`
	MutationType       string     `json:"mutation_type"`
	BriefPath          string     `json:"brief_path,omitempty"`
	CandidatePath      string     `json:"candidate_path,omitempty"`
	ResultPath         string     `json:"result_path,omitempty"`
	Status             string     `json:"status"`
	GitCommitID        string     `json:"git_commit_id,omitempty"`
	ParamsSnapshot     []byte     `json:"params_snapshot,omitempty"`
	ResultSnapshot     []byte     `json:"result_snapshot,omitempty"`
	BaselineValue      *float64   `json:"baseline_value,omitempty"`
	CandidateValue     *float64   `json:"candidate_value,omitempty"`
	ImprovementValue   *float64   `json:"improvement_value,omitempty"`
	RecordedAt         time.Time  `json:"recorded_at"`
	JudgedAt           *time.Time `json:"judged_at,omitempty"`
}

type BaselineHistoryRecord struct {
	ID            string    `json:"id"`
	ExecutionID   string    `json:"execution_id,omitempty"`
	ExperimentID  string    `json:"experiment_id,omitempty"`
	VersionBefore int       `json:"version_before"`
	VersionAfter  int       `json:"version_after"`
	PromotedBy    string    `json:"promoted_by"`
	PromotedAt    time.Time `json:"promoted_at"`
	BaselinePath  string    `json:"baseline_path"`
	DiffSummary   []byte    `json:"diff_summary,omitempty"`
	DiffPatch     string    `json:"diff_patch,omitempty"`
	Metadata      []byte    `json:"metadata,omitempty"`
}

type MetricTrendPoint struct {
	ID            string    `json:"id"`
	ExecutionID   string    `json:"execution_id"`
	ExperimentID  string    `json:"experiment_id,omitempty"`
	SeriesKey     string    `json:"series_key"`
	MetricName    string    `json:"metric_name"`
	MetricScope   string    `json:"metric_scope"`
	MetricValue   float64   `json:"metric_value"`
	BaselineValue *float64  `json:"baseline_value,omitempty"`
	DeltaValue    *float64  `json:"delta_value,omitempty"`
	SampledAt     time.Time `json:"sampled_at"`
	Tags          []byte    `json:"tags,omitempty"`
}

type ExecutionFilter struct {
	TaskType     string
	Status       string
	ExperimentID string
	Actor        string
	Limit        int
	Cursor       string
	Since        *time.Time
}

type MetricTrendFilter struct {
	ExperimentID string
	SeriesKey    string
	MetricName   string
	MetricScope  string
	Start        time.Time
	End          time.Time
	Limit        int
}
