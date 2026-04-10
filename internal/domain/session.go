package domain

import "time"

type ReplaySession struct {
	ID          string
	Mode        string
	Market      string
	SessionDate time.Time
	DataSource  string
	StartedAt   time.Time
}

type SessionSummary struct {
	SessionID             string
	Regime                Regime
	OrderCount            int
	PositionCount         int
	EndingCash            float64
	OutcomeCount          int
	NextExperimentAgentID string
	ProposalID            string
	CommitID              string
	ApprovalID            string
	GuardOutcomes         []GuardOutcome
	RecordedAt            time.Time
}
