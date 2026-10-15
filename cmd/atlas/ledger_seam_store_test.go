package main

// ledger_seam_store_test.go — Q2 batch 2, unit B+C (sites 1 and 2).
//
// Two production sites in cmd/atlas used to build a raw JSONL ledger store
// (ledger.NewStore(...)) for session-summary reads:
//
//   * site 1 — the daily report's regime provider (main.go): under
//     ATLAS_STORE_BACKEND=postgres the daily sim writes session summaries to PG
//     and the flat ledger is only a 12h reconcile mirror (reconcile_task.go), so
//     the report could pick a regime a day stale.
//   * site 2 — the stage-3 pipeline service (stage3_tasks.go): the instance only
//     serves LoadRegimeHistory (which reads the injected historicalStore), so it
//     is a latent seam, but a raw JSONL store there would read the mirror the
//     moment another pipeline method is called on it.
//
// These tests pin the seam: an injected backend-aware store decides the answer,
// the factory decides the backend, and a store that cannot be built falls back
// to the pre-migration disk behavior (nil) instead of silently reading JSONL.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// stubOutcomeStore implements only the read the regime provider uses; the
// embedded nil interface satisfies the rest of ledger.OutcomeStore (any other
// call would panic, which is exactly what a test should notice) and lets the
// test observe that the store — not the disk — answered.
type stubOutcomeStore struct {
	ledger.OutcomeStore
	summaries []domain.SessionSummary
	err       error
	calls     int
}

func (s *stubOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	s.calls++
	return s.summaries, s.err
}

var _ ledger.OutcomeStore = (*stubOutcomeStore)(nil)

// writeSummaryFixture writes the flat-ledger mirror file
// <ledgerDir>/sessions/<sessionID>/summary.json the way the JSONL store does.
func writeSummaryFixture(t *testing.T, ledgerDir, sessionID string, regime domain.Regime, outcomeCount int) {
	t.Helper()
	dir := filepath.Join(ledgerDir, "sessions", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir session fixture: %v", err)
	}
	summary := domain.SessionSummary{
		SessionID:    sessionID,
		Regime:       regime,
		OutcomeCount: outcomeCount,
		RecordedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0o644); err != nil {
		t.Fatalf("write summary fixture: %v", err)
	}
}

// withoutPostgresPool makes the postgres branch of the ledger factories fail
// deterministically (they must never fall back to JSONL on their own).
func withoutPostgresPool(t *testing.T) {
	t.Helper()
	ledger.SetPostgresPool(nil)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })
}

// TestSite1_LatestSessionRegime_StoreWinsOverStaleDiskMirror is the hermetic
// "injected ⇒ goes through the store" proof: the store answers with the newest
// session (RISK_ON) while the flat mirror on disk still holds the stale one
// (RISK_OFF), and the nil path is asserted to return the stale value so the two
// paths provably differ.
func TestSite1_LatestSessionRegime_StoreWinsOverStaleDiskMirror(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFixture(t, dir, "session-20260101-daily", domain.RegimeRiskOff, 0)

	store := &stubOutcomeStore{summaries: []domain.SessionSummary{{
		SessionID:    "session-20260102-daily",
		Regime:       domain.RegimeRiskOn,
		OutcomeCount: 7,
		RecordedAt:   time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}}}

	if got := latestSessionRegime(store, dir); got != domain.RegimeRiskOn {
		t.Fatalf("store-backed regime = %q, want %q (the store must decide, not the disk mirror)", got, domain.RegimeRiskOn)
	}
	if store.calls != 1 {
		t.Fatalf("LoadSessionSummaries called %d times, want 1", store.calls)
	}

	// Control: the pre-migration (nil store) path still reads the stale mirror.
	if got := latestSessionRegime(nil, dir); got != domain.RegimeRiskOff {
		t.Fatalf("nil-store regime = %q, want %q (disk mirror behavior must be unchanged)", got, domain.RegimeRiskOff)
	}
}

// TestSite1_LatestSessionRegime_NilStoreKeepsDiskBehavior pins "nil ⇒ original
// behavior" and the parity of the two paths on an identical fixture: the
// backend-aware JSONL store reads exactly what the disk scan reads.
func TestSite1_LatestSessionRegime_NilStoreKeepsDiskBehavior(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFixture(t, dir, "session-20260101-daily", domain.RegimeRiskOff, 0)
	writeSummaryFixture(t, dir, "session-20260102-daily", domain.RegimeRiskOn, 3)

	nilRegime := latestSessionRegime(nil, dir)
	if nilRegime != domain.RegimeRiskOn {
		t.Fatalf("nil-store regime = %q, want %q (newest session with outcomes)", nilRegime, domain.RegimeRiskOn)
	}

	jsonlStore := newRegimeStore(config.Config{LedgerDir: dir, StoreBackend: "jsonl"})
	if jsonlStore == nil {
		t.Fatal("newRegimeStore(jsonl) = nil, want a JSONL store")
	}
	if got := latestSessionRegime(jsonlStore, dir); got != nilRegime {
		t.Fatalf("parity: store path = %q, disk path = %q, want identical", got, nilRegime)
	}

	// An empty ledger reports NEUTRAL through both paths (no fixture ⇒ no data).
	empty := t.TempDir()
	if got := latestSessionRegime(nil, empty); got != domain.RegimeNeutral {
		t.Fatalf("empty nil-store regime = %q, want %q", got, domain.RegimeNeutral)
	}
	if got := latestSessionRegime(newRegimeStore(config.Config{LedgerDir: empty, StoreBackend: "jsonl"}), empty); got != domain.RegimeNeutral {
		t.Fatalf("empty store regime = %q, want %q", got, domain.RegimeNeutral)
	}
}

