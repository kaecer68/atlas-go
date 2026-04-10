package domain

type GuardSeverity string

const (
	GuardSeveritySoft GuardSeverity = "soft"
	GuardSeverityHard GuardSeverity = "hard"
)

type GuardOutcome struct {
	GuardID     string
	GuardSkill  string
	Severity    GuardSeverity
	Passed      bool
	Reason      string
	InputCount  int
	OutputCount int
}
