//go:build integration

package system

// PostgreSQL half of Q2 batch 2, unit 2: the capital-phase session-history
// read must follow the deployment's declared backend, because production no
// longer writes the JSONL ledger for L-cold data (ATLAS_STORE_BACKEND=postgres;
// the session summaries live in PG session_summaries). A reader that still
// opened LedgerDir/sessions/*/summary.json would then answer empty (or stale)
// — the capital series, its drawdown and its rolling Sharpe would silently
// diverge from the performance report.
//
// These tests run against the real server (DATABASE_URL; skipped locally when
// unset, fatal in CI — internal/testdb policy) and pin the fact through the
// production wiring (ledger.NewReportOutcomeStore, i.e. declaration → store,
// not a hand-built stub):
//
//  1. direction — with the backend declared postgres the capital series carries
//     the PostgreSQL rows (900,000 / non-zero drawdown); the control (same
//     fixture, declaration flipped to jsonl) must carry the JSONL rows (505,000)
//     instead, so "always JSONL" cannot satisfy the claim;
//  2. fallback — an unreachable PostgreSQL still leaves a working read path
//     (the JSONL ledger, labeled degraded by SourceBackend()/Degraded()).

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// q2b2FileSummaries is the JSONL-side fixture: a DIFFERENT, monotonic series
// (500,000 → 505,000), so which backend answered is readable from the payload.
func q2b2FileSummaries() []domain.SessionSummary {
	return []domain.SessionSummary{
		{SessionID: q2b2Session13, PortfolioValue: 500_000, EndingCash: 200_000, RecordedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)},
		{SessionID: q2b2Session14, PortfolioValue: 505_000, EndingCash: 205_000, RecordedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)},
	}
}

func TestCapitalPhase_PostgresServesTheHistoryWhenDeclared(t *testing.T) {
	pool := testdb.Pool(t, q2b2MigrationsPath)
	q2b2CleanPG(t, pool)
	q2b2SeedPG(t, pool)

	dir := t.TempDir()
	q2b2WriteSummaries(t, dir, q2b2FileSummaries())

	snap := q2b2CapitalPhase(t, q2b2Handlers(t, dir).WithStore(q2b2PostgresStore(t, pool, dir)))
	if snap.TotalCapital != 900_000 {
		t.Errorf("total_capital = %v, want 900000 from postgres (505000 would mean the JSONL ledger answered)", snap.TotalCapital)
	}
	if snap.MaxDrawdown == 0 {
		t.Error("max_drawdown = 0; the postgres series dips 1020000 → 900000, so the JSONL series was read")
	}
	if snap.Source != "postgres" {
		t.Errorf("source = %q, want postgres", snap.Source)
	}

	// Control: identical fixtures, only the declaration changes.
	control := q2b2CapitalPhase(t, q2b2Handlers(t, dir).WithStore(q2b2JSONLStore(t, dir)))
	if control.TotalCapital != 505_000 {
		t.Errorf("declared jsonl: total_capital = %v, want 505000 from the JSONL ledger", control.TotalCapital)
	}
}

func TestCapitalPhase_FallsBackToJSONLWhenPostgresUnreachable(t *testing.T) {
	dir := t.TempDir()
	q2b2WriteSummaries(t, dir, q2b2FileSummaries())

	store := q2b2PostgresStore(t, testdb.UnconnectedPool(t, q2b2UnreachableDSN), dir)
	snap := q2b2CapitalPhase(t, q2b2Handlers(t, dir).WithStore(store))

	if snap.TotalCapital != 505_000 {
		t.Errorf("total_capital = %v, want 505000; an unreachable postgres must degrade to the JSONL ledger", snap.TotalCapital)
	}
	if snap.Source != "jsonl" || !snap.Degraded {
		t.Errorf("source/degraded = %q/%v, want jsonl/true (the degraded read must be visible to operators)", snap.Source, snap.Degraded)
	}
}

// ── PostgreSQL fixtures (shared by the integration tests) ─────────────

// q2b2MigrationsPath is the repo-relative migrations directory from this
// package (internal/monitoring/api/system → repo root).
const q2b2MigrationsPath = "../../../../sql/migrations"

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
	summaries := q2b2Summaries()
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
