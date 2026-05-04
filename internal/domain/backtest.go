package domain

import "time"

type BacktestWindowSummary struct {
	WindowID              string     `json:"window_id"`
	StartDate             time.Time  `json:"start_date"`
	EndDate               time.Time  `json:"end_date"`
	SessionCount          int        `json:"session_count"`
	OutcomeCount          int        `json:"outcome_count"`
	WorstAgentID          string     `json:"worst_agent_id"`
	WorstAgentSkill       string     `json:"worst_agent_skill"`
	WorstAgentLayer       AgentLayer `json:"worst_agent_layer"`
	WorstAgentWindowCount int        `json:"worst_agent_window_count"`
	WorstAgentSharpeLike  float64    `json:"worst_agent_sharpe_like"`
	GeneratedAt           time.Time  `json:"generated_at"`
}
