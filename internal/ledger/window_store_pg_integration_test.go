//go:build integration

package ledger

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// Q2 batch 2, T2 (window family) — the PostgreSQL half.
//
// It pins the claim the file-only producers could not satisfy: a window
// summary / mutation brief written through the production factory
// (ATLAS_STORE_BACKEND=postgres) really lands in the PG tables that the UI
// readers (report + system health) then read back. It also pins the two
// semantics the JSONL store had for free and the relational stores had to
// learn: re-writing one window refreshes the row (upsert, no duplicate-key
// failure) and a pre-migration JSONL file never masks a usable-but-empty
// PostgreSQL.

const q2b2wWindowPrefix = "window-q2b2w-"

// q2b2wCleanPG deletes only this suite's fixture rows (before and after), so
// the shared database is left as found even when a test fails half way.
func q2b2wCleanPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	run := func() {
		ctx := context.Background()
		if _, err := pool.Exec(ctx, "DELETE FROM window_summaries WHERE window_id LIKE $1", q2b2wWindowPrefix+"%"); err != nil {
			t.Fatalf("clean window_summaries: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM mutation_briefs WHERE window_id LIKE $1", q2b2wWindowPrefix+"%"); err != nil {
			t.Fatalf("clean mutation_briefs: %v", err)
		}
	}
	run()
	t.Cleanup(run)
}

// q2b2wRowCount counts the fixture rows a production write produced.
func q2b2wRowCount(t *testing.T, pool *pgxpool.Pool, table, windowID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM "+table+" WHERE window_id = $1", windowID).Scan(&count); err != nil {
		t.Fatalf("count %s rows for %s: %v", table, windowID, err)
	}
	return count
}

// q2b2wSource reports the backend that answered plus the degraded flag. The
// window seam itself stays narrow (no SourceBackend/Degraded on the interface);
// the PG-first wrapper advertises them and the callers log them when present.
func q2b2wSource(t *testing.T, store WindowStore) (string, bool) {
	t.Helper()
	reporter, ok := store.(interface {
		SourceBackend() string
		Degraded() bool
	})
	if !ok {
		t.Fatalf("store %T does not report its read source", store)
	}
	return reporter.SourceBackend(), reporter.Degraded()
}

func q2b2wSummary(windowID string, generatedAt time.Time, sessions int) domain.BacktestWindowSummary {
	return domain.BacktestWindowSummary{
		WindowID:        windowID,
		StartDate:       generatedAt.AddDate(0, 0, -20),
		EndDate:         generatedAt,
		SessionCount:    sessions,
		OutcomeCount:    11,
		GeneratedAt:     generatedAt,
		WorstAgentID:    "q2b2w-agent",
		WorstAgentSkill: "q2b2w-skill",
	}
}

