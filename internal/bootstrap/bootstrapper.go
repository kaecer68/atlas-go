package bootstrap

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
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

func InitDatabase(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, nil
	}
	migrationsPath := filepath.Join(cfg.WorkDir, "sql/migrations")
	if _, err := os.Stat(migrationsPath); err == nil {
		pool, err := db.Init(context.Background(), dsn, migrationsPath)
		if err != nil {
			log.Printf("[DB] failed to initialize database: %v", err)
			return nil, err
		}
		log.Printf("[DB] connected and migrations applied")
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
		log.Printf("[Stores] alert store init warning: %v", err)
	}
	metricsStore, err := monitoring.NewMetricsStore(filepath.Join(cfg.WorkDir, "data/state"))
	if err != nil {
		log.Printf("[Stores] metrics store init warning: %v", err)
	}
	outcomeStore := ledger.NewStore(cfg.LedgerDir)

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
		log.Printf("[TaskExec] PostgreSQL store initialized")
		registerTaskRunners(mgr, cfg)
		return mgr
	}
	mgr := taskexec.NewManager(taskexec.NewInMemoryStore())
	mgr.SetContext(ctx)
	log.Printf("[TaskExec] in-memory store initialized (data will not persist across restarts)")
	registerTaskRunners(mgr, cfg)
	return mgr
}

func registerTaskRunners(mgr *taskexec.Manager, cfg Config) {
	taskCfg := config.Config{WorkDir: cfg.WorkDir}
	mgr.RegisterRunner(string(domain.TaskTypeRunExperiment), taskexec.NewRunExperimentRunner(taskCfg))
	mgr.RegisterRunner(string(domain.TaskTypeJudgeExperiment), taskexec.NewJudgeExperimentRunner(taskCfg))
	mgr.RegisterRunner(string(domain.TaskTypePromoteBaseline), taskexec.NewPromoteBaselineRunner(taskCfg))
	mgr.RegisterRunner(string(domain.TaskTypeBacktestWindow), taskexec.NewBacktestWindowRunner(taskCfg))
	log.Printf("[TaskExec] manager initialized with 4 runners")
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
	log.Printf("[Metrics] collector initialized")

	pool, err := InitDatabase(ctx, cfg)
	if err != nil {
		log.Printf("[DB] initialization warning: %v", err)
	}
	rt.Pool = pool

	stores, err := InitStores(cfg)
	if err != nil {
		return nil, err
	}
	rt.Stores = stores

	rt.Repository = InitRepository(pool, stores)
	if rt.Repository != nil {
		log.Printf("[Repository] dual-write mode initialized")
	}

	rt.TaskManager = InitTaskManager(ctx, pool, cfg)

	return rt, nil
}

func (rt *Runtime) Close() {
	if rt.Pool != nil {
		rt.Pool.Close()
	}
}
