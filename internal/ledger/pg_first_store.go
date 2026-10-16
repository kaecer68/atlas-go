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

// LoadSessionScorecardOutcomes is the slim per-session read used by the
// performance report: scalar columns only, metadata JSONB never transferred.
// Falls back to the full JSONL read when postgres is unavailable (degraded).
func (s *PGFirstOutcomeStore) LoadSessionScorecardOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	slim, ok := s.pg.(interface {
		LoadSessionScorecardOutcomes(string) ([]domain.RecommendationOutcome, error)
	})
	if !ok {
		// Backend without the slim projection: keep the full read.
		return s.LoadSessionOutcomes(sessionID)
	}
	outcomes, err := slim.LoadSessionScorecardOutcomes(sessionID)
	if err == nil {
		s.markDegraded(false)
		return outcomes, nil
	}
	if s.jsonl == nil {
		return nil, fmt.Errorf("load session scorecard outcomes from postgres: %w", err)
	}
	fallback, ferr := s.jsonl.LoadSessionOutcomes(sessionID)
	if ferr != nil {
		return nil, fmt.Errorf("postgres unavailable (%v) and jsonl fallback failed: %w", err, ferr)
	}
	s.markDegraded(true)
	return fallback, nil
}

// LoadOutcomes reads the whole outcome table, PostgreSQL first with the same
// error-only JSONL fallback as LoadSessionSummaries: only a FAILED PG read
// degrades, a usable-but-empty one is authoritative (a reconcile/backfill
// concern, never a reason to silently mix backends).
//
// This used to delegate straight to PG while its sibling reads degraded, which
// gave one store two different availability semantics that no caller could see
// from the interface (FU-20261016-08). Callers — internal/calibration's
// darwinian seam among them — had to carry their own fallback as a result.
func (s *PGFirstOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	outcomes, err := s.pg.LoadOutcomes()
	if err == nil {
		s.markDegraded(false)
		return outcomes, nil
	}
	if s.jsonl == nil {
		return nil, fmt.Errorf("load outcomes from postgres: %w", err)
	}
	fallback, ferr := s.jsonl.LoadOutcomes()
	if ferr != nil {
		return nil, fmt.Errorf("postgres unavailable (%v) and jsonl fallback failed: %w", err, ferr)
	}
	s.markDegraded(true)
	return fallback, nil
}

// LoadOutcomesFromSessions aggregates the per-session outcome files. Same
// semantics as LoadOutcomes, and the same asymmetry fix: the JSONL side reads
// the richer per-session source, and that source is used ONLY when PostgreSQL
// errors.
func (s *PGFirstOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	outcomes, err := s.pg.LoadOutcomesFromSessions()
	if err == nil {
		s.markDegraded(false)
		return outcomes, nil
	}
	if s.jsonl == nil {
		return nil, fmt.Errorf("load outcomes from sessions: %w", err)
	}
	fallback, ferr := s.jsonl.LoadOutcomesFromSessions()
	if ferr != nil {
		return nil, fmt.Errorf("postgres unavailable (%v) and jsonl fallback failed: %w", err, ferr)
	}
	s.markDegraded(true)
	return fallback, nil
}

// ---- the rest of OutcomeStore is the write path: it delegates to PG. ----

func (s *PGFirstOutcomeStore) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	return s.pg.RecordOutcomes(outcomes)
}

func (s *PGFirstOutcomeStore) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	return s.pg.RecordSessionOutcomes(session, outcomes)
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

// ---- window family (window summary + mutation brief) ----
//
// The window family used to be entirely file-based on both sides, so a
// PostgreSQL deployment could not see its own window summaries. The PG-first
// wrapper carries the window surface for the same reason it carries the
// outcome surface: PostgreSQL is the SSoT once ATLAS_STORE_BACKEND=postgres,
// reads fall back to the JSONL ledger ONLY when PostgreSQL errors (a
// successful-but-empty PostgreSQL stays authoritative — a backfill concern,
// see cmd/migrate-data -windows), and every fallback marks the store degraded
// so the caller can log/flag it.

