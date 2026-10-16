package ledger

// FU-20261016-08 — PGFirstOutcomeStore.LoadOutcomes / LoadOutcomesFromSessions
// had no JSONL fallback.
//
// The store's summary reads (LoadSessionSummaries / LoadSessionOutcomes /
// LoadAllSessionTrades) are PG-first and fall back to the JSONL ledger ONLY
// when PostgreSQL ERRORS, marking the store degraded; the two whole-table
// outcome reads delegated straight to PG instead. One store therefore had two
// availability semantics that no caller could tell apart from the interface,
// and internal/calibration's darwinian seam had to carry its own fallback
// because of it.
//
// These tests pin (a) the repaired semantics for the two reads and (b) the
// symmetry itself: every read in the store answers the same fixture the same
// way, so a future read method cannot quietly be added with the old shape.

import (
	"errors"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func outcomeFixture(agent string) domain.RecommendationOutcome {
	return domain.RecommendationOutcome{AgentID: agent, Symbol: "2330"}
}

func summaryFixture(session string) domain.SessionSummary {
	return domain.SessionSummary{SessionID: session}
}

// TestPGFirstOutcomeStore_AllOutcomesFallback: a FAILED PG read degrades to the
// JSONL ledger and says so.
func TestPGFirstOutcomeStore_AllOutcomesFallback(t *testing.T) {
	pg := &stubOutcomeStore{allErr: errors.New("connection refused")}
	jsonl := &stubOutcomeStore{allOutcomes: []domain.RecommendationOutcome{outcomeFixture("jsonl")}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes should fall back to JSONL, got err: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].AgentID != "jsonl" {
		t.Fatalf("expected JSONL fallback outcomes, got %+v", outcomes)
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
	if pg.readCounts["LoadOutcomes"] != 1 {
		t.Errorf("PG LoadOutcomes reached %d times, want 1 (PG must be tried first)", pg.readCounts["LoadOutcomes"])
	}
}

// TestPGFirstOutcomeStore_AllOutcomesFromSessionsFallback is the per-session
// twin: the JSONL side answers from the richer per-session files, and it does so
// only because PG failed.
func TestPGFirstOutcomeStore_AllOutcomesFromSessionsFallback(t *testing.T) {
	pg := &stubOutcomeStore{fromSessErr: errors.New("connection refused")}
	jsonl := &stubOutcomeStore{allFromSess: []domain.RecommendationOutcome{outcomeFixture("session-jsonl")}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	outcomes, err := store.LoadOutcomesFromSessions()
	if err != nil {
		t.Fatalf("LoadOutcomesFromSessions should fall back to JSONL, got err: %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].AgentID != "session-jsonl" {
		t.Fatalf("expected JSONL per-session fallback outcomes, got %+v", outcomes)
	}
	if !store.Degraded() || store.SourceBackend() != "jsonl" {
		t.Errorf("degraded=%v source=%q, want degraded jsonl", store.Degraded(), store.SourceBackend())
	}
	if pg.readCounts["LoadOutcomesFromSessions"] != 1 {
		t.Errorf("PG LoadOutcomesFromSessions reached %d times, want 1", pg.readCounts["LoadOutcomesFromSessions"])
	}
}

// TestPGFirstOutcomeStore_AllOutcomesPGEmptyIsAuthoritative: a usable-but-empty
// PG is the SSoT answer. Reading the JSONL mirror here would re-introduce the
// multi-backend mixing the SSoT decision removed.
func TestPGFirstOutcomeStore_AllOutcomesPGEmptyIsAuthoritative(t *testing.T) {
	pg := &stubOutcomeStore{allOutcomes: nil} // usable, empty
	jsonl := &stubOutcomeStore{allOutcomes: []domain.RecommendationOutcome{outcomeFixture("jsonl")}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	outcomes, err := store.LoadOutcomes()
	if err != nil {
		t.Fatalf("LoadOutcomes: %v", err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("expected the authoritative empty PG result, got %+v", outcomes)
	}
	if store.Degraded() {
		t.Error("Degraded() = true, want false (PG usable and authoritative)")
	}
	if store.SourceBackend() != "postgres" {
		t.Errorf("SourceBackend() = %q, want postgres", store.SourceBackend())
	}
	if store.FallbackCount() != 0 {
		t.Errorf("FallbackCount() = %d, want 0 (no fallback on a successful read)", store.FallbackCount())
	}
}

// TestPGFirstOutcomeStore_AllOutcomesNoFallbackConfigured: with no JSONL side
// (PG-only deployment) the read must fail, not answer empty.
func TestPGFirstOutcomeStore_AllOutcomesNoFallbackConfigured(t *testing.T) {
	boom := errors.New("connection refused")
	pg := &stubOutcomeStore{allErr: boom, fromSessErr: boom}
	store := NewPGFirstOutcomeStore(pg, nil)

	if _, err := store.LoadOutcomes(); err == nil {
		t.Fatal("LoadOutcomes() = nil error, want an error when the fallback is disabled")
	}
	if _, err := store.LoadOutcomesFromSessions(); err == nil {
		t.Fatal("LoadOutcomesFromSessions() = nil error, want an error when the fallback is disabled")
	}
}

// TestPGFirstOutcomeStore_AllOutcomesBothSidesFail: the error must name both
// sides — an operator cannot act on "load failed".
func TestPGFirstOutcomeStore_AllOutcomesBothSidesFail(t *testing.T) {
	pg := &stubOutcomeStore{allErr: errors.New("connection refused")}
	jsonl := &stubOutcomeStore{allErr: errors.New("io error")}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	_, err := store.LoadOutcomes()
	if err == nil {
		t.Fatal("LoadOutcomes() = nil error, want an error when both sides fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "connection refused") || !strings.Contains(msg, "io error") {
		t.Fatalf("error %q must name both failures", msg)
	}
}

// TestPGFirstOutcomeStore_AllOutcomesRecoversAfterPGReturns: the degraded flag
// is per read, not sticky.
func TestPGFirstOutcomeStore_AllOutcomesRecoversAfterPGReturns(t *testing.T) {
	pg := &stubOutcomeStore{allErr: errors.New("connection refused")}
	jsonl := &stubOutcomeStore{allOutcomes: []domain.RecommendationOutcome{outcomeFixture("jsonl")}}
	store := NewPGFirstOutcomeStore(pg, jsonl)

	if _, err := store.LoadOutcomes(); err != nil {
		t.Fatalf("first read: %v", err)
	}
	if !store.Degraded() {
		t.Fatal("expected degraded after the PG failure")
	}

	pg.allErr = nil
	pg.allOutcomes = []domain.RecommendationOutcome{outcomeFixture("pg")}
	if _, err := store.LoadOutcomes(); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if store.Degraded() {
		t.Error("Degraded() = true after PG recovery, want false")
	}
	if store.FallbackCount() != 1 {
		t.Errorf("FallbackCount() = %d, want 1 (only the failed read)", store.FallbackCount())
	}
}

// readProbe adapts every read of the store to one shape so the contract can be
// asserted across all of them at once.
type readProbe struct {
	name string
	read func(*PGFirstOutcomeStore) (int, error)
}

func outcomeReadProbes() []readProbe {
	return []readProbe{
		{"LoadSessionSummaries", func(s *PGFirstOutcomeStore) (int, error) {
			got, err := s.LoadSessionSummaries()
			return len(got), err
		}},
		{"LoadSessionOutcomes", func(s *PGFirstOutcomeStore) (int, error) {
			got, err := s.LoadSessionOutcomes("session-fu101608")
			return len(got), err
		}},
		{"LoadSessionScorecardOutcomes", func(s *PGFirstOutcomeStore) (int, error) {
			got, err := s.LoadSessionScorecardOutcomes("session-fu101608")
			return len(got), err
		}},
		{"LoadOutcomes", func(s *PGFirstOutcomeStore) (int, error) {
			got, err := s.LoadOutcomes()
			return len(got), err
		}},
		{"LoadOutcomesFromSessions", func(s *PGFirstOutcomeStore) (int, error) {
			got, err := s.LoadOutcomesFromSessions()
			return len(got), err
		}},
		{"LoadAllSessionTrades", func(s *PGFirstOutcomeStore) (int, error) {
			got, err := s.LoadAllSessionTrades()
			return len(got), err
		}},
	}
}

// jsonlFilled returns a JSONL stub with one row in every read.
func jsonlFilled() *stubOutcomeStore {
	return &stubOutcomeStore{
		summaries:   []domain.SessionSummary{summaryFixture("jsonl")},
		outcomes:    []domain.RecommendationOutcome{outcomeFixture("jsonl")},
		allOutcomes: []domain.RecommendationOutcome{outcomeFixture("jsonl")},
		allFromSess: []domain.RecommendationOutcome{outcomeFixture("jsonl")},
		trades:      []domain.TradeRecord{{Symbol: "2330"}},
	}
}

// pgEmpty returns a PG stub that answers successfully with nothing.
func pgEmpty() *stubOutcomeStore { return &stubOutcomeStore{} }

// pgBroken returns a PG stub that fails every read.
func pgBroken() *stubOutcomeStore {
	boom := errors.New("connection refused")
	return &stubOutcomeStore{
		summariesErr: boom,
		outcomesErr:  boom,
		allErr:       boom,
		fromSessErr:  boom,
		tradesErr:    boom,
	}
}

// TestPGFirstOutcomeStore_ReadContractIsSymmetric is the parity pin: every read
// of the store must answer the same fixture identically. The asymmetry this
// change removes was invisible per method and only shows up when the whole
// contract is asserted at once.
func TestPGFirstOutcomeStore_ReadContractIsSymmetric(t *testing.T) {
	t.Run("PG errors fall back and report degraded", func(t *testing.T) {
		for _, probe := range outcomeReadProbes() {
			store := NewPGFirstOutcomeStore(pgBroken(), jsonlFilled())
			n, err := probe.read(store)
			if err != nil {
				t.Errorf("%s: err = %v, want a JSONL fallback", probe.name, err)
				continue
			}
			if n != 1 {
				t.Errorf("%s: rows = %d, want 1 from the JSONL fallback", probe.name, n)
			}
			if !store.Degraded() {
				t.Errorf("%s: Degraded() = false, want true", probe.name)
			}
			if store.SourceBackend() != "jsonl" {
				t.Errorf("%s: SourceBackend() = %q, want jsonl", probe.name, store.SourceBackend())
			}
			if store.FallbackCount() != 1 {
				t.Errorf("%s: FallbackCount() = %d, want 1", probe.name, store.FallbackCount())
			}
		}
	})

	t.Run("a usable but empty PG is authoritative", func(t *testing.T) {
		for _, probe := range outcomeReadProbes() {
			store := NewPGFirstOutcomeStore(pgEmpty(), jsonlFilled())
			n, err := probe.read(store)
			if err != nil {
				t.Errorf("%s: err = %v, want nil", probe.name, err)
				continue
			}
			if n != 0 {
				t.Errorf("%s: rows = %d, want the authoritative empty PG result (JSONL must not be mixed in)", probe.name, n)
			}
			if store.Degraded() {
				t.Errorf("%s: Degraded() = true, want false", probe.name)
			}
			if store.SourceBackend() != "postgres" {
				t.Errorf("%s: SourceBackend() = %q, want postgres", probe.name, store.SourceBackend())
			}
		}
	})

	t.Run("no fallback configured fails loudly", func(t *testing.T) {
		for _, probe := range outcomeReadProbes() {
			store := NewPGFirstOutcomeStore(pgBroken(), nil)
			if _, err := probe.read(store); err == nil {
				t.Errorf("%s: err = nil, want an error (PG failed, no JSONL configured)", probe.name)
			}
		}
	})
}
