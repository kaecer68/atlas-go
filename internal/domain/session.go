package domain

import (
	"fmt"
	"strings"
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
	ParametersVersion     string             `json:"parameters_version,omitempty"`
	RiskCommentary        string             `json:"risk_commentary,omitempty"`
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

// SessionDateFromID extracts the trading date from a session ID string.
// Format: session-YYYYMMDD-* (e.g. "session-20260413-daily")
func SessionDateFromID(id string) time.Time {
	const prefix = "session-"
	if !strings.HasPrefix(id, prefix) {
		return time.Time{}
	}
	trimmed := strings.TrimPrefix(id, prefix)
	parts := strings.Split(trimmed, "-")
	if len(parts) < 1 {
		return time.Time{}
	}
	if d, err := time.Parse("20060102", parts[0]); err == nil {
		return d
	}
	return time.Time{}
}

// isValidRegime reports whether r is one of the legal market regimes
// (RISK_ON / RISK_OFF / NEUTRAL). The empty regime is not legal — it is the
// signature of a legacy count-only row, which callers must handle explicitly
// (ValidateLegacy) rather than silently treating as a real market state.
func isValidRegime(r Regime) bool {
	switch r {
	case RegimeRiskOn, RegimeRiskOff, RegimeNeutral:
		return true
	default:
		return false
	}
}

// Validate performs strict validation of a SessionSummary before it is
// persisted through a real-time write path (sim → DualWriteRepository /
// SQLiteOutcomeStore / Store / PostgresLedgerStore).
//
// It is the write-side half of the performance-report SSoT contract
// (docs/decisions/2026-08-23-performance-report-ssot.md): every summary the
// sim persists must be structurally sound, so a corrupted row (missing
// SessionID, zero portfolio value, negative cash/counts, illegal regime)
// fails loudly at write time instead of surfacing weeks later as one more
// "newly discovered data problem" in the performance report.
//
// PortfolioValue must be strictly positive. Zero-valued summaries are NEVER
// written through the real-time path: a zero portfolio value with orders is
// the sim's own anomaly signature (zero_portfolio_with_orders), and the
// count-only legacy rows produced by cmd/backfill-summaries are a
// migration-time construct that must go through ValidateLegacy instead.
//
// Regime: the empty regime is rejected here because a real-time sim run
// always classifies the session's market state; NULL-regime rows are a
// legacy-read concern and are tolerated on the read path.
func (s *SessionSummary) Validate() error {
	if s.SessionID == "" {
		return fmt.Errorf("SessionSummary.Validate: missing SessionID")
	}
	if s.PortfolioValue <= 0 {
		return fmt.Errorf("SessionSummary.Validate: PortfolioValue must be > 0, got %v", s.PortfolioValue)
	}
	if s.EndingCash < 0 {
		return fmt.Errorf("SessionSummary.Validate: EndingCash must be >= 0, got %v", s.EndingCash)
	}
	if s.OutcomeCount < 0 {
		return fmt.Errorf("SessionSummary.Validate: OutcomeCount must be >= 0, got %d", s.OutcomeCount)
	}
	if s.Regime != "" && !isValidRegime(s.Regime) {
		return fmt.Errorf("SessionSummary.Validate: illegal regime %q (want RISK_ON/RISK_OFF/NEUTRAL)", s.Regime)
	}
	if s.Regime == "" {
		return fmt.Errorf("SessionSummary.Validate: missing Regime")
	}
	return nil
}

// ValidateLegacy performs the lenient validation used by backfill / migration
// write paths (DualWriteRepository.SaveSessionSummary, reconcile-sessions,
// cmd/migrate-jsonl-to-sqlite). It exists because legacy count-only rows are
// real production data: rows written before the summary_json backfill carry
// only session_id + outcome_count (PortfolioValue=0, EndingCash=0, Regime=""),
// and cmd/backfill-summaries intentionally writes the same shape.
//
// "Legal 0" vs "corrupted 0" discrimination:
//
//   - Legal 0: PortfolioValue == 0 AND EndingCash == 0 — a count-only row
//     that carries no equity data by construction. The report already
//     excludes these from the equity curve.
//   - Corrupted 0: PortfolioValue == 0 while EndingCash > 0 (cash exists but
//     portfolio value is zero) or OrderCount > 0 (orders were placed into a
//     zero-valued portfolio) — inconsistent data that must not be propagated
//     into the SSoT backend.
//
// Regime may be empty (legacy), but a non-empty regime must still be legal.
func (s *SessionSummary) ValidateLegacy() error {
	if s.SessionID == "" {
		return fmt.Errorf("SessionSummary.ValidateLegacy: missing SessionID")
	}
	if s.PortfolioValue < 0 {
		return fmt.Errorf("SessionSummary.ValidateLegacy: PortfolioValue must be >= 0, got %v", s.PortfolioValue)
	}
	if s.PortfolioValue == 0 && s.EndingCash != 0 {
		return fmt.Errorf("SessionSummary.ValidateLegacy: corrupted zero portfolio (PortfolioValue=0 but EndingCash=%v)", s.EndingCash)
	}
	if s.PortfolioValue == 0 && s.OrderCount > 0 {
		return fmt.Errorf("SessionSummary.ValidateLegacy: corrupted zero portfolio (PortfolioValue=0 but OrderCount=%d)", s.OrderCount)
	}
	if s.EndingCash < 0 {
		return fmt.Errorf("SessionSummary.ValidateLegacy: EndingCash must be >= 0, got %v", s.EndingCash)
	}
	if s.OutcomeCount < 0 {
		return fmt.Errorf("SessionSummary.ValidateLegacy: OutcomeCount must be >= 0, got %d", s.OutcomeCount)
	}
	if s.Regime != "" && !isValidRegime(s.Regime) {
		return fmt.Errorf("SessionSummary.ValidateLegacy: illegal regime %q (want RISK_ON/RISK_OFF/NEUTRAL)", s.Regime)
	}
	return nil
}
