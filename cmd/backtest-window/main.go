package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	atlasdb "github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("backtest-window", flag.ContinueOnError)
	start := fs.String("start", "2026-03-26", "backtest window start date (YYYY-MM-DD)")
	end := fs.String("end", "2026-03-27", "backtest window end date (YYYY-MM-DD)")
	serve := fs.Bool("serve", false, "start dashboard API server after backtest completes")
	addr := fs.String("addr", constants.AdminHTTPPort, "dashboard API listen address (used with -serve)")
	paramOverride := arrayFlags{}
	fs.Var(&paramOverride, "param-override", "override a ParametersConfig value (repeatable: -param-override name=value)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	startDate, err := time.Parse("2006-01-02", *start)
	if err != nil {
		return fmt.Errorf("parse start date: %w", err)
	}
	endDate, err := time.Parse("2006-01-02", *end)
	if err != nil {
		return fmt.Errorf("parse end date: %w", err)
	}

	cfg := config.Load()

	tempCleanup := func() {}
	if len(paramOverride) > 0 {
		ie := config.NewInferenceEngine(config.GetParametersConfig())
		if err := applyParamOverrides(ie, paramOverride); err != nil {
			return fmt.Errorf("param-override: %w", err)
		}
		overridePath, err := materializeParamConfig(ie)
		if err != nil {
			return fmt.Errorf("materialize param-override: %w", err)
		}
		cfg.ParametersConfigPath = overridePath
		tempCleanup = func() { _ = os.RemoveAll(overridePath) }
	}
	defer tempCleanup()

	store, closeStore, err := openRunnerStore(context.Background(), cfg, defaultStoreDeps())
	if err != nil {
		return err
	}
	defer closeStore()

	runner := backtest.NewRunner(cfg, store)
	summary, err := runner.Run(startDate, endDate)
	if err != nil {
		return fmt.Errorf("run backtest window: %w", err)
	}

	fmt.Printf("window: %s\n", summary.WindowID)
	fmt.Printf("sessions: %d\n", summary.SessionCount)
	fmt.Printf("outcomes: %d\n", summary.OutcomeCount)
	fmt.Printf("worst_agent: %s\n", summary.WorstAgentID)
	fmt.Printf("worst_skill: %s\n", summary.WorstAgentSkill)
	fmt.Printf("worst_sharpe_like: %.6f\n", summary.WorstAgentSharpeLike)

	report, err := runner.GenerateReport(summary)
	if err != nil {
		log.Printf("warn: failed to generate report: %v", err)
	} else {
		fmt.Println(report)
	}

	if *serve {
		mux := http.NewServeMux()
		// backtest-window serve mode only displays backtest reports; macro data is not required.
		// Use the Gateway-backed constructor with a no-op fetcher to avoid the legacy CompositeMacroProvider path.
		dashboard := monitoring.NewDashboardAPIWithGateway(cfg.WorkDir, cfg.LedgerDir, nil, monitoring.NoopFetcher())
		dashboard.RegisterRoutes(mux)
		dashboard.RegisterNarrativeRoutes(mux)
		dashboard.RegisterControlRoutes(mux)
		dashboard.RegisterMacroRoutes(mux)
		dashboard.RegisterExperimentRoutes(mux)
		dashboard.RegisterLiveRoutes(mux)
		dashboard.RegisterBacktestRoutes(mux)
		fmt.Printf("\nDashboard ready at http://localhost%s\n", *addr)
		fmt.Printf("Latest report: http://localhost%s/api/report/latest\n", *addr)
		srv := &http.Server{
			Addr:         *addr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
		if err := srv.ListenAndServe(); err != nil {
			return fmt.Errorf("dashboard api server failed: %w", err)
		}
	}
	return nil
}

// ─── store seam（Q2 batch 2；站點 cmd/backtest-window/main.go:65） ───────────
//
// 為什麼這條路徑必須是 backend-aware（2026-10 生產缺陷）：
// PR #2238（Q2b2 T2）之後，window 家族的**生產寫入端**已改走 backend-aware factory
// —— cmd/atlas 的 window_backtest 週任務與 internal/taskexec/runners.go 都用
// ledger.NewFullStore(cfg)，在 ATLAS_STORE_BACKEND=postgres 的機器上寫進 PG 的
// window_summaries／mutation_briefs。本 CLI 原本直建 JSONL-only 的 ledger store
// （指向 cfg.LedgerDir），於是同一個 window 會有**兩個互不相見的副本**：CLI 算出來的窗口
// 落到沒有人讀的 <LedgerDir>/windows/*.json（讀取端走 PG）。失效模式不是「報錯」，
// 而是**安靜地寫錯後端**，所以本檔不得再出現裸 JSONL store。
//
// 後端決策只有一條路徑（ledger.ResolveStoreBackend）：未知值直接回錯誤，
// postgres 缺 DSN 也直接回錯誤，**不靜默退回 JSONL**（#2107 的事故形狀）。
//
// -serve 模式的 dashboard 吃同一個後端決策：monitoring 的 store factory 在
// postgres 後端需要已注入的 pool（否則建 store 失敗），所以池必須在 run() 內先開好、
// 注入 ledger.SetPostgresPool，dashboard 才建得起來。

// migrationsRelDir 是相對 cfg.WorkDir 的 migration 目錄；只有 cfg.MigrationsPath
// 為空時才用到（與 cmd/backfill-quotes-range 的慣例一致）。
const migrationsRelDir = "sql/migrations"

// defaultWorkDir 是 cfg.WorkDir 為空時的 fallback（migrations 路徑的基準）。
const defaultWorkDir = "."

// storeDeps 是 store wiring 的可注入依賴：生產走真 atlasdb.Init ＋
// ledger.SetPostgresPool，單元測試用 fake（不必真的連上 PostgreSQL 就能驗證
// 「開池 → 注入 → 建 store」的順序）。
type storeDeps struct {
	initPool   func(ctx context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error)
	injectPool func(pool *pgxpool.Pool)
}

// defaultStoreDeps 是生產 wiring。
func defaultStoreDeps() storeDeps {
	return storeDeps{initPool: atlasdb.Init, injectPool: ledger.SetPostgresPool}
}

// storeForBackend 以**已正規化**的後端名建立 backend-aware store
// （與 cmd/backfill-quotes-range 的 storeForBackend 同形）。
func storeForBackend(backend string, cfg config.Config) (ledger.FullStore, error) {
	cfg.StoreBackend = backend
	return ledger.NewFullStore(cfg)
}

// openRunnerStore 回傳本次執行實際使用的 ledger store 與它的 closer。
//
// 步驟與 cmd/backfill-quotes-range 的 resolveStore 相同：
// 解析後端 → postgres 時取 DSN（沒有就明確報錯）→ atlasdb.Init 開池 →
// 注入 factory → 建 store。回傳的 closer 關閉本函式自己開的池；非 postgres 後端為 no-op
// （jsonl/sqlite **完全不碰 DSN**）。
func openRunnerStore(ctx context.Context, cfg config.Config, deps storeDeps) (ledger.FullStore, func(), error) {
	noop := func() {}

	backend, err := ledger.ResolveStoreBackend(cfg.StoreBackend)
	if err != nil {
		return nil, nil, fmt.Errorf("backtest-window store: %w", err)
	}

	if backend != "postgres" {
		store, err := storeForBackend(backend, cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("backtest-window store: %w", err)
		}
		// 記下實際使用的後端（診斷用）；後端名不是機密，DSN 才是。
		log.Printf("backtest-window: store backend=%s", backend)
		return store, noop, nil
	}

	dsn := cfg.DatabaseURL
	if dsn == "" {
		// 明確可診斷：說清楚是哪個後端、要補哪個環境變數，並寫明拒絕降級。
		return nil, nil, fmt.Errorf(
			"backtest-window store: backend %q requires a DSN: set DATABASE_URL (refusing to write windows to JSONL while production reads PostgreSQL)",
			backend)
	}
	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = defaultWorkDir
	}
	migrationsPath := cfg.MigrationsPath
	if migrationsPath == "" {
		migrationsPath = filepath.Join(workDir, migrationsRelDir)
	}

	pool, err := deps.initPool(ctx, dsn, migrationsPath)
	if err != nil {
		return nil, nil, fmt.Errorf("backtest-window store: connect postgres: %w", err)
	}

	deps.injectPool(pool)

	store, err := storeForBackend(backend, cfg)
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("backtest-window store: %w", err)
	}
	// 只記後端名與「有沒有 DSN」，絕不印 DSN 本身。
	log.Printf("backtest-window: store backend=%s dsn_set=true", backend)
	return store, pool.Close, nil
}

