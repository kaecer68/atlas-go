package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

func TestRunExecutesExperiment(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte("# Test prompt\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	replayPath := filepath.Join(dir, "test-replay.csv")
	replayContent := `Date,Code,Name,TradeVolume,Open,High,Low,Close
2026-03-25,0050,Test,1000000,100,110,90,105
2026-03-26,0050,Test,1000000,105,115,95,110
2026-03-27,0050,Test,1000000,110,120,100,115
2026-03-30,0050,Test,1000000,115,125,105,120
2026-03-31,0050,Test,1000000,120,130,110,125
`
	if err := os.WriteFile(replayPath, []byte(replayContent), 0o644); err != nil {
		t.Fatalf("write replay: %v", err)
	}

	brief := domain.MutationBrief{
		ContractVersion:     domain.MutationBriefContractVersion,
		WindowID:            "window-20260325-20260331",
		TargetAgentID:       "test-agent-01",
		TargetSkill:         "test_skill",
		TargetLayer:         domain.LayerStyle,
		PromptFile:          promptPath,
		MutationType:        "prompt_tightening",
		AcceptanceMetric:    "sharpe_like",
		AcceptanceGates:     []string{"improve_sharpe_like"},
		ForbiddenActions:    []string{"bad_action"},
		RequiredSkills:      []string{"test_skill"},
		ObservedWindowCount: 1,
		MaturityLevel:       "level_1_exploratory",
		GeneratedAt:         time.Now(),
	}
	briefPath := filepath.Join(dir, "brief.json")
	b, err := json.Marshal(brief)
	if err != nil {
		t.Fatalf("marshal brief: %v", err)
	}
	if err := os.WriteFile(briefPath, b, 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origBaseline := os.Getenv("ATLAS_BASELINE_POLICY_PATH")
	origReplay := os.Getenv("ATLAS_REPLAY_DATA_PATH")
	// Q2 batch 2 unit A: run() now builds a backend-aware store. Under
	// ATLAS_STORE_BACKEND=sqlite (make test-backend / ci-full) the default
	// sqlite path would be resolved inside this package directory and leave an
	// atlas.db behind; pin it to the test's temp dir so the run is hermetic
	// under every backend.
	origSQLite := os.Getenv("ATLAS_SQLITE_PATH")
	defer func() {
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_BASELINE_POLICY_PATH", origBaseline)
		os.Setenv("ATLAS_REPLAY_DATA_PATH", origReplay)
		os.Setenv("ATLAS_SQLITE_PATH", origSQLite)
	}()
	os.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	os.Setenv("ATLAS_BASELINE_POLICY_PATH", filepath.Join(dir, "baseline.json"))
	os.Setenv("ATLAS_REPLAY_DATA_PATH", replayPath)

	if err := run([]string{"--brief", briefPath}); err != nil {
		t.Fatalf("run run-experiment: %v", err)
	}
}

func TestRunRejectsInvalidBrief(t *testing.T) {
	dir := t.TempDir()
	briefPath := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(briefPath, []byte(`{"invalid":true}`), 0o644); err != nil {
		t.Fatalf("write invalid brief: %v", err)
	}

	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origBaseline := os.Getenv("ATLAS_BASELINE_POLICY_PATH")
	origSQLite := os.Getenv("ATLAS_SQLITE_PATH")
	defer func() {
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_BASELINE_POLICY_PATH", origBaseline)
		os.Setenv("ATLAS_SQLITE_PATH", origSQLite)
	}()
	os.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	os.Setenv("ATLAS_BASELINE_POLICY_PATH", filepath.Join(dir, "baseline.json"))

	if err := run([]string{"--brief", briefPath}); err == nil {
		t.Fatalf("expected error for invalid brief contract")
	}
}

// ─── store seam 測試（Q2 batch 2 unit A） ────────────────────────────────────
//
// hermetic：不需要真 PostgreSQL。以 fake deps 驗證「要不要開池、用什麼 DSN、
// 失敗時是否明確報錯（不靜默退回 JSONL）」。

func TestOpenExperimentStore_NonPostgresNeverTouchesThePool(t *testing.T) {
	for _, backend := range []string{"", "jsonl", "sqlite"} {
		dir := t.TempDir()
		deps := storeDeps{
			initPool: func(context.Context, string, string) (*pgxpool.Pool, error) {
				t.Fatalf("backend %q must not open a PostgreSQL pool", backend)
				return nil, nil
			},
			injectPool: func(*pgxpool.Pool) {
				t.Fatalf("backend %q must not inject a pool", backend)
			},
		}
		store, closeStore, err := openExperimentStore(context.Background(), config.Config{
			StoreBackend: backend,
			LedgerDir:    dir,
			SQLitePath:   filepath.Join(dir, "atlas.db"),
			DatabaseURL:  "postgres://ignored/should-not-be-used",
		}, deps)
		if err != nil {
			t.Fatalf("backend %q: %v", backend, err)
		}
		if store == nil {
			t.Fatalf("backend %q: nil store", backend)
		}
		closeStore()
	}
}

func TestOpenExperimentStore_PostgresWithoutDSNFailsLoudly(t *testing.T) {
	initCalled := false
	deps := storeDeps{
		initPool: func(context.Context, string, string) (*pgxpool.Pool, error) {
			initCalled = true
			return nil, nil
		},
		injectPool: func(*pgxpool.Pool) {},
	}

	store, _, err := openExperimentStore(context.Background(), config.Config{
		StoreBackend: "postgres",
		LedgerDir:    t.TempDir(),
	}, deps)
	if err == nil {
		t.Fatalf("postgres without a DSN must fail, got store %T", store)
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error must name DATABASE_URL, got: %v", err)
	}
	if initCalled {
		t.Fatal("no pool may be opened without a DSN")
	}
}

func TestOpenExperimentStore_PostgresOpensAndInjectsThePool(t *testing.T) {
	var gotDSN, gotMigrations string
	var injected *pgxpool.Pool
	deps := storeDeps{
		initPool: func(_ context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error) {
			gotDSN, gotMigrations = dsn, migrationsPath
			return testdb.UnconnectedPool(t, "postgres://user:pw@127.0.0.1:1/db"), nil
		},
		injectPool: func(pool *pgxpool.Pool) {
			injected = pool
			// 真的注入到 ledger factory：這樣 store 才會走到 postgres 分支，
			// 測試驗的是「注入後建得起來」，不只是「有呼叫 inject」。
			ledger.SetPostgresPool(pool)
		},
	}
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })

	cfg := config.Config{
		StoreBackend:   "postgres",
		LedgerDir:      t.TempDir(),
		DatabaseURL:    "postgres://user:pw@127.0.0.1:1/db",
		MigrationsPath: "sql/migrations",
	}
	store, closeStore, err := openExperimentStore(context.Background(), cfg, deps)
	if err != nil {
		t.Fatalf("openExperimentStore: %v", err)
	}
	defer closeStore()

	if gotDSN != cfg.DatabaseURL {
		t.Fatalf("initPool dsn = %q, want the configured DATABASE_URL", gotDSN)
	}
	if gotMigrations != "sql/migrations" {
		t.Fatalf("initPool migrations = %q, want the configured path", gotMigrations)
	}
	if injected == nil {
		t.Fatal("the pool must be injected into the ledger factory before the store is built")
	}
	if store == nil {
		t.Fatal("nil store")
	}
}

func TestOpenExperimentStore_UnknownBackendFailsLoudly(t *testing.T) {
	deps := storeDeps{
		initPool:   func(context.Context, string, string) (*pgxpool.Pool, error) { return nil, nil },
		injectPool: func(*pgxpool.Pool) {},
	}
	if _, _, err := openExperimentStore(context.Background(), config.Config{StoreBackend: "postgre"}, deps); err == nil {
		t.Fatal("unknown backend must fail")
	}
}
