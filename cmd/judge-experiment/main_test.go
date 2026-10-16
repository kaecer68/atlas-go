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

func TestRunRejectsMissingResult(t *testing.T) {
	if err := run([]string{"--result", "/nonexistent/path.json"}); err == nil {
		t.Fatalf("expected error for missing result file")
	}
}

func TestRunJudgesValidExperimentResult(t *testing.T) {
	dir := t.TempDir()

	// Setup prompt file
	promptPath := filepath.Join(dir, "v2.md")
	if err := os.WriteFile(promptPath, []byte("# Candidate\nrequire trend confirmation\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	// Setup brief
	brief := domain.MutationBrief{
		ContractVersion:     domain.MutationBriefContractVersion,
		WindowID:            "window-20260326-20260327",
		TargetAgentID:       "growth-momentum-01",
		TargetSkill:         "growth_momentum",
		TargetLayer:         domain.LayerStyle,
		PromptFile:          filepath.Join("..", "..", "prompts", "agents", "growth_momentum.md"),
		MutationType:        "prompt_tightening",
		AcceptanceMetric:    "sharpe_like",
		AcceptanceGates:     []string{"improve_sharpe_like"},
		ForbiddenActions:    []string{"illiquid_breakout_chasing"},
		RequiredSkills:      []string{"growth_momentum"},
		ObservedWindowCount: 2,
		MaturityLevel:       "level_1_exploratory",
		GeneratedAt:         time.Now(),
	}

	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:              "exec-growth-momentum-01-test",
			ProposalID:      "proposal-test",
			TargetAgentID:   "growth-momentum-01",
			Skill:           "growth_momentum",
			MutationType:    "prompt_tightening",
			AcceptanceGates: []string{"improve_sharpe_like"},
			BaselineValue:   0,
			CandidateValue:  0,
			Status:          domain.ExperimentRunning,
			WindowStart:     time.Now().AddDate(0, 0, -10),
			WindowEnd:       time.Now(),
		},
		Brief:           brief,
		CandidatePrompt: promptPath,
		EvaluationMode:  "policy_checked_pending_replay",
		RecordedAt:      time.Now(),
	}

	experimentsDir := filepath.Join(dir, "experiments")
	os.MkdirAll(experimentsDir, 0o755)
	resultPath := filepath.Join(experimentsDir, "result.json")
	b, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(resultPath, b, 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	// Setup window summary (judge.go infers window dir as sibling of experiments dir)
	windowSummary := domain.BacktestWindowSummary{
		WindowID:  "window-20260326-20260327",
		StartDate: time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
	}
	windowsDir := filepath.Join(dir, "windows")
	os.MkdirAll(windowsDir, 0o755)
	wb, _ := json.MarshalIndent(windowSummary, "", "  ")
	os.WriteFile(filepath.Join(windowsDir, "window-20260326-20260327.json"), wb, 0o644)

	// Set env overrides
	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origReplay := os.Getenv("ATLAS_REPLAY_DATA_PATH")
	origBaseline := os.Getenv("ATLAS_BASELINE_POLICY_PATH")
	// Q2 batch 2 unit A: run() now builds a backend-aware store, so under
	// ATLAS_STORE_BACKEND=sqlite (make test-backend / ci-full) the package
	// directory's default sqlite path would be used — and it does not exist,
	// which correctly fails loudly. Point it at the test's temp dir so the test
	// stays deterministic under every backend instead of depending on the
	// developer's env.
	origSQLite := os.Getenv("ATLAS_SQLITE_PATH")
	defer func() {
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_REPLAY_DATA_PATH", origReplay)
		os.Setenv("ATLAS_BASELINE_POLICY_PATH", origBaseline)
		os.Setenv("ATLAS_SQLITE_PATH", origSQLite)
	}()
	os.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	os.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	os.Setenv("ATLAS_BASELINE_POLICY_PATH", filepath.Join(dir, "baseline.json"))

	if err := run([]string{"--result", resultPath}); err != nil {
		t.Fatalf("run judge-experiment: %v", err)
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
