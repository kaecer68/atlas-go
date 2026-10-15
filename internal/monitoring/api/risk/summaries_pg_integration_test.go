//go:build integration

package risk

// PostgreSQL half of Q2 batch 2, unit 2: the risk handlers' L-cold reads
// (session-summary history behind /api/dashboard/risk, persisted commentary
// behind /api/risk/commentary) must follow the deployment's declared backend,
// because production no longer writes the JSONL ledger for L-cold data
// (ATLAS_STORE_BACKEND=postgres; session summaries live in PG
// session_summaries). A reader stuck on the JSONL ledger would answer stale (or
// empty) VaR inputs and a blank commentary after a restart.
//
// These tests run against the real server (DATABASE_URL; skipped locally when
// unset, fatal in CI — internal/testdb policy) and pin the facts through the
// production wiring (ledger.NewReportOutcomeStore, i.e. declaration → store):
//
//  1. direction — with the backend declared postgres the risk metrics carry the
//     PostgreSQL series (3 sessions / 2 returns); the control (same fixture,
//     declaration flipped to jsonl) must carry the JSONL series (2 sessions)
//     instead, so "always JSONL" cannot satisfy the claim;
//  2. commentary — the persisted risk_commentary is served from PG as
//     source=session_summaries_pg;
//  3. fallback — an unreachable PostgreSQL still leaves a working read path
//     (the JSONL ledger, labeled degraded in the payload).

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

func TestRiskMetrics_PostgresServesTheHistoryWhenDeclared(t *testing.T) {
	pool := testdb.Pool(t, q2b2MigrationsPath)
	q2b2CleanPG(t, pool)
	q2b2SeedPG(t, pool)

	dir := t.TempDir()
	q2b2WriteSummaries(t, dir, q2b2FileSummaries())

	gate := risk.NewRiskGate(nil, nil, nil)
	payload := q2b2RiskMetrics(t, NewHandlers(dir).WithRiskGate(gate).WithStore(q2b2PostgresStore(t, pool, dir)))
	if got := payload["session_count"]; got != float64(3) {
		t.Errorf("session_count = %v, want 3 from postgres (2 would mean the JSONL ledger answered)", got)
	}
	if got := q2b2Snapshot(t, payload)["data_points"]; got != float64(2) {
		t.Errorf("data_points = %v, want 2 (the postgres series has 3 points ⇒ 2 returns)", got)
	}
	if got := payload["source"]; got != "postgres" {
		t.Errorf("source = %v, want postgres", got)
	}

	// Control: identical fixtures, only the declaration changes.
	control := q2b2RiskMetrics(t, NewHandlers(dir).WithRiskGate(risk.NewRiskGate(nil, nil, nil)).WithStore(q2b2JSONLStore(t, dir)))
	if got := control["session_count"]; got != float64(2) {
		t.Errorf("declared jsonl: session_count = %v, want 2 from the JSONL ledger", got)
	}
}

func TestRiskCommentary_PostgresServesThePersistedCommentary(t *testing.T) {
	pool := testdb.Pool(t, q2b2MigrationsPath)
	q2b2CleanPG(t, pool)
	q2b2SeedPG(t, pool)

	// The newest fixture session (year 9999) carries the commentary; the JSONL
	// side stays empty, so only PostgreSQL can answer.
	commentary := "風險水位穩定，曝險低於閾值，維持現行配置。"
	newest := q2b2PGSummaries()[2]
	newest.RiskCommentary = commentary
	pg := ledger.NewPostgresLedgerStore(pool)
	if err := pg.RecordSessionSummary(domain.ReplaySession{ID: newest.SessionID}, newest); err != nil {
		t.Fatalf("seed postgres commentary: %v", err)
	}

	dir := t.TempDir()
	h := NewHandlers(dir).WithRiskGate(risk.NewRiskGate(nil, nil, nil)).WithStore(q2b2PostgresStore(t, pool, dir))

	payload := q2b2RiskCommentary(t, h)
	if got := payload["session_id"]; got != newest.SessionID {
		t.Errorf("session_id = %v, want %s (the postgres commentary row)", got, newest.SessionID)
	}
	if got := payload["reason_zh"]; got != commentary {
		t.Errorf("reason_zh = %v, want the persisted commentary", got)
	}
	if got := payload["source"]; got != "session_summaries_pg" {
		t.Errorf("source = %v, want session_summaries_pg", got)
	}
}

func TestRiskMetrics_FallsBackToJSONLWhenPostgresUnreachable(t *testing.T) {
	dir := t.TempDir()
	q2b2WriteSummaries(t, dir, q2b2FileSummaries())

	store := q2b2PostgresStore(t, testdb.UnconnectedPool(t, q2b2UnreachableDSN), dir)
	payload := q2b2RiskMetrics(t, NewHandlers(dir).WithRiskGate(risk.NewRiskGate(nil, nil, nil)).WithStore(store))

	if got := payload["session_count"]; got != float64(2) {
		t.Errorf("session_count = %v, want 2; an unreachable postgres must degrade to the JSONL ledger", got)
	}
	if got := payload["source"]; got != "jsonl" {
		t.Errorf("source = %v, want jsonl (the degraded read must be visible to consumers)", got)
	}
	if got := payload["degraded"]; got != true {
		t.Errorf("degraded = %v, want true after the JSONL fallback", got)
	}
	if _, ok := q2b2Snapshot(t, payload)["insufficient_data"]; !ok {
		t.Error("insufficient_data missing from the degraded payload")
	}
}

// ── PostgreSQL fixtures (shared by the integration tests) ─────────────

// q2b2MigrationsPath is the repo-relative migrations directory from this
// package (risk → repo root).
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

// q2b2FileSummaries is the JSONL-side fixture: a DIFFERENT, monotonic series
// (500,000 → 505,000), so which backend answered is readable from the payload.
func q2b2FileSummaries() []domain.SessionSummary {
	return []domain.SessionSummary{
		{SessionID: q2b2Session13, PortfolioValue: 500_000, EndingCash: 200_000, RecordedAt: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)},
		{SessionID: q2b2Session14, PortfolioValue: 505_000, EndingCash: 205_000, RecordedAt: time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)},
	}
}
