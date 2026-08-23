package ledger

import (
	"errors"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// stubOutcomeStore implements OutcomeStore with programmable read behavior.
// All write methods panic — the PG-first store delegates writes to PG and the
// tests here only exercise reads.
type stubOutcomeStore struct {
	summaries    []domain.SessionSummary
	outcomes     []domain.RecommendationOutcome
	summariesErr error
	outcomesErr  error
}

func (s *stubOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return s.summaries, s.summariesErr
}

func (s *stubOutcomeStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	return s.outcomes, s.outcomesErr
}

func (s *stubOutcomeStore) RecordOutcomes([]domain.RecommendationOutcome) error { panic("unused") }
func (s *stubOutcomeStore) RecordSessionOutcomes(domain.ReplaySession, []domain.RecommendationOutcome) error {
	panic("unused")
}
func (s *stubOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) { panic("unused") }
func (s *stubOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	panic("unused")
}
func (s *stubOutcomeStore) RecordSessionScreeningRejects(string, []domain.ScreeningReject) error {
	panic("unused")
}
func (s *stubOutcomeStore) LoadSessionScreeningRejects(string) ([]domain.ScreeningReject, error) {
	panic("unused")
}
func (s *stubOutcomeStore) RecordSessionTrades(string, []domain.TradeRecord) error { panic("unused") }
func (s *stubOutcomeStore) LoadSessionTrades(string) ([]domain.TradeRecord, error) { panic("unused") }
func (s *stubOutcomeStore) LoadAllSessionTrades() ([]domain.TradeRecord, error)    { panic("unused") }
func (s *stubOutcomeStore) RecordExperiment(domain.ExperimentRecord) error         { panic("unused") }
func (s *stubOutcomeStore) RecordSessionExperiment(domain.ReplaySession, domain.ExperimentRecord) error {
	panic("unused")
}
func (s *stubOutcomeStore) RecordSessionSummary(domain.ReplaySession, domain.SessionSummary) error {
	panic("unused")
}
func (s *stubOutcomeStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	panic("unused")
}
func (s *stubOutcomeStore) RecordHumanIntervention(domain.HumanIntervention) error { panic("unused") }
func (s *stubOutcomeStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	panic("unused")
}

var _ OutcomeStore = (*stubOutcomeStore)(nil)

func TestPGFirstOutcomeStore_PGFull(t *testing.T) {
	pg := &stubOutcomeStore{summaries: []domain.SessionSummary{{SessionID: "pg-1"}}}
	jsonl := &stubOutcomeStore{summaries: []domain.SessionSummary{{SessionID: "jsonl-1"}}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries: %v", err)
	}
	if len(summaries) != 1 || summaries[0].SessionID != "pg-1" {
		t.Fatalf("expected PG summaries, got %+v", summaries)
	}
	if store.Degraded() {
		t.Error("Degraded() = true, want false (PG healthy)")
	}
	if store.SourceBackend() != "postgres" {
		t.Errorf("SourceBackend() = %q, want postgres", store.SourceBackend())
	}
	if store.FallbackCount() != 0 {
		t.Errorf("FallbackCount() = %d, want 0", store.FallbackCount())
	}
}

func TestPGFirstOutcomeStore_PGUnavailable_FallsBackDegraded(t *testing.T) {
	pg := &stubOutcomeStore{summariesErr: errors.New("connection refused")}
	jsonl := &stubOutcomeStore{summaries: []domain.SessionSummary{{SessionID: "jsonl-1"}}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries should fall back, got err: %v", err)
	}
	if len(summaries) != 1 || summaries[0].SessionID != "jsonl-1" {
		t.Fatalf("expected JSONL fallback summaries, got %+v", summaries)
	}
	if !store.Degraded() {
		t.Error("Degraded() = false, want true (PG unavailable)")
	}
	if store.SourceBackend() != "jsonl" {
		t.Errorf("SourceBackend() = %q, want jsonl", store.SourceBackend())
	}
	if store.FallbackCount() != 1 {
		t.Errorf("FallbackCount() = %d, want 1", store.FallbackCount())
	}
}

func TestPGFirstOutcomeStore_PGUnavailable_BothFail(t *testing.T) {
	pg := &stubOutcomeStore{summariesErr: errors.New("connection refused")}
	jsonl := &stubOutcomeStore{summariesErr: errors.New("io error")}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	if _, err := store.LoadSessionSummaries(); err == nil {
		t.Fatal("LoadSessionSummaries() = nil error, want error when both sides fail")
	}
}

func TestPGFirstOutcomeStore_PGUnavailable_NoFallbackConfigured(t *testing.T) {
	pg := &stubOutcomeStore{summariesErr: errors.New("connection refused")}
	store := NewPGFirstOutcomeStore(pg, nil)

	if _, err := store.LoadSessionSummaries(); err == nil {
		t.Fatal("LoadSessionSummaries() = nil error, want error when fallback is disabled")
	}
}

func TestPGFirstOutcomeStore_PGEmptyIsAuthoritative(t *testing.T) {
	// SSoT contract: a usable-but-empty PG is authoritative — the JSONL side
	// must NOT be silently mixed in. The 8-fix cycle was caused by exactly
	// that kind of multi-backend semantic drift.
	pg := &stubOutcomeStore{summaries: nil} // usable, empty
	jsonl := &stubOutcomeStore{summaries: []domain.SessionSummary{{SessionID: "jsonl-1"}}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		t.Fatalf("LoadSessionSummaries: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected authoritative empty PG result, got %+v", summaries)
	}
	if store.Degraded() {
		t.Error("Degraded() = true, want false (PG usable and authoritative)")
	}
	if store.SourceBackend() != "postgres" {
		t.Errorf("SourceBackend() = %q, want postgres", store.SourceBackend())
	}
}

func TestPGFirstOutcomeStore_OutcomesFallback(t *testing.T) {
	pg := &stubOutcomeStore{outcomesErr: errors.New("connection refused")}
	jsonl := &stubOutcomeStore{outcomes: []domain.RecommendationOutcome{{AgentID: "a1"}}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	outcomes, err := store.LoadSessionOutcomes("session-x")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes should fall back, got err: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].AgentID != "a1" {
		t.Fatalf("expected JSONL fallback outcomes, got %+v", outcomes)
	}
	if !store.Degraded() {
		t.Error("Degraded() = false, want true")
	}
}

func TestPGFirstOutcomeStore_OutcomesPGEmptyIsAuthoritative(t *testing.T) {
	pg := &stubOutcomeStore{outcomes: nil} // usable, empty
	jsonl := &stubOutcomeStore{outcomes: []domain.RecommendationOutcome{{AgentID: "jsonl"}}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	outcomes, err := store.LoadSessionOutcomes("session-x")
	if err != nil {
		t.Fatalf("LoadSessionOutcomes: %v", err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("expected authoritative empty PG outcomes, got %+v", outcomes)
	}
	if store.Degraded() {
		t.Error("Degraded() = true, want false")
	}
}

func TestPGFirstOutcomeStore_RecoversAfterPGReturns(t *testing.T) {
	pg := &stubOutcomeStore{summariesErr: errors.New("connection refused")}
	jsonl := &stubOutcomeStore{summaries: []domain.SessionSummary{{SessionID: "jsonl-1"}}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	if _, err := store.LoadSessionSummaries(); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if !store.Degraded() {
		t.Fatal("expected degraded after PG failure")
	}

	// PG recovers.
	pg.summariesErr = nil
	pg.summaries = []domain.SessionSummary{{SessionID: "pg-1"}}
	if _, err := store.LoadSessionSummaries(); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if store.Degraded() {
		t.Error("Degraded() = true after PG recovery, want false")
	}
	if store.SourceBackend() != "postgres" {
		t.Errorf("SourceBackend() = %q, want postgres", store.SourceBackend())
	}
	if store.FallbackCount() != 1 {
		t.Errorf("FallbackCount() = %d, want 1 (only the failed read)", store.FallbackCount())
	}
}
