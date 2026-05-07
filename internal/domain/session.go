package domain

import (
	"encoding/json"
	"fmt"
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
	TaxSnapshots          []TaxSnapshot      `json:"tax_snapshots,omitempty"`
	AfterTaxPnL           float64            `json:"after_tax_pnl"`
	TotalTaxPaid          float64            `json:"total_tax_paid"`
}

func (s *SessionSummary) UnmarshalJSON(data []byte) error {
	type alias SessionSummary
	var current alias
	if err := json.Unmarshal(data, &current); err == nil && current.SessionID != "" {
		*s = SessionSummary(current)
		return nil
	}

	type legacyBrokerRuntimeAudit struct {
		Mode             string `json:"Mode"`
		Adapter          string `json:"Adapter"`
		Signer           string `json:"Signer"`
		SignerVersion    string `json:"SignerVersion"`
		KeyID            string `json:"KeyID"`
		MaxRetries       int    `json:"MaxRetries"`
		HTTPTimeoutSec   int    `json:"HTTPTimeoutSec"`
		HTTPAttempts     int    `json:"HTTPAttempts"`
		RetryStatusCodes []int  `json:"RetryStatusCodes"`
		MaxClockSkewSec  int    `json:"MaxClockSkewSec"`
		NonceTTLSec      int    `json:"NonceTTLSec"`
		NonceStore       string `json:"NonceStore"`
		NonceStorePath   string `json:"NonceStorePath"`
		NonceRedisPrefix string `json:"NonceRedisPrefix"`
	}

	type legacyGuardOutcome struct {
		GuardID     string        `json:"GuardID"`
		GuardSkill  string        `json:"GuardSkill"`
		Severity    GuardSeverity `json:"Severity"`
		Passed      bool          `json:"Passed"`
		Reason      string        `json:"Reason"`
		InputCount  int           `json:"InputCount"`
		OutputCount int           `json:"OutputCount"`
	}

	type legacySessionSummary struct {
		SessionID             string                   `json:"SessionID"`
		Regime                Regime                   `json:"Regime"`
		OrderCount            int                      `json:"OrderCount"`
		PositionCount         int                      `json:"PositionCount"`
		EndingCash            float64                  `json:"EndingCash"`
		PortfolioValue        float64                  `json:"PortfolioValue"`
		OutcomeCount          int                      `json:"OutcomeCount"`
		BrokerRuntime         legacyBrokerRuntimeAudit `json:"BrokerRuntime"`
		NextExperimentAgentID string                   `json:"NextExperimentAgentID"`
		ProposalID            string                   `json:"ProposalID"`
		CommitID              string                   `json:"CommitID"`
		ApprovalID            string                   `json:"ApprovalID"`
		GuardOutcomes         []legacyGuardOutcome     `json:"GuardOutcomes"`
		RecordedAt            time.Time                `json:"RecordedAt"`
	}

	var legacy legacySessionSummary
	if err := json.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("unmarshal legacy session summary: %w", err)
	}

	guardOutcomes := make([]GuardOutcome, 0, len(legacy.GuardOutcomes))
	for _, g := range legacy.GuardOutcomes {
		guardOutcomes = append(guardOutcomes, GuardOutcome{
			GuardID:     g.GuardID,
			GuardSkill:  g.GuardSkill,
			Severity:    g.Severity,
			Passed:      g.Passed,
			Reason:      g.Reason,
			InputCount:  g.InputCount,
			OutputCount: g.OutputCount,
		})
	}

	*s = SessionSummary{
		SessionID:      legacy.SessionID,
		Regime:         legacy.Regime,
		OrderCount:     legacy.OrderCount,
		PositionCount:  legacy.PositionCount,
		EndingCash:     legacy.EndingCash,
		PortfolioValue: legacy.PortfolioValue,
		OutcomeCount:   legacy.OutcomeCount,
		BrokerRuntime: BrokerRuntimeAudit{
			Mode:             legacy.BrokerRuntime.Mode,
			Adapter:          legacy.BrokerRuntime.Adapter,
			Signer:           legacy.BrokerRuntime.Signer,
			SignerVersion:    legacy.BrokerRuntime.SignerVersion,
			KeyID:            legacy.BrokerRuntime.KeyID,
			MaxRetries:       legacy.BrokerRuntime.MaxRetries,
			HTTPTimeoutSec:   legacy.BrokerRuntime.HTTPTimeoutSec,
			HTTPAttempts:     legacy.BrokerRuntime.HTTPAttempts,
			RetryStatusCodes: legacy.BrokerRuntime.RetryStatusCodes,
			MaxClockSkewSec:  legacy.BrokerRuntime.MaxClockSkewSec,
			NonceTTLSec:      legacy.BrokerRuntime.NonceTTLSec,
			NonceStore:       legacy.BrokerRuntime.NonceStore,
			NonceStorePath:   legacy.BrokerRuntime.NonceStorePath,
			NonceRedisPrefix: legacy.BrokerRuntime.NonceRedisPrefix,
		},
		NextExperimentAgentID: legacy.NextExperimentAgentID,
		ProposalID:            legacy.ProposalID,
		CommitID:              legacy.CommitID,
		ApprovalID:            legacy.ApprovalID,
		GuardOutcomes:         guardOutcomes,
		RecordedAt:            legacy.RecordedAt,
	}
	return nil
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
