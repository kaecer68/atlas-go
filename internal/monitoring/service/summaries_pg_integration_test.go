//go:build integration

package service

// PostgreSQL half of Q2 batch 2, unit 2: the monitoring services' L-cold
// session-summary reads (LiveService equity curve / portfolio history,
// BacktestService run window) must follow the deployment's declared backend,
// because production no longer writes the JSONL ledger for L-cold data
// (ATLAS_STORE_BACKEND=postgres; session summaries live in PG
// session_summaries). A service stuck on the JSONL ledger would build an empty
// equity curve (and a backtest window over no sessions) on production.
//
// These tests run against the real server (DATABASE_URL; skipped locally when
// unset, fatal in CI — internal/testdb policy) and pin the facts through the
// production wiring (ledger.NewReportOutcomeStore, i.e. declaration → store):
//
//  1. direction — with the backend declared postgres the equity curve carries
//     the PostgreSQL series (3 points, last 900,000); the control (same fixture,
//     declaration flipped to jsonl) must carry the JSONL series (2 points,
//     505,000) instead, so "always JSONL" cannot satisfy the claim;
//  2. fallback — an unreachable PostgreSQL still leaves a working read path
//     (the JSONL ledger, labeled degraded by SourceBackend()/Degraded()).

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

func TestLiveService_PostgresServesTheHistoryWhenDeclared(t *testing.T) {
	pool := testdb.Pool(t, q2b2MigrationsPath)
	q2b2CleanPG(t, pool)
	q2b2SeedPG(t, pool)

	dir := t.TempDir()
	q2b2WriteSummaries(t, dir, q2b2FileSummary())

	store := q2b2PostgresStore(t, pool, dir)
	svc := NewLiveService(t.TempDir(), dir).WithSummaryStore(store)
	points := svc.HistoryPoints()
	if len(points) != 3 {
		t.Fatalf("HistoryPoints = %d points, want the 3 postgres sessions (2 would mean the JSONL ledger answered)", len(points))
	}
	if got := points[len(points)-1].PortfolioValue; got != 900_000 {
		t.Errorf("latest portfolio value = %v, want 900000 from postgres", got)
	}
	if src := svc.HistorySource(); src != "postgres" {
		t.Errorf("HistorySource = %q, want postgres", src)
	}

	// Control: identical fixtures, only the declaration changes.
	control := NewLiveService(t.TempDir(), dir).WithSummaryStore(q2b2JSONLStore(t, dir))
	if got := len(control.HistoryPoints()); got != 2 {
		t.Errorf("declared jsonl: HistoryPoints = %d points, want 2 from the JSONL ledger", got)
	}
}

func TestLiveService_FallsBackToJSONLWhenPostgresUnreachable(t *testing.T) {
	dir := t.TempDir()
	q2b2WriteSummaries(t, dir, q2b2FileSummary())

	store := q2b2PostgresStore(t, testdb.UnconnectedPool(t, q2b2UnreachableDSN), dir)
	svc := NewLiveService(t.TempDir(), dir).WithSummaryStore(store)
	points := svc.HistoryPoints()

	if len(points) != 2 || points[len(points)-1].PortfolioValue != 505_000 {
		t.Fatalf("HistoryPoints = %+v, want the 2 JSONL sessions ending at 505000 (unreachable postgres must degrade)", points)
	}
	if src := svc.HistorySource(); src != "jsonl" {
		t.Errorf("HistorySource = %q, want jsonl (the degraded read must be visible to operators)", src)
	}
	if !svc.HistoryDegraded() {
		t.Error("HistoryDegraded = false, want true after the JSONL fallback")
	}
}

