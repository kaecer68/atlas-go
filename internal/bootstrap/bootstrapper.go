package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/taskexec"
)

type Config struct {
	WorkDir   string
	LedgerDir string
}

func NewConfig(cfg config.Config) Config {
	return Config{
		WorkDir:   cfg.WorkDir,
		LedgerDir: cfg.LedgerDir,
	}
}

func InitMetrics() *monitoring.MetricsCollector {
	return monitoring.NewMetricsCollector()
}

func InitDatabase(ctx context.Context, cfg Config, collector *monitoring.MetricsCollector) (*pgxpool.Pool, error) {
	// config.GetSecret reads env then Keychain (envOrKeychain); raw os.Getenv
	// here violated constitution Article 1 (a5-violations.json:66-69).
	dsn := config.GetSecret("DATABASE_URL")
	if dsn == "" {
		return nil, nil
	}
	migrationsPath := filepath.Join(cfg.WorkDir, "sql/migrations")
	if _, err := os.Stat(migrationsPath); err == nil {
		pool, err := db.Init(context.Background(), dsn, migrationsPath)
		if err != nil {
			logging.Error("bootstrap", "db_init_failed", "err", err)
			monitoring.RecordDBInitFailure(collector)
			return nil, err
		}
		logging.Info("bootstrap", "db_connected_migrations_applied")
		return pool, nil
	}
	return nil, nil
}

type Stores struct {
	AlertStore   *monitoring.AlertStore
	MetricsStore *monitoring.MetricsStore
	OutcomeStore ledger.OutcomeStore
}

func InitStores(cfg Config) (Stores, error) {
	alertStore, err := monitoring.NewAlertStore(filepath.Join(cfg.WorkDir, "data/state/alerts"))
	if err != nil {
		logging.Warn("bootstrap", "alert_store_init_warning", "err", err)
	}
	metricsStore, err := monitoring.NewMetricsStore(filepath.Join(cfg.WorkDir, "data/state"))
	if err != nil {
		logging.Warn("bootstrap", "metrics_store_init_warning", "err", err)
	}
	// Read via config.GetSecret (envOrKeychain) instead of raw os.Getenv —
	// fixes the constitution Article 1 violation flagged in
	// a5-violations.json:66-69. Unlike config.Load(), no default is applied:
	// when unset the store falls back to jsonl (legacy behavior preserved).
	fullCfg := config.Config{
		LedgerDir:    cfg.LedgerDir,
		StoreBackend: config.GetSecret("ATLAS_STORE_BACKEND"),
		SQLitePath:   config.GetSecret("ATLAS_SQLITE_PATH"),
	}
	outcomeStore, err := ledger.NewOutcomeStore(fullCfg)
	if err != nil {
		return Stores{}, fmt.Errorf("create outcome store: %w", err)
	}

	return Stores{
		AlertStore:   alertStore,
		MetricsStore: metricsStore,
		OutcomeStore: outcomeStore,
	}, nil
}

func InitRepository(pool *pgxpool.Pool, stores Stores) *repository.DualWriteRepository {
	if pool == nil {
		return nil
	}

	var alertStoreAdapter repository.AlertStore
	if stores.AlertStore != nil {
		alertStoreAdapter = monitoring.NewAlertStoreAdapter(stores.AlertStore)
	}

	var metricsStoreAdapter repository.MetricsStore
	if stores.MetricsStore != nil {
		metricsStoreAdapter = monitoring.NewMetricsStoreAdapter(stores.MetricsStore)
	}

	return repository.NewDualWriteRepository(
		pool,
		alertStoreAdapter,
		metricsStoreAdapter,
		monitoring.NewOutcomeStoreAdapter(stores.OutcomeStore),
		stores.OutcomeStore,
		stores.OutcomeStore,
		stores.OutcomeStore,
	)
}

func InitTaskManager(ctx context.Context, pool *pgxpool.Pool, cfg Config) *taskexec.Manager {
	if pool != nil {
		pgRepo := repository.NewPostgresRepository(pool)
		taskStore := repository.NewTaskExecutionStore(pgRepo)
		mgr := taskexec.NewManager(taskStore)
		mgr.SetContext(ctx)
		logging.Info("bootstrap", "postgres_store_initialized")
		registerTaskRunners(mgr, cfg)
		return mgr
	}
	mgr := taskexec.NewManager(taskexec.NewInMemoryStore())
	mgr.SetContext(ctx)
	logging.Info("bootstrap", "inmemory_store_initialized")
	registerTaskRunners(mgr, cfg)
	return mgr
}

func registerTaskRunners(mgr *taskexec.Manager, cfg Config) {
	taskCfg := config.Config{WorkDir: cfg.WorkDir}
	mgr.RegisterRunner(string(domain.TaskTypeRunExperiment), taskexec.NewRunExperimentRunner(taskCfg))
	mgr.RegisterRunner(string(domain.TaskTypeJudgeExperiment), taskexec.NewJudgeExperimentRunner(taskCfg))
	mgr.RegisterRunner(string(domain.TaskTypePromoteBaseline), taskexec.NewPromoteBaselineRunner(taskCfg))
	mgr.RegisterRunner(string(domain.TaskTypeBacktestWindow), taskexec.NewBacktestWindowRunner(taskCfg))
	mgr.RegisterRunner("margin_backfill", taskexec.NewMarginBackfillRunner(cfg.WorkDir))
	logging.Info("bootstrap", "task_manager_initialized", "runner_count", 5)
}

type Runtime struct {
	Config           Config
	MetricsCollector *monitoring.MetricsCollector
	Pool             *pgxpool.Pool
	Stores           Stores
	Repository       *repository.DualWriteRepository
	TaskManager      *taskexec.Manager
}

func InitRuntime(ctx context.Context, cfg Config) (*Runtime, error) {
	rt := &Runtime{Config: cfg}

	rt.MetricsCollector = InitMetrics()
	logging.Info("bootstrap", "metrics_collector_initialized")

	pool, err := InitDatabase(ctx, cfg, rt.MetricsCollector)
	if err != nil {
		logging.Warn("bootstrap", "db_init_warning", "err", err)
	}
	rt.Pool = pool

	stores, err := InitStores(cfg)
	if err != nil {
		return nil, err
	}
	rt.Stores = stores

	rt.Repository = InitRepository(pool, stores)
	if rt.Repository != nil {
		logging.Info("repository", "dual_write_initialized")
	}

	rt.TaskManager = InitTaskManager(ctx, pool, cfg)

	return rt, nil
}

func (rt *Runtime) Close() {
	if rt.Pool != nil {
		rt.Pool.Close()
	}
}
