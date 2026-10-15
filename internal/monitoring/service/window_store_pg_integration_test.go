//go:build integration

package service

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

// PostgreSQL half of Q2 batch 2, T2 (window family).
//
// The producers (weekly window_backtest task, daily auto-backtest,
// /api/backtest/start) persist the window summary + mutation brief through the
// backend-aware factory, so on ATLAS_STORE_BACKEND=postgres the family lives in
// PostgreSQL. These tests pin the reader half through the production wiring
// (ledger.NewWindowStore, i.e. declaration → store):
//
//  1. direction — with postgres declared and populated, the report and the
//     system-health window age come from PostgreSQL even though older JSONL
//     files are present on disk (those files are exactly what a stale reader
//     would serve);
//  2. the write really lands in the PG tables (checked with SQL, not with the
//     store's own read path);
//  3. an unreachable PostgreSQL degrades to the JSONL window directory and
//     labels the read, instead of failing the report.

const q2b2wWindowPrefix = "window-q2b2w-"

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

func q2b2wSummary(windowID string, generatedAt time.Time, sessions int) domain.BacktestWindowSummary {
	return domain.BacktestWindowSummary{
		WindowID:        windowID,
		StartDate:       generatedAt.AddDate(0, 0, -20),
		EndDate:         generatedAt,
		SessionCount:    sessions,
		OutcomeCount:    sessions * 3,
		GeneratedAt:     generatedAt,
		WorstAgentID:    "q2b2w-agent",
		WorstAgentSkill: "q2b2w-skill",
	}
}

// q2b2wPostgresWindowStore is the store the production wiring injects
// (dashboard_api.go: ledger.NewWindowStore(storeCfg)); the pool is injected
// exactly like cmd/atlas does at bootstrap.
func q2b2wPostgresWindowStore(t *testing.T, pool *pgxpool.Pool, ledgerDir string) ledger.WindowStore {
	t.Helper()
	ledger.SetPostgresPool(pool)
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })
	store, err := ledger.NewWindowStore(config.Config{LedgerDir: ledgerDir, StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewWindowStore(postgres): %v", err)
	}
	return store
}

func q2b2wSeedWindows(t *testing.T, dir string, summaries ...domain.BacktestWindowSummary) {
	t.Helper()
	store := ledger.NewStore(dir).(*ledger.Store)
	for _, summary := range summaries {
		if err := store.RecordWindowSummary(summary); err != nil {
			t.Fatalf("seed JSONL window %s: %v", summary.WindowID, err)
		}
	}
}

func TestWindowFamily_PostgresServesTheReportAndHealth(t *testing.T) {
	pool := testdb.Pool(t, q2b2MigrationsPath)
	q2b2wCleanPG(t, pool)

	dir := t.TempDir()
	// Pre-migration history on disk: the reader must not serve it once the
	// declared backend is populated.
	stale := q2b2wSummary(q2b2wWindowPrefix+"stale", time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC), 1)
	q2b2wSeedWindows(t, dir, stale)

	store := q2b2wPostgresWindowStore(t, pool, dir)
	windowID := q2b2wWindowPrefix + "report"
	summary := q2b2wSummary(windowID, time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), 6)
	if err := store.RecordWindowSummary(summary); err != nil {
		t.Fatalf("RecordWindowSummary: %v", err)
	}
	if err := store.RecordMutationBrief(windowID, domain.MutationBrief{
		ContractVersion: 1,
		WindowID:        windowID,
		TargetAgentID:   "q2b2w-agent",
		MutationType:    "prompt_tightening",
		GeneratedAt:     summary.GeneratedAt,
	}); err != nil {
		t.Fatalf("RecordMutationBrief: %v", err)
	}

	// The write really reached the PG tables.
	var rows int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM window_summaries WHERE window_id = $1", windowID).Scan(&rows); err != nil {
		t.Fatalf("count window_summaries: %v", err)
	}
	if rows != 1 {
		t.Fatalf("window_summaries rows for %s = %d, want 1", windowID, rows)
	}
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM mutation_briefs WHERE window_id = $1", windowID).Scan(&rows); err != nil {
		t.Fatalf("count mutation_briefs: %v", err)
	}
	if rows != 1 {
		t.Fatalf("mutation_briefs rows for %s = %d, want 1", windowID, rows)
	}

	// Report: the newest window in the declared backend, and only that one.
	reportSvc := NewReportService(dir, dir, ledger.NewStore(dir).(*ledger.Store)).WithWindowStore(store)
	list, err := reportSvc.LoadReportList()
	if err != nil {
		t.Fatalf("LoadReportList: %v", err)
	}
	if len(list) != 1 || list[0].Filename != "backtest_"+windowID+".md" {
		t.Fatalf("LoadReportList = %+v, want exactly the postgres window %s", list, windowID)
	}
	content, filename, err := reportSvc.LoadLatestReport()
	if err != nil {
		t.Fatalf("LoadLatestReport: %v", err)
	}
	if filename != "backtest_"+windowID+".md" {
		t.Errorf("report filename = %q, want the postgres window", filename)
	}
	if len(content) == 0 {
		t.Error("rendered report is empty")
	}

	// System health: last window from the declared backend, no false
	// "找不到回測窗口摘要" while the stale file exists.
	systemSvc := NewSystemService(dir, dir, testBaselinePolicyPath(t), nil, nil, nil).WithWindowStore(store)
	health, err := systemSvc.LoadSystemHealth()
	if err != nil {
		t.Fatalf("LoadSystemHealth: %v", err)
	}
	if health.LastWindowID != windowID {
		t.Errorf("LastWindowID = %q, want the postgres window %s", health.LastWindowID, windowID)
	}
	if !health.LastWindowGeneratedAt.Equal(summary.GeneratedAt) {
		t.Errorf("LastWindowGeneratedAt = %v, want %v", health.LastWindowGeneratedAt, summary.GeneratedAt)
	}
	if contains(health.Warnings, "找不到回測窗口摘要") {
		t.Errorf("warnings = %v; the declared backend answered, so this is a false positive", health.Warnings)
	}
}

func TestWindowFamily_UnreachablePostgresDegradesToTheJSONLWindows(t *testing.T) {
	dir := t.TempDir()
	window := q2b2wSummary(q2b2wWindowPrefix+"fallback", time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), 2)
	q2b2wSeedWindows(t, dir, window)

	store := q2b2wPostgresWindowStore(t, testdb.UnconnectedPool(t, q2b2UnreachableDSN), dir)

	reportSvc := NewReportService(dir, dir, ledger.NewStore(dir).(*ledger.Store)).WithWindowStore(store)
	list, err := reportSvc.LoadReportList()
	if err != nil {
		t.Fatalf("LoadReportList: %v", err)
	}
	if len(list) != 1 || list[0].Filename != "backtest_"+window.WindowID+".md" {
		t.Fatalf("LoadReportList = %+v, want the JSONL fallback window", list)
	}
	if source, ok := store.(interface{ SourceBackend() string }); !ok || source.SourceBackend() != "jsonl" {
		t.Errorf("SourceBackend() = %v, want jsonl after the fallback", store)
	}
	if degraded, ok := store.(interface{ Degraded() bool }); !ok || !degraded.Degraded() {
		t.Errorf("Degraded() = %v, want true after the fallback", store)
	}
}
