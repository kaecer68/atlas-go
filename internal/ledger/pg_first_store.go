package ledger

import (
	"fmt"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// PGFirstOutcomeStore is the performance-report read path per the SSoT
// decision (docs/decisions/2026-08-23-performance-report-ssot.md).
//
// PostgreSQL is the single source of truth for session summaries and
// recommendation outcomes. Reads go to PG first; the JSONL ledger is used
// ONLY when PG is unavailable (error), and every such fallback marks the
// store degraded so the report can surface it. The fallback is deliberately
// error-only: a usable-but-empty PG is authoritative (the SSoT backend may
// simply have no data yet — a reconcile/backfill concern, not a reason to
// silently mix backends again).
type PGFirstOutcomeStore struct {
	pg    OutcomeStore // PostgresLedgerStore
	jsonl OutcomeStore // JSONL *Store

	mu            sync.Mutex
	degraded      bool
	fallbackCount int64
}

// NewPGFirstOutcomeStore wraps the authoritative PG store with a JSONL
// fallback. jsonl may be nil to disable the fallback (PG-only).
func NewPGFirstOutcomeStore(pg, jsonl OutcomeStore) *PGFirstOutcomeStore {
	return &PGFirstOutcomeStore{pg: pg, jsonl: jsonl}
}

// Degraded reports whether the most recent read fell back to JSONL because
// PG was unavailable. Implemented so reporting.GenerateReport can mark the
// report output (reportSourceInfo).
func (s *PGFirstOutcomeStore) Degraded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.degraded
}

// SourceBackend reports which backend actually served the most recent read:
// "postgres" normally, "jsonl" when degraded. Implemented so
// reporting.GenerateReport can label the report output.
func (s *PGFirstOutcomeStore) SourceBackend() string {
	if s.Degraded() {
		return "jsonl"
	}
	return "postgres"
}

// FallbackCount returns how many reads have degraded to the JSONL side.
func (s *PGFirstOutcomeStore) FallbackCount() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fallbackCount
}

func (s *PGFirstOutcomeStore) markDegraded(degraded bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.degraded = degraded
	if degraded {
		s.fallbackCount++
	}
}

// LoadSessionSummaries reads PG first; JSONL fallback (marked degraded) only
// when PG errors. A successful PG read — even an empty one — is authoritative.
func (s *PGFirstOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	summaries, err := s.pg.LoadSessionSummaries()
	if err == nil {
		s.markDegraded(false)
		return summaries, nil
	}
	if s.jsonl == nil {
		return nil, fmt.Errorf("load session summaries from postgres: %w", err)
	}
	fallback, ferr := s.jsonl.LoadSessionSummaries()
	if ferr != nil {
		return nil, fmt.Errorf("postgres unavailable (%v) and jsonl fallback failed: %w", err, ferr)
	}
	s.markDegraded(true)
	return fallback, nil
}

// LoadSessionOutcomes reads PG first; JSONL fallback (marked degraded) only
// when PG errors. A successful PG read returning zero outcomes is
// authoritative (the session simply has no outcomes in the SSoT backend).
func (s *PGFirstOutcomeStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	outcomes, err := s.pg.LoadSessionOutcomes(sessionID)
	if err == nil {
		s.markDegraded(false)
		return outcomes, nil
	}
	if s.jsonl == nil {
		return nil, fmt.Errorf("load session outcomes from postgres: %w", err)
	}
	fallback, ferr := s.jsonl.LoadSessionOutcomes(sessionID)
	if ferr != nil {
		return nil, fmt.Errorf("postgres unavailable (%v) and jsonl fallback failed: %w", err, ferr)
	}
	s.markDegraded(true)
	return fallback, nil
}

// ---- the rest of OutcomeStore delegates to PG (write paths). ----

func (s *PGFirstOutcomeStore) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	return s.pg.RecordOutcomes(outcomes)
}

func (s *PGFirstOutcomeStore) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	return s.pg.RecordSessionOutcomes(session, outcomes)
}

func (s *PGFirstOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return s.pg.LoadOutcomes()
}

func (s *PGFirstOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return s.pg.LoadOutcomesFromSessions()
}

func (s *PGFirstOutcomeStore) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	return s.pg.RecordSessionScreeningRejects(sessionID, rejects)
}

func (s *PGFirstOutcomeStore) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	return s.pg.LoadSessionScreeningRejects(sessionID)
}

func (s *PGFirstOutcomeStore) RecordSessionTrades(sessionID string, trades []domain.TradeRecord) error {
	return s.pg.RecordSessionTrades(sessionID, trades)
}

func (s *PGFirstOutcomeStore) LoadSessionTrades(sessionID string) ([]domain.TradeRecord, error) {
	return s.pg.LoadSessionTrades(sessionID)
}

// LoadAllSessionTrades reads PG first; JSONL fallback (marked degraded) only
// when PG errors. A successful PG read returning zero trades is authoritative
// (the SSoT backend simply has no executed trades). The performance report
// counts executed trades during generation (SSOT P1-4), so this read needs
// the same degraded semantics as LoadSessionSummaries — otherwise a PG blip
// would fail the whole report.
func (s *PGFirstOutcomeStore) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	trades, err := s.pg.LoadAllSessionTrades()
	if err == nil {
		s.markDegraded(false)
		return trades, nil
	}
	if s.jsonl == nil {
		return nil, fmt.Errorf("load all session trades from postgres: %w", err)
	}
	fallback, ferr := s.jsonl.LoadAllSessionTrades()
	if ferr != nil {
		return nil, fmt.Errorf("postgres unavailable (%v) and jsonl fallback failed: %w", err, ferr)
	}
	s.markDegraded(true)
	return fallback, nil
}

func (s *PGFirstOutcomeStore) RecordExperiment(record domain.ExperimentRecord) error {
	return s.pg.RecordExperiment(record)
}

func (s *PGFirstOutcomeStore) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	return s.pg.RecordSessionExperiment(session, record)
}

func (s *PGFirstOutcomeStore) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	return s.pg.RecordSessionSummary(session, summary)
}

func (s *PGFirstOutcomeStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return s.pg.LoadAllSessionScorecards()
}

func (s *PGFirstOutcomeStore) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	return s.pg.RecordHumanIntervention(intervention)
}

func (s *PGFirstOutcomeStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return s.pg.LoadHumanInterventions()
}

var _ OutcomeStore = (*PGFirstOutcomeStore)(nil)