// TestSite1_LatestSessionRegime_StoreErrorFallsBackToDisk documents the error
// contract: a failing store read degrades to the disk scan (FindLatestSessionSummary
// behavior), it never panics and never reports a fabricated regime.
func TestSite1_LatestSessionRegime_StoreErrorFallsBackToDisk(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFixture(t, dir, "session-20260101-daily", domain.RegimeRiskOff, 0)

	store := &stubOutcomeStore{err: errors.New("pg unavailable")}
	if got := latestSessionRegime(store, dir); got != domain.RegimeRiskOff {
		t.Fatalf("store-error regime = %q, want the disk fallback %q", got, domain.RegimeRiskOff)
	}
	if store.calls != 1 {
		t.Fatalf("LoadSessionSummaries called %d times, want 1", store.calls)
	}
}

// TestSite1_NewRegimeStore_BackendDispatch pins that the store factory decides
// the backend and that a buildable-but-undeclared backend fails closed (nil ⇒
// the historical disk path) instead of silently answering from JSONL.
func TestSite1_NewRegimeStore_BackendDispatch(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFixture(t, dir, "session-20260101-daily", domain.RegimeRiskOff, 0)

	t.Run("jsonl", func(t *testing.T) {
		withoutPostgresPool(t)
		store := newRegimeStore(config.Config{LedgerDir: dir, StoreBackend: "jsonl"})
		if store == nil {
			t.Fatal("newRegimeStore(jsonl) = nil, want store")
		}
		if _, ok := store.(*ledger.Store); !ok {
			t.Fatalf("newRegimeStore(jsonl) = %T, want *ledger.Store", store)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		withoutPostgresPool(t)
		store := newRegimeStore(config.Config{
			LedgerDir:    dir,
			SQLitePath:   filepath.Join(t.TempDir(), "atlas.db"),
			StoreBackend: "sqlite",
		})
		if store == nil {
			t.Fatal("newRegimeStore(sqlite) = nil, want store")
		}
		if _, ok := store.(*ledger.Store); ok {
			t.Fatal("newRegimeStore(sqlite) fell back to the JSONL store")
		}
	})

	t.Run("postgres without pool is nil", func(t *testing.T) {
		withoutPostgresPool(t)
		if store := newRegimeStore(config.Config{LedgerDir: dir, StoreBackend: "postgres"}); store != nil {
			t.Fatalf("newRegimeStore(postgres, no pool) = %T, want nil (no silent JSONL downgrade)", store)
		}
	})

	t.Run("unknown backend is nil", func(t *testing.T) {
		withoutPostgresPool(t)
		if store := newRegimeStore(config.Config{LedgerDir: dir, StoreBackend: "bogus"}); store != nil {
			t.Fatalf("newRegimeStore(bogus) = %T, want nil", store)
		}
	})
}

// TestSite2_Stage3PipelineStore_BackendDispatch is the site-2 counterpart: the
// pipeline service must receive a backend-aware store, and an unresolvable
// backend must yield nil (the pipeline service then keeps its disk fallback)
// rather than a JSONL store that reads the stale mirror.
func TestSite2_Stage3PipelineStore_BackendDispatch(t *testing.T) {
	dir := t.TempDir()
	writeSummaryFixture(t, dir, "session-20260101-daily", domain.RegimeRiskOff, 0)

	t.Run("jsonl", func(t *testing.T) {
		withoutPostgresPool(t)
		store := stage3PipelineStore(config.Config{WorkDir: dir, LedgerDir: dir, StoreBackend: "jsonl"})
		if store == nil {
			t.Fatal("stage3PipelineStore(jsonl) = nil, want store")
		}
		if _, ok := store.(*ledger.Store); !ok {
			t.Fatalf("stage3PipelineStore(jsonl) = %T, want *ledger.Store", store)
		}
		// Parity: the resolved store serves the same summaries as the disk scan.
		storeSummary, err := store.LoadSessionSummaries()
		if err != nil {
			t.Fatalf("LoadSessionSummaries through store: %v", err)
		}
		if len(storeSummary) != 1 || storeSummary[0].SessionID != "session-20260101-daily" {
			t.Fatalf("store summaries = %+v, want the single fixture session", storeSummary)
		}
		if got, want := latestSessionRegime(store, dir), latestSessionRegime(nil, dir); got != want {
			t.Fatalf("parity: store regime = %q, disk regime = %q, want identical", got, want)
		}
	})

	t.Run("sqlite", func(t *testing.T) {
		withoutPostgresPool(t)
		store := stage3PipelineStore(config.Config{
			WorkDir:      dir,
			LedgerDir:    dir,
			SQLitePath:   filepath.Join(t.TempDir(), "atlas.db"),
			StoreBackend: "sqlite",
		})
		if store == nil {
			t.Fatal("stage3PipelineStore(sqlite) = nil, want store")
		}
		if _, ok := store.(*ledger.Store); ok {
			t.Fatal("stage3PipelineStore(sqlite) fell back to the JSONL store")
		}
	})

	t.Run("postgres without pool is nil", func(t *testing.T) {
		withoutPostgresPool(t)
		if store := stage3PipelineStore(config.Config{WorkDir: dir, LedgerDir: dir, StoreBackend: "postgres"}); store != nil {
			t.Fatalf("stage3PipelineStore(postgres, no pool) = %T, want nil", store)
		}
	})

	t.Run("unknown backend is nil", func(t *testing.T) {
		withoutPostgresPool(t)
		if store := stage3PipelineStore(config.Config{WorkDir: dir, LedgerDir: dir, StoreBackend: "bogus"}); store != nil {
			t.Fatalf("stage3PipelineStore(bogus) = %T, want nil", store)
		}
	})
}
