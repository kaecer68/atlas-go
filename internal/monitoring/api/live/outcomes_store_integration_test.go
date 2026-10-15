//go:build integration

package live

// PostgreSQL half of Q2 batch 2, unit 1: the live handlers' outcome reads must
// follow the deployment's declared backend, because production no longer writes
// the JSONL ledger (ATLAS_STORE_BACKEND=postgres; the stockpicker/ledger writes
// moved to PG first). A reader that still opened
// LedgerDir/sessions/*/recommendation_outcomes.jsonl would then answer empty —
// the P&L attribution and the sector/factor exposure of the risk-exposure
// endpoint would silently zero out on production.
//
// These tests run against the real server (DATABASE_URL; skipped locally when
// unset, fatal in CI — internal/testdb policy) and pin three endpoint-level
// facts through the production wiring (ledger.NewReportOutcomeStore, i.e.
// declaration → store, not a hand-built stub):
//
//  1. direction — with the backend declared postgres, the response carries the
//     PostgreSQL rows; the control (same fixture, declaration flipped to jsonl)
//     must carry the JSONL rows instead, so "always JSONL" cannot satisfy the
//     claim;
//  2. empty ≠ outage — a reachable PostgreSQL holding the session but no
//     outcome rows must NOT fall back to JSONL rows that would fill the
//     response (PGFirstOutcomeStore semantics);
//  3. fallback — an unreachable PostgreSQL still leaves a working read path
//     (the JSONL ledger, labeled degraded by SourceBackend()).
//
// The "latest session" resolution reads session summaries from the backend, so
// the fixture session id sorts above any real `session-YYYYMMDD-*` id (year
// 9999) and its rows are deleted before and after each test: the shared
// database is left as found. No producer writes this id.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// q2b1PGSession sorts above every real session id ("session-YYYYMMDD-*") so the
// handler's latest-session resolution deterministically lands on the fixture.
const q2b1PGSession = "session-99991231-q2b1"

func TestLiveOutcomes_PostgresServesTheEndpointsWhenDeclared(t *testing.T) {
	pool := testdb.Pool(t, q2b1MigrationsPath)
	q2b1CleanPG(t, pool)

	// PostgreSQL: the fixture session, with an outcome row for 2330.
	q2b1SeedPGSession(t, pool)
	pgStore := ledger.NewPostgresLedgerStore(pool)
	if err := pgStore.RecordSessionOutcomes(domain.ReplaySession{ID: q2b1PGSession}, q2b1Outcomes("2330", 0.05)); err != nil {
		t.Fatalf("seed postgres outcomes: %v", err)
	}

	// JSONL ledger: the same session id, a different symbol (2311), so the
	// response names the backend that answered.
	dir := t.TempDir()
	q2b1WriteFixtureWithID(t, dir, q2b1PGSession)

	h := (&Handlers{LedgerDir: filepath.Join(dir, "no-such-ledger"), WorkDir: dir}).
		WithStore(q2b1PostgresStore(t, pool, dir))

	pnl := q2b1GetPnL(t, h)
	if pnl.SessionID != q2b1PGSession {
		t.Fatalf("pnl session_id = %q, want the fixture session %q", pnl.SessionID, q2b1PGSession)
	}
	if got := q2b1AttributedSymbols(pnl); !slices.Equal(got, []string{"2330"}) {
		t.Errorf("pnl symbol_attribution = %v, want [2330] from postgres (2311 would mean the JSONL ledger answered)", got)
	}
	if risk := q2b1GetRisk(t, h); len(risk.SectorExposure) == 0 {
		t.Errorf("risk sector_exposure empty; the postgres outcome rows did not reach the risk-exposure endpoint")
	}

	// Control: identical fixtures, only the declaration changes. The JSONL
	// ledger holds 2311, so the answer must flip — the negative proof that the
	// first half measured the PostgreSQL read.
	control := (&Handlers{LedgerDir: filepath.Join(dir, "no-such-ledger"), WorkDir: dir}).
		WithStore(q2b1JSONLStore(t, dir))
	if got := q2b1AttributedSymbols(q2b1GetPnL(t, control)); !slices.Equal(got, []string{"2311"}) {
		t.Errorf("declared jsonl: symbol_attribution = %v, want [2311] from the JSONL ledger", got)
	}
}

func TestLiveOutcomes_EmptyPostgresDoesNotFallBackToJSONL(t *testing.T) {
	pool := testdb.Pool(t, q2b1MigrationsPath)
	q2b1CleanPG(t, pool)

	// PostgreSQL knows the session (summary only, no outcome rows).
	q2b1SeedPGSession(t, pool)

	dir := t.TempDir()
	q2b1WriteFixtureWithID(t, dir, q2b1PGSession)
	// The JSONL half really holds a row that would fill the response: without
	// this check the empty assertion below could come from a broken fixture.
	jsonlRows, err := ledger.NewStore(dir).LoadSessionOutcomes(q2b1PGSession)
	if err != nil || len(jsonlRows) == 0 {
		t.Fatalf("JSONL fixture rows: n=%d err=%v, want the fixture to have outcomes (otherwise the assertion is vacuous)", len(jsonlRows), err)
	}

	h := (&Handlers{LedgerDir: filepath.Join(dir, "no-such-ledger"), WorkDir: dir}).
		WithStore(q2b1PostgresStore(t, pool, dir))

	pnl := q2b1GetPnL(t, h)
	if pnl.SessionID != q2b1PGSession {
		t.Fatalf("pnl session_id = %q, want %q (postgres holds the summary)", pnl.SessionID, q2b1PGSession)
	}
	if got := q2b1AttributedSymbols(pnl); len(got) != 0 {
		t.Errorf("pnl symbol_attribution = %v with an empty but reachable postgres; the read fell back to the JSONL ledger", got)
	}
	if len(pnl.AgentAttribution) != 0 {
		t.Errorf("pnl agent_attribution = %+v, want none from the authoritative empty postgres", pnl.AgentAttribution)
	}
}

