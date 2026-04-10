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
	BrokerRuntime         BrokerRuntimeAudit
	NextExperimentAgentID string
	ProposalID            string
	CommitID              string
	ApprovalID            string
	GuardOutcomes         []GuardOutcome
	RecordedAt            time.Time
}

type BrokerRuntimeAudit struct {
	Mode             string
	Adapter          string
	Signer           string
	KeyID            string
	MaxRetries       int
	HTTPTimeoutSec   int
	HTTPAttempts     int
	RetryStatusCodes []int
	MaxClockSkewSec  int
	NonceTTLSec      int
	NonceStore       string
	NonceStorePath   string
}