// arrayFlags implements flag.Value to accept repeated string flags.
type arrayFlags []string

func (a *arrayFlags) String() string { return strings.Join(*a, ",") }
func (a *arrayFlags) Set(v string) error {
	*a = append(*a, v)
	return nil
}

// parseParamOverride splits "name=value" and parses the value as float64.
// It rejects malformed entries and unknown parameter names.
func parseParamOverride(input string) (name string, value float64, err error) {
	parts := strings.SplitN(input, "=", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("param-override: malformed %q (want name=value)", input)
	}
	name = strings.TrimSpace(parts[0])
	valStr := strings.TrimSpace(parts[1])
	if name == "" {
		return "", 0, fmt.Errorf("param-override: empty name in %q", input)
	}
	if valStr == "" {
		return "", 0, fmt.Errorf("param-override: empty value for %q", name)
	}
	value, err = strconv.ParseFloat(valStr, 64)
	if err != nil {
		return "", 0, fmt.Errorf("param-override: non-numeric value for %q: %w", name, err)
	}
	// Validate the parameter name is registered in the parameter table.
	ie := config.NewInferenceEngine(config.DefaultParametersConfig())
	if _, ok := ie.GetParameter(name); !ok {
		return "", 0, fmt.Errorf("param-override: unknown parameter %q", name)
	}
	return name, value, nil
}

// applyParamOverrides parses each override string and applies it to the
// InferenceEngine via SetParameter.
func applyParamOverrides(ie *config.InferenceEngine, overrides []string) error {
	for _, o := range overrides {
		name, value, err := parseParamOverride(o)
		if err != nil {
			return err
		}
		if err := ie.SetParameter(name, value); err != nil {
			return fmt.Errorf("param-override: SetParameter(%q, %v): %w", name, value, err)
		}
	}
	return nil
}

// materializeParamConfig serializes the ParametersConfig from the
// InferenceEngine to a temporary JSON file and returns its path.
func materializeParamConfig(ie *config.InferenceEngine) (string, error) {
	tmpDir, err := os.MkdirTemp("", "backtest-params-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	path := tmpDir + "/parameters.json"

	data, err := json.MarshalIndent(ie.Parameters(), "", "  ")
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("marshal params: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write params: %w", err)
	}
	return path, nil
}
