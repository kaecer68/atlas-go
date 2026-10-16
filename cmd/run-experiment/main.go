package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("run-experiment", flag.ContinueOnError)
	brief := fs.String("brief", "data/state/windows/window-20260326-20260327-mutation-brief.json", "mutation brief json path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Load()
	if _, err := baseline.Load(cfg.BaselinePolicyPath); err != nil {
		return fmt.Errorf("load baseline policy: %w", err)
	}
	store, closeStore, err := openExperimentStore(context.Background(), cfg, defaultStoreDeps())
	if err != nil {
		return err
	}
	defer closeStore()
	executor := experiment.NewExecutor(store, cfg.BaselinePolicyPath)
	result, err := executor.Run(*brief, cfg.ReplayDataPath)
	if err != nil {
		return fmt.Errorf("run experiment: %w", err)
	}

	fmt.Printf("experiment: %s\n", result.Experiment.ID)
	fmt.Printf("status: %s\n", result.Experiment.Status)
	fmt.Printf("candidate_prompt: %s\n", result.CandidatePrompt)
	fmt.Printf("evaluation_mode: %s\n", result.EvaluationMode)
	fmt.Printf("policy_checks: %d\n", len(result.PolicyChecks))
	if result.DataMetadata != nil {
		fmt.Printf("data_range: %s to %s\n", result.DataMetadata.DateRangeStart.Format("2006-01-02"), result.DataMetadata.DateRangeEnd.Format("2006-01-02"))
		if result.DataMetadata.DaysDelayed > 2 {
			fmt.Printf("data_delay_warning: replay data is %d days delayed\n", result.DataMetadata.DaysDelayed)
		}
	}
	return nil
}

// ─── store seam（Q2 batch 2 unit A；站點 cmd/run-experiment/main.go:32） ──────
//
// 為什麼這條路徑必須是 backend-aware：實驗紀錄（experiments 表/experiments.jsonl）
// 與實驗結果（prompt_experiment_results／<LedgerDir>/experiments/*.json）在生產由
// 部署宣告的後端決定。本 CLI 原本直建 JSONL-only 的 ledger store，在
// ATLAS_STORE_BACKEND=postgres 的機器上會把生命週期寫到没有任何 PG 讀者會看的目錄
// ——正是 scripts/ci/check_ledger_seam.py 存在的理由。
//
// 步驟與 cmd/backtest-window 的 openRunnerStore 相同：解析後端 → postgres 時取 DSN
// （沒有就明確報錯，不靜默退回 JSONL）→ db.Init 開池 → 注入 ledger factory → 建 store。
// 非 postgres 後端完全不碰 DSN。
type storeDeps struct {
	initPool   func(ctx context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error)
	injectPool func(pool *pgxpool.Pool)
}

// defaultStoreDeps wires the production implementations. Tests replace them.
func defaultStoreDeps() storeDeps {
	return storeDeps{initPool: db.Init, injectPool: ledger.SetPostgresPool}
}

// openExperimentStore returns the family store for the declared backend plus a
// closer for the pool this call opened itself (no-op for jsonl/sqlite).
func openExperimentStore(ctx context.Context, cfg config.Config, deps storeDeps) (ledger.FullStore, func(), error) {
	noop := func() {}

	backend, err := ledger.ResolveStoreBackend(cfg.StoreBackend)
	if err != nil {
		return nil, nil, fmt.Errorf("run-experiment store: %w", err)
	}
	if backend != "postgres" {
		store, err := ledger.NewExperimentFamilyStore(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("run-experiment store: %w", err)
		}
		log.Printf("run-experiment: store backend=%s", backend)
		return store, noop, nil
	}

	dsn := cfg.DatabaseURL
	if dsn == "" {
		return nil, nil, fmt.Errorf(
			"run-experiment store: backend %q requires a DSN: set DATABASE_URL (refusing to write experiment records to JSONL while production reads PostgreSQL)",
			backend)
	}
	migrationsPath := cfg.MigrationsPath
	if migrationsPath == "" {
		migrationsPath = filepath.Join(cfg.WorkDir, "sql", "migrations")
	}
	pool, err := deps.initPool(ctx, dsn, migrationsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("run-experiment store: connect postgres: %w", err)
	}
	deps.injectPool(pool)

	store, err := ledger.NewExperimentFamilyStore(cfg)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("run-experiment store: %w", err)
	}
	// 只記後端名與「有沒有 DSN」，絕不印 DSN 本身。
	log.Printf("run-experiment: store backend=%s dsn_set=true", backend)
	return store, pool.Close, nil
}
