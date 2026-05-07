package domain

type GuardSeverity string

const (
	GuardSeveritySoft GuardSeverity = "soft"
	GuardSeverityHard GuardSeverity = "hard"
)

type GuardOutcome struct {
	GuardID     string        `json:"guard_id"`
	GuardSkill  string        `json:"guard_skill"`
	Severity    GuardSeverity `json:"severity"`
	Passed      bool          `json:"passed"`
	Reason      string        `json:"reason"`
	InputCount  int           `json:"input_count"`
	OutputCount int           `json:"output_count"`
}