func TestLiveOutcomes_FallBackToJSONLWhenPostgresUnreachable(t *testing.T) {
	dir := t.TempDir()
	q2b1WriteFixtureWithID(t, dir, q2b1PGSession)
	store := q2b1PostgresStore(t, testdb.UnconnectedPool(t, q2b1UnreachableDSN), dir)

	h := (&Handlers{LedgerDir: filepath.Join(dir, "no-such-ledger"), WorkDir: dir}).WithStore(store)
	if got := q2b1AttributedSymbols(q2b1GetPnL(t, h)); !slices.Equal(got, []string{"2311"}) {
		t.Errorf("symbol_attribution = %v, want [2311]; an unreachable postgres must degrade to the JSONL ledger", got)
	}
	if src := q2b1SourceBackend(store); src != "jsonl" {
		t.Errorf("SourceBackend() = %q, want jsonl (the degraded read must be visible to operators)", src)
	}
}

// ── helpers ───────────────────────────────────────────────────────────

// q2b1MigrationsPath is the repo-relative migrations directory from this
// package (internal/monitoring/api/live → repo root).
const q2b1MigrationsPath = "../../../../sql/migrations"

// q2b1PostgresStore builds the production store for a declared-postgres
// deployment: the shared pool is injected exactly like the cmd/atlas wiring
// does, and the config carries the JSONL fallback directory.
func q2b1PostgresStore(t *testing.T, pool *pgxpool.Pool, ledgerDir string) ledger.OutcomeStore {
	t.Helper()
	ledger.SetPostgresPool(pool)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })
	store, err := ledger.NewReportOutcomeStore(config.Config{LedgerDir: ledgerDir, StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewReportOutcomeStore(postgres): %v", err)
	}
	return store
}

// q2b1JSONLStore is the control: the same factory with the declaration flipped
// to jsonl, so no PostgreSQL read can happen.
func q2b1JSONLStore(t *testing.T, ledgerDir string) ledger.OutcomeStore {
	t.Helper()
	store, err := ledger.NewReportOutcomeStore(config.Config{LedgerDir: ledgerDir, StoreBackend: "jsonl"})
	if err != nil {
		t.Fatalf("NewReportOutcomeStore(jsonl): %v", err)
	}
	return store
}

func q2b1SourceBackend(store ledger.OutcomeStore) string {
	if s, ok := store.(interface{ SourceBackend() string }); ok {
		return s.SourceBackend()
	}
	return ""
}

// q2b1SeedPGSession writes only the fixture session summary (no outcomes), so
// the handler's latest-session resolution finds it in PostgreSQL.
func q2b1SeedPGSession(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	summary := domain.SessionSummary{
		SessionID:      q2b1PGSession,
		Regime:         domain.RegimeNeutral,
		PortfolioValue: 1020000,
		OutcomeCount:   1,
		RecordedAt:     time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
	}
	if err := ledger.NewPostgresLedgerStore(pool).RecordSessionSummary(domain.ReplaySession{ID: q2b1PGSession}, summary); err != nil {
		t.Fatalf("seed postgres session summary: %v", err)
	}
}

// q2b1CleanPG deletes exactly the fixture rows (before and after each test), so
// the shared database is left as found even when a test fails half way.
func q2b1CleanPG(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	run := func() {
		ctx := context.Background()
		for _, table := range []string{"recommendation_outcomes", "session_summaries"} {
			if _, err := pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE session_id = $1", table), q2b1PGSession); err != nil {
				t.Fatalf("clean %s for %s: %v", table, q2b1PGSession, err)
			}
		}
	}
	run()
	t.Cleanup(run)
}

// q2b1WriteFixtureWithID writes one session (the given id) whose summary and
// outcome rows live on disk, reusing the hermetic fixtures. The JSONL row uses
// symbol 2311 while the PostgreSQL fixtures use 2330, so which backend answered
// is readable from the response.
func q2b1WriteFixtureWithID(t *testing.T, dir, sessionID string) {
	t.Helper()
	sessionDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	summary := q2b1Summaries()[1]
	summary.SessionID = sessionID
	b, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), b, 0o644); err != nil {
		t.Fatalf("write summary.json: %v", err)
	}
	var lines []string
	for _, oc := range q2b1Outcomes("2311", 0.01) {
		b, err := json.Marshal(oc)
		if err != nil {
			t.Fatalf("marshal outcome: %v", err)
		}
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write recommendation_outcomes.jsonl: %v", err)
	}
}

// q2b1AttributedSymbols lists the symbols the response attributes to, sorted.
func q2b1AttributedSymbols(resp PnLAttributionResponse) []string {
	var symbols []string
	for _, s := range resp.SymbolAttribution {
		symbols = append(symbols, s.Symbol)
	}
	slices.Sort(symbols)
	return symbols
}
