package domain

import "time"

type BacktestWindowSummary struct {
	WindowID              string
	StartDate             time.Time
	EndDate               time.Time
	SessionCount          int
	OutcomeCount          int
	WorstAgentID          string
	WorstAgentSkill       string
	WorstAgentLayer       AgentLayer
	WorstAgentWindowCount int
	WorstAgentSharpeLike  float64
	GeneratedAt           time.Time
}