// q2b2wPostgresWindowStore builds the production store shape for a declared
// postgres deployment: the shared pool injected exactly like cmd/atlas does,
// with the JSONL directory present as the (unused) fallback.
func q2b2wPostgresWindowStore(t *testing.T, pool *pgxpool.Pool, ledgerDir string) WindowStore {
	t.Helper()
	SetPostgresPool(pool)
	t.Cleanup(func() { SetPostgresPool(nil) })
	store, err := NewWindowStore(config.Config{LedgerDir: ledgerDir, StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewWindowStore(postgres): %v", err)
	}
	return store
}

func TestWindowStore_PostgresRowsAreReadBackThroughTheFactory(t *testing.T) {
	pool := connectTestPG(t)
	q2b2wCleanPG(t, pool)

	jsonlDir := t.TempDir()
	// A pre-migration JSONL window exists on disk: it must NOT be what answers
	// once PostgreSQL is declared and populated (that is exactly the "read the
	// old files" failure mode this unit removes).
	stale := NewStore(jsonlDir).(*Store)
	if err := stale.RecordWindowSummary(q2b2wSummary(q2b2wWindowPrefix+"stale", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 1)); err != nil {
		t.Fatalf("seed stale JSONL window: %v", err)
	}

	store := q2b2wPostgresWindowStore(t, pool, jsonlDir)
	windowID := q2b2wWindowPrefix + "pg"
	summary := q2b2wSummary(windowID, time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), 5)
	if err := store.RecordWindowSummary(summary); err != nil {
		t.Fatalf("RecordWindowSummary: %v", err)
	}
	if err := store.RecordMutationBrief(windowID, domain.MutationBrief{
		ContractVersion: 1,
		WindowID:        windowID,
		TargetAgentID:   "q2b2w-agent",
		MutationType:    "prompt_tightening",
		GeneratedAt:     time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("RecordMutationBrief: %v", err)
	}

	// The row really is in PostgreSQL (the claim "new data reaches PG" is
	// checked against the database, not against the store's own read path).
	if got := q2b2wRowCount(t, pool, "window_summaries", windowID); got != 1 {
		t.Fatalf("window_summaries rows for %s = %d, want 1", windowID, got)
	}
	if got := q2b2wRowCount(t, pool, "mutation_briefs", windowID); got != 1 {
		t.Fatalf("mutation_briefs rows for %s = %d, want 1", windowID, got)
	}

	summaries, err := store.LoadWindowSummaries()
	if err != nil {
		t.Fatalf("LoadWindowSummaries: %v", err)
	}
	found := false
	for _, got := range summaries {
		if got.WindowID == q2b2wWindowPrefix+"stale" {
			t.Fatalf("LoadWindowSummaries served the pre-migration JSONL window %s while postgres was usable", got.WindowID)
		}
		if got.WindowID == windowID {
			found = true
			if got.SessionCount != summary.SessionCount || got.WorstAgentID != summary.WorstAgentID {
				t.Errorf("postgres window payload = %+v, want %+v", got, summary)
			}
		}
	}
	if !found {
		t.Fatalf("LoadWindowSummaries did not return the postgres window %s: %+v", windowID, summaries)
	}

	got, ok, err := store.LoadWindowSummary(windowID)
	if err != nil || !ok || got.SessionCount != summary.SessionCount {
		t.Fatalf("LoadWindowSummary(%s) = %+v (found %v, err %v)", windowID, got, ok, err)
	}
	if source, _ := q2b2wSource(t, store); source != "postgres" {
		t.Errorf("source = %q, want postgres", source)
	}
}

func TestWindowStore_ReWriteSameWindowRefreshesTheRow(t *testing.T) {
	pool := connectTestPG(t)
	q2b2wCleanPG(t, pool)

	store := q2b2wPostgresWindowStore(t, pool, t.TempDir())
	windowID := q2b2wWindowPrefix + "rerun"
	if err := store.RecordWindowSummary(q2b2wSummary(windowID, time.Now(), 5)); err != nil {
		t.Fatalf("RecordWindowSummary: %v", err)
	}
	// A restarted weekly task / a second /api/backtest/start on the same window
	// / an idempotent backfill re-writes the same window id. The JSONL store
	// overwrote the file; the relational store must refresh, not fail.
	if err := store.RecordWindowSummary(q2b2wSummary(windowID, time.Now(), 9)); err != nil {
		t.Fatalf("RecordWindowSummary (same window again): %v", err)
	}
	if got := q2b2wRowCount(t, pool, "window_summaries", windowID); got != 1 {
		t.Fatalf("window_summaries rows after re-write = %d, want 1", got)
	}
	got, ok, err := store.LoadWindowSummary(windowID)
	if err != nil || !ok {
		t.Fatalf("LoadWindowSummary(%s) = found %v, err %v", windowID, ok, err)
	}
	if got.SessionCount != 9 {
		t.Errorf("SessionCount = %d, want 9 (the re-run must win)", got.SessionCount)
	}
}

func TestWindowStore_PGUnavailableFallsBackToJSONLDegraded(t *testing.T) {
	dir := t.TempDir()
	jsonl := NewStore(dir).(*Store)
	windowID := q2b2wWindowPrefix + "fallback"
	if err := jsonl.RecordWindowSummary(q2b2wSummary(windowID, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), 2)); err != nil {
		t.Fatalf("seed JSONL window: %v", err)
	}

	// Closed port: the pool parses the DSN and never dials, so the postgres
	// half errors and the window read degrades to the JSONL ledger.
	pool, err := pgxpool.New(context.Background(),
		"postgres://atlas:atlas@127.0.0.1:1/atlas_q2b2w_unreachable?sslmode=disable")
	if err != nil {
		t.Fatalf("parse unreachable DSN: %v", err)
	}
	t.Cleanup(pool.Close)

	store := NewPGFirstOutcomeStore(NewPostgresLedgerStore(pool), NewStore(dir))
	summaries, err := store.LoadWindowSummaries()
	if err != nil {
		t.Fatalf("LoadWindowSummaries with postgres unreachable: %v", err)
	}
	if len(summaries) != 1 || summaries[0].WindowID != windowID {
		t.Fatalf("LoadWindowSummaries = %+v, want the JSONL fallback window", summaries)
	}
	if source, degraded := q2b2wSource(t, store); !degraded || source != "jsonl" {
		t.Errorf("degraded = %v, source = %q; want true/jsonl after the fallback", degraded, source)
	}

	// Writes must NOT fall back: a window summary that only exists on the local
	// disk of one container is exactly the failure this unit removes.
	if err := store.RecordWindowSummary(q2b2wSummary(q2b2wWindowPrefix+"write", time.Now(), 1)); err == nil {
		t.Error("RecordWindowSummary with postgres unreachable returned nil error; writes must fail loudly")
	}
}

// TestWindowStore_BacktestServiceStoreImplementsBacktestStore pins the
// regression that motivated carrying the window surface on the PG-first
// wrapper: the store injected into backtest.Runner must implement
// ledger.BacktestStore, otherwise the run completes and then fails with
// "store does not implement BacktestStore" without persisting anything.
func TestWindowStore_BacktestServiceStoreImplementsBacktestStore(t *testing.T) {
	pool := connectTestPG(t)
	SetPostgresPool(pool)
	t.Cleanup(func() { SetPostgresPool(nil) })

	store, err := NewReportOutcomeStore(config.Config{LedgerDir: filepath.Join(t.TempDir(), "state"), StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewReportOutcomeStore(postgres): %v", err)
	}
	if _, ok := store.(BacktestStore); !ok {
		t.Fatal("the store injected by BacktestService.WithStore does not implement ledger.BacktestStore")
	}
}