// pgWindowStore is the PostgreSQL half of the window surface.
func (s *PGFirstOutcomeStore) pgWindowStore() (WindowStore, error) {
	store, ok := s.pg.(WindowStore)
	if !ok {
		return nil, fmt.Errorf("postgres store %T does not implement ledger.WindowStore", s.pg)
	}
	return store, nil
}

// jsonlWindowReader is the JSONL half of the window READ surface (the fallback
// target); nil when the store was built without a JSONL directory.
func (s *PGFirstOutcomeStore) jsonlWindowReader() (WindowSummaryReader, bool) {
	reader, ok := s.jsonl.(WindowSummaryReader)
	return reader, ok
}

// RecordWindowSummary writes the summary to PostgreSQL. Write paths do not
// fall back: a PostgreSQL failure must be visible (the window summary is the
// input of the report/health reads), not silently redirected to a JSONL file
// no longer in the read path.
func (s *PGFirstOutcomeStore) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	pg, err := s.pgWindowStore()
	if err != nil {
		return err
	}
	return pg.RecordWindowSummary(summary)
}

// RecordMutationBrief writes the brief to PostgreSQL (same no-fallback policy
// as RecordWindowSummary).
func (s *PGFirstOutcomeStore) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	pg, err := s.pgWindowStore()
	if err != nil {
		return err
	}
	return pg.RecordMutationBrief(windowID, brief)
}

// LoadWindowSummaries reads PostgreSQL first; the JSONL window directory is
// used only when PostgreSQL errors (degraded).
func (s *PGFirstOutcomeStore) LoadWindowSummaries() ([]domain.BacktestWindowSummary, error) {
	pg, err := s.pgWindowStore()
	if err != nil {
		return nil, err
	}
	summaries, err := pg.LoadWindowSummaries()
	if err == nil {
		s.markDegraded(false)
		return summaries, nil
	}
	fallback, ok := s.jsonlWindowReader()
	if !ok {
		return nil, fmt.Errorf("load window summaries from postgres: %w", err)
	}
	jsonlSummaries, ferr := fallback.LoadWindowSummaries()
	if ferr != nil {
		return nil, fmt.Errorf("postgres unavailable (%v) and jsonl fallback failed: %w", err, ferr)
	}
	s.markDegraded(true)
	return jsonlSummaries, nil
}

// LoadWindowSummary reads one window summary, PostgreSQL first with the same
// error-only JSONL fallback as LoadWindowSummaries.
func (s *PGFirstOutcomeStore) LoadWindowSummary(windowID string) (domain.BacktestWindowSummary, bool, error) {
	pg, err := s.pgWindowStore()
	if err != nil {
		return domain.BacktestWindowSummary{}, false, err
	}
	summary, found, err := pg.LoadWindowSummary(windowID)
	if err == nil {
		s.markDegraded(false)
		if found {
			return summary, true, nil
		}
		// PostgreSQL answered authoritatively: the window is not there. Do not
		// mask that with a pre-migration JSONL file.
		return domain.BacktestWindowSummary{}, false, nil
	}
	fallback, ok := s.jsonlWindowReader()
	if !ok {
		return domain.BacktestWindowSummary{}, false, fmt.Errorf("load window summary %s from postgres: %w", windowID, err)
	}
	jsonlSummary, found, ferr := fallback.LoadWindowSummary(windowID)
	if ferr != nil {
		return domain.BacktestWindowSummary{}, false, fmt.Errorf("postgres unavailable (%v) and jsonl fallback failed: %w", err, ferr)
	}
	s.markDegraded(true)
	return jsonlSummary, found, nil
}

var (
	_ OutcomeStore = (*PGFirstOutcomeStore)(nil)
	// WindowStore ⇒ BacktestStore: without the write half here, the handler
	// that injects this store into backtest.Runner (BacktestService.WithStore,
	// dashboard_api.go) ran the whole simulation and then failed with
	// "store does not implement BacktestStore" — the window summary was never
	// persisted anywhere (regression pinned by
	// internal/monitoring/service/backtest_window_store_test.go).
	_ WindowStore = (*PGFirstOutcomeStore)(nil)
)
