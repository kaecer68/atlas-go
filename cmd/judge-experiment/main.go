package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/repository"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func findLatestExperiment(dir string) string {
	return experiment.FindLatestExperiment(dir)
}

func run(args []string) error {
	defaultPath := findLatestExperiment(constants.StateExperiments)
	if defaultPath == "" {
		defaultPath = "data/state/experiments/exec-value-yield-01-1776084503.json"
	}

	fs := flag.NewFlagSet("judge-experiment", flag.ContinueOnError)
	path := fs.String("result", defaultPath, "experiment result json path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Load()
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = constants.ReplayCSVPath
	}
	store, closeStore, err := openExperimentStore(context.Background(), cfg, defaultStoreDeps())
	if err != nil {
		return err
	}
	defer closeStore()
	judge := experiment.NewJudge(store, cfg.ReplayDataPath, cfg.BaselinePolicyPath)
	result, err := judge.Evaluate(*path)
	if err != nil {
		return fmt.Errorf("judge experiment: %w", err)
	}

	fmt.Printf("experiment: %s\n", result.Experiment.ID)
	fmt.Printf("status: %s\n", result.Experiment.Status)
	fmt.Printf("baseline: %.6f\n", result.Experiment.BaselineValue)
	fmt.Printf("candidate: %.6f\n", result.Experiment.CandidateValue)
	fmt.Printf("evaluation_mode: %s\n", result.EvaluationMode)

	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pool, poolErr := db.Init(context.Background(), dsn, filepath.Join(filepath.Dir(os.Args[0]), "..", "..", "sql", "migrations"))
		if poolErr == nil {
			defer pool.Close()
			collector := monitoring.NewMetricsCollector()
			snap := collector.GetMetricsSnapshot()
			repoSnap := repository.MetricsSnapshot{
				ScreeningTotal:     snap.ScreeningTotal,
				ScreeningPassed:    snap.ScreeningPassed,
				ScreeningRate:      snap.ScreeningRate,
				AlertsTriggered:    snap.AlertsTriggered,
				AlertsAcknowledged: snap.AlertsAcknowledged,
				AlertsByType:       snap.AlertsByType,
				Timestamp:          time.Now(),
			}
			repo := repository.NewDualWriteRepository(pool, nil, nil, nil, nil, nil, nil)
			if err := repo.SaveSnapshot(context.Background(), &repoSnap); err != nil {
				log.Printf("[Metrics] snapshot save failed: %v", err)
			} else {
				log.Printf("[Metrics] snapshot saved")
			}
		}
	}
	return nil
}

// ─── store seam（Q2 batch 2 unit A；站點 cmd/judge-experiment/main.go:47） ─────
//
// 為什麼這條路徑必須是 backend-aware：judge 的 store 有兩件事靠它——① 判決結果
// 寫回（UpdatePromptExperimentResult）要落在部署宣告的後端，② DSR gate 以型別斷言
// 找 ledger.TrialSharpeLoader 讀歷史 trial Sharpe（internal/experiment/dsr.go）。
// 直建 JSONL store 時，生產（PG）的 judge 讀不到寫入端剛寫進 PG 的結果。
//
// 步驟與 cmd/run-experiment 的同名 helper 相同（也與 cmd/backtest-window 的
// openRunnerStore 同形）：解析後端 → postgres 時取 DSN（沒有就明確報錯，不靜默退回
// JSONL）→ db.Init 開池 → 注入 ledger factory → 建 store。非 postgres 不碰 DSN。
//
// 註：本檔下方的 metrics snapshot 區塊在 DATABASE_URL 有值時另外開自己的池
// （既有行為，本單元不動）。兩者是獨立的連線用途，不共用池。
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
		return nil, nil, fmt.Errorf("judge-experiment store: %w", err)
	}
	if backend != "postgres" {
		store, err := ledger.NewExperimentFamilyStore(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("judge-experiment store: %w", err)
		}
		log.Printf("judge-experiment: store backend=%s", backend)
		return store, noop, nil
	}

	dsn := cfg.DatabaseURL
	if dsn == "" {
		return nil, nil, fmt.Errorf(
			"judge-experiment store: backend %q requires a DSN: set DATABASE_URL (refusing to judge through a JSONL ledger while production reads PostgreSQL)",
			backend)
	}
	migrationsPath := cfg.MigrationsPath
	if migrationsPath == "" {
		migrationsPath = filepath.Join(cfg.WorkDir, "sql", "migrations")
	}
	pool, err := deps.initPool(ctx, dsn, migrationsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("judge-experiment store: connect postgres: %w", err)
	}
	deps.injectPool(pool)

	store, err := ledger.NewExperimentFamilyStore(cfg)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("judge-experiment store: %w", err)
	}
	// 只記後端名與「有沒有 DSN」，絕不印 DSN 本身。
	log.Printf("judge-experiment: store backend=%s dsn_set=true", backend)
	return store, pool.Close, nil
}