func TestBacktestService_PostgresServesTheHistoryWhenDeclared(t *testing.T) {
	pool := testdb.Pool(t, q2b2MigrationsPath)
	q2b2CleanPG(t, pool)
	q2b2SeedPG(t, pool)

	dir := t.TempDir()
	q2b2WriteSummaries(t, dir, q2b2FileSummary())
	cfg := config.Config{LedgerDir: dir}

	summaries, err := NewBacktestService(cfg).WithStore(q2b2PostgresStore(t, pool, dir)).summariesStore().LoadSessionSummaries()
	if err != nil {
		t.Fatalf("postgres LoadSessionSummaries: %v", err)
	}
	if len(summaries) != 3 {
		t.Errorf("summaries = %d, want 3 from postgres (2 would mean the JSONL ledger answered)", len(summaries))
	}

	// Control: identical fixtures, only the declaration changes.
	control, err := NewBacktestService(cfg).WithStore(q2b2JSONLStore(t, dir)).summariesStore().LoadSessionSummaries()
	if err != nil {
		t.Fatalf("jsonl LoadSessionSummaries: %v", err)
	}
	if len(control) != 2 {
		t.Errorf("declared jsonl: summaries = %d, want 2 from the JSONL ledger", len(control))
	}
}

// ── PostgreSQL fixtures (shared by the integration tests) ─────────────

// q2b2MigrationsPath is the repo-relative migrations directory from this
// package (internal/monitoring/service → repo root).
const q2b2MigrationsPath = "../../../sql/migrations"

// q2b2UnreachableDSN points at a closed port: the pool parses the DSN and never
// dials, so any read fails with a connection error (same trick as the C6c and
// Q2b2-1 fallback tests).
const q2b2UnreachableDSN = "postgres://atlas:atlas@127.0.0.1:1/atlas_q2b2_unreachable?sslmode=disable"

// q2b2PGSessionIDs are fixture session ids that sort above every real
// `session-YYYYMMDD-*` id (year 9999) and never collide with a producer; the
// tests delete exactly these rows before and after each run.
var q2b2PGSessionIDs = []string{
	"session-99991229-q2b2",
	"session-99991230-q2b2",
	"session-99991231-q2b2",
}

// q2b2PGSummaries maps the hermetic fixture values onto the PG fixture session
// ids, so the two halves of the suite disagree on the backend, not on the data.
func q2b2PGSummaries() []domain.SessionSummary {
	summaries := q2b2StoreSummary()
	for i, id := range q2b2PGSessionIDs {
		summaries[i].SessionID = id
		summaries[i].Regime = domain.RegimeNeutral
	}
	return summaries
}

// q2b2SeedPG writes the fixture session summaries to PostgreSQL.
func q2b2SeedPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	pg := ledger.NewPostgresLedgerStore(pool)
	for _, summary := range q2b2PGSummaries() {
		if err := pg.RecordSessionSummary(domain.ReplaySession{ID: summary.SessionID}, summary); err != nil {
			t.Fatalf("seed postgres session summary %s: %v", summary.SessionID, err)
		}
	}
}

// q2b2CleanPG deletes exactly the fixture rows (before and after each test), so
// the shared database is left as found even when a test fails half way.
func q2b2CleanPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	run := func() {
		ctx := context.Background()
		for _, id := range q2b2PGSessionIDs {
			if _, err := pool.Exec(ctx, "DELETE FROM session_summaries WHERE session_id = $1", id); err != nil {
				t.Fatalf("clean session_summaries for %s: %v", id, err)
			}
		}
	}
	run()
	t.Cleanup(run)
}

// q2b2PostgresStore builds the production store for a declared-postgres
// deployment: the shared pool is injected exactly like the cmd/atlas wiring
// does, and the config carries the JSONL fallback directory.
func q2b2PostgresStore(t *testing.T, pool *pgxpool.Pool, ledgerDir string) ledger.OutcomeStore {
	t.Helper()
	ledger.SetPostgresPool(pool)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })
	store, err := ledger.NewReportOutcomeStore(config.Config{LedgerDir: ledgerDir, StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewReportOutcomeStore(postgres): %v", err)
	}
	return store
}

// q2b2JSONLStore is the control: the same factory with the declaration flipped
// to jsonl, so no PostgreSQL read can happen.
func q2b2JSONLStore(t *testing.T, ledgerDir string) ledger.OutcomeStore {
	t.Helper()
	store, err := ledger.NewReportOutcomeStore(config.Config{LedgerDir: ledgerDir, StoreBackend: "jsonl"})
	if err != nil {
		t.Fatalf("NewReportOutcomeStore(jsonl): %v", err)
	}
	return store
}
