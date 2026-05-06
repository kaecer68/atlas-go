package domain

import (
	"time"
)

type ReplaySession struct {
	ID          string
	Mode        string
	Market      string
	SessionDate time.Time
	DataSource  string
	StartedAt   time.Time
}

type SessionSummary struct {
	SessionID             string             `json:"session_id"`
	Regime                Regime             `json:"regime"`
	OrderCount            int                `json:"order_count"`
	PositionCount         int                `json:"position_count"`
	EndingCash            float64            `json:"ending_cash"`
	PortfolioValue        float64            `json:"portfolio_value"`
	OutcomeCount          int                `json:"outcome_count"`
	BrokerRuntime         BrokerRuntimeAudit `json:"broker_runtime"`
	NextExperimentAgentID string             `json:"next_experiment_agent_id"`
	ProposalID            string             `json:"proposal_id"`
	CommitID              string             `json:"commit_id"`
	ApprovalID            string             `json:"approval_id"`
	GuardOutcomes         []GuardOutcome     `json:"guard_outcomes"`
	RecordedAt            time.Time          `json:"recorded_at"`
}

type BrokerRuntimeAudit struct {
	Mode             string `json:"mode"`
	Adapter          string `json:"adapter"`
	Signer           string `json:"signer"`
	SignerVersion    string `json:"signer_version"`
	KeyID            string `json:"key_id"`
	MaxRetries       int    `json:"max_retries"`
	HTTPTimeoutSec   int    `json:"http_timeout_sec"`
	HTTPAttempts     int    `json:"http_attempts"`
	RetryStatusCodes []int  `json:"retry_status_codes"`
	MaxClockSkewSec  int    `json:"max_clock_skew_sec"`
	NonceTTLSec      int    `json:"nonce_ttl_sec"`
	NonceStore       string `json:"nonce_store"`
	NonceStorePath   string `json:"nonce_store_path"`
	NonceRedisPrefix string `json:"nonce_redis_prefix"`
}
