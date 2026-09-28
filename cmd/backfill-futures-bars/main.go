// Command backfill-futures-bars 回補期貨日行情（TX/MTX 等）到設定的儲存後端。
//
// 背景：internal/marketdata 原本只有期貨的 OI／口數，沒有任何成交價序列，
// 因此無法回答「某日某契約的 OHLC ＋ 未平倉」。本 CLI 用期交所 first-party 的
// 官網 CSV 端點（可指定日期區間）做歷史回補，資料直接進 backend-aware 的
// futures_bars 表，並把連續契約的 splice 事件寫進 futures_rollovers。
// 詳見 docs/specs/futures-bars-firstparty-spec.md。
//
// 儲存後端**一律**由 -backend 或 ATLAS_STORE_BACKEND 決定（經 ledger.ResolveStoreBackend）。
// 刻意不提供「預設 sqlite、Postgres 需明示 flag」的形狀 —— 那正是 #2107 的事故形狀：
// 生產設了 postgres 卻靜默寫到本機 sqlite artifact，查詢層看到空表。
//
// 用法：
//
//	backfill-futures-bars -contracts TX,MTX -start 2001-01-01 -end 2026-09-24
//	backfill-futures-bars -contracts TX -start 2026-09-01 -dry-run
//
// 節奏：provider 內建 rate limiter（每 3 秒 1 次）＋ retry ＋ breaker；
// 分段（每段相差 ≤ 31 天，實測上限）亦由 provider 負責。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

const (
	defaultContracts = "TX,MTX"
	defaultStartDate = "2001-01-01"
	defaultPacingMS  = 3000
	maxRetriesMax    = 3
)

type cliConfig struct {
	contracts  []string
	start      time.Time
	end        time.Time
	backend    string
	source     string
	pacingMS   int
	maxRetries int
	dryRun     bool
	force      bool
	rollovers  bool
}

// runStats 是單次回補的摘要（同時作為本次回補的稽核紀錄）。
type runStats struct {
	chunks      int
	rows        int
	written     int
	rollovers   int
	fetchErrors int
}

// futuresBarsFetcher 是 CLI 對 provider 的最小需求（測試可注入 fake）。
type futuresBarsFetcher interface {
	FetchFuturesBars(ctx context.Context, contracts []string, start, end time.Time) ([]domain.FuturesBar, error)
	FetchLatestFuturesBars(ctx context.Context, contracts []string) ([]domain.FuturesBar, error)
	SetObserver(o marketdata.FuturesBarsObserver)
}

// fetchObserver 是 provider 的觀測鉤子實作：計數抓取次數與列數。
//
// 這是 SetObserver 的**生產消費者**（不是只有測試在呼叫），也為未來接
// internal/monitoring 的真指標留下 seam（規格 §8：本階段不發明 prometheus 指標）。
type fetchObserver struct {
	openAPICalls int64
	csvCalls     int64
	rows         int64
}

func (o *fetchObserver) ObserveFuturesBarsFetch(source string, _ error, rows int) {
	switch source {
	case "taifex_openapi":
		atomic.AddInt64(&o.openAPICalls, 1)
	case "taifex_csv":
		atomic.AddInt64(&o.csvCalls, 1)
	}
	atomic.AddInt64(&o.rows, int64(rows))
}

func main() {
	if err := runFromOSArgs(); err != nil {
		log.Fatalf("backfill-futures-bars: %v", err)
	}
}

func runFromOSArgs() error {
	var (
		contracts  = flag.String("contracts", defaultContracts, "逗號分隔的契約代碼（例如 TX,MTX）")
		start      = flag.String("start", defaultStartDate, "回補起日 YYYY-MM-DD（Asia/Taipei）")
		end        = flag.String("end", "", "回補迄日 YYYY-MM-DD（預設：今天 Asia/Taipei）")
		backend    = flag.String("backend", "", "儲存後端 jsonl|sqlite|postgres（預設：讀 ATLAS_STORE_BACKEND；不得硬編 sqlite）")
		source     = flag.String("source", "csv", "資料來源：csv（歷史回補）|openapi（僅最新交易日）")
		pacingMS   = flag.Int("pacing", defaultPacingMS, "每次上游請求的最小間隔毫秒")
		maxRetries = flag.Int("max-retries", maxRetriesMax, "每段請求的重試次數上限 0..3")
		dryRun     = flag.Bool("dry-run", false, "只抓取與統計，不寫入")
		force      = flag.Bool("force", false, "允許覆寫既有列（寫入語意為 upsert；此 flag 供未來切換為拒絕覆寫時使用）")
		rollovers  = flag.Bool("rollovers", true, "回補後推導連續契約 splice 事件並寫入 futures_rollovers")
	)
	flag.Parse()

	if *pacingMS < 0 {
		return fmt.Errorf("--pacing must be >= 0 (got %d)", *pacingMS)
	}
	if *maxRetries < 0 || *maxRetries > maxRetriesMax {
		return fmt.Errorf("--max-retries must be in 0..%d (got %d)", maxRetriesMax, *maxRetries)
	}
	switch *source {
	case "csv", "openapi":
	default:
		return fmt.Errorf("--source must be csv or openapi (got %q)", *source)
	}

	loc := marketdata.TaiwanLocation()
	startTime, err := time.ParseInLocation("2006-01-02", *start, loc)
	if err != nil {
		return fmt.Errorf("parse --start: %w", err)
	}
	endStr := *end
	if endStr == "" {
		endStr = time.Now().In(loc).Format("2006-01-02")
	}
	endTime, err := time.ParseInLocation("2006-01-02", endStr, loc)
	if err != nil {
		return fmt.Errorf("parse --end: %w", err)
	}
	if endTime.Before(startTime) {
		return fmt.Errorf("--end %s is before --start %s", endStr, *start)
	}

	contractsList, err := parseContracts(*contracts)
	if err != nil {
		return err
	}

	return run(cliConfig{
		contracts:  contractsList,
		start:      startTime,
		end:        endTime,
		backend:    *backend,
		source:     *source,
		pacingMS:   *pacingMS,
		maxRetries: *maxRetries,
		dryRun:     *dryRun,
		force:      *force,
		rollovers:  *rollovers,
	})
}

func parseContracts(s string) ([]string, error) {
	var out []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(s, ",") {
		code := strings.ToUpper(strings.TrimSpace(part))
		if code == "" || seen[code] {
			continue
		}
		if _, ok := domain.FuturesContractSpecFor(code); !ok {
			return nil, fmt.Errorf("unknown futures contract %q (known: %v)", code, domain.FuturesContractCodes())
		}
		seen[code] = true
		out = append(out, code)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--contracts must list at least one contract")
	}
	return out, nil
}

// run 建立真實 provider 與 store 後執行回補。
func run(cfg cliConfig) error {
	appCfg := config.Load()

	store, err := resolveStore(cfg, appCfg)
	if err != nil {
		return err
	}
	provider := marketdata.NewTAIFEXFuturesBarsProvider()
	return runWith(context.Background(), cfg, provider, store, appCfg, os.Stdout)
}

// runWith 是可測的核心：provider／store／輸出都可由呼叫端注入。
func runWith(ctx context.Context, cfg cliConfig, fetcher futuresBarsFetcher, store ledger.FuturesBarStore, appCfg config.Config, out *os.File) error {
	observer := &fetchObserver{}
	fetcher.SetObserver(observer)

	stats := &runStats{chunks: len(marketdata.ChunkFuturesDateRange(cfg.start, cfg.end))}

	var (
		bars     []domain.FuturesBar
		fetchErr error
	)
	if cfg.source == "openapi" {
		bars, fetchErr = fetcher.FetchLatestFuturesBars(ctx, cfg.contracts)
	} else {
		bars, fetchErr = fetcher.FetchFuturesBars(ctx, cfg.contracts, cfg.start, cfg.end)
	}
	// 部分段失敗時仍寫入成功的段；錯誤最後一併回報（非零 exit code）。
	if fetchErr != nil && len(bars) == 0 {
		return fmt.Errorf("fetch futures bars: %w", fetchErr)
	}
	if fetchErr != nil {
		stats.fetchErrors++
		fmt.Fprintf(os.Stderr, "[warn] 部分區段失敗（仍寫入成功段）: %v\n", fetchErr)
	}
	stats.rows = len(bars)

	if cfg.dryRun {
		fmt.Fprintf(out, "dry-run: contracts=%v rows=%d chunks=%d source=%s backend=%s force=%v\n",
			cfg.contracts, stats.rows, stats.chunks, cfg.source, backendName(cfg, appCfg), cfg.force)
		return fetchErr
	}

	written, err := store.RecordFuturesBars(ctx, bars)
	if err != nil {
		return fmt.Errorf("record futures bars: %w", err)
	}
	stats.written = written

	if cfg.rollovers {
		n, err := persistRollovers(ctx, store, cfg, bars)
		if err != nil {
			return err
		}
		stats.rollovers = n
	}

	fmt.Fprintf(out, "backfill-futures-bars: contracts=%v rows=%d written=%d rollovers=%d chunks=%d source=%s backend=%s window=%s..%s (openapi_calls=%d csv_calls=%d)\n",
		cfg.contracts, stats.rows, stats.written, stats.rollovers, stats.chunks, cfg.source,
		backendName(cfg, appCfg),
		cfg.start.Format("2006-01-02"), cfg.end.Format("2006-01-02"),
		atomic.LoadInt64(&observer.openAPICalls), atomic.LoadInt64(&observer.csvCalls))

	if stats.fetchErrors > 0 {
		return fmt.Errorf("completed with %d failed chunk group(s); %w", stats.fetchErrors, fetchErr)
	}
	return nil
}

// persistRollovers 由本次回補的 bar 推導連續契約 splice 事件並落庫（規格 §6.3 配套 2）。
//
// 調整後的序列本身**不落庫**（可重算）；落庫的是 splice 錨點，讓第三人能用
// 「原始 bar ＋ futures_rollovers」重算出同一條連續序列。
func persistRollovers(ctx context.Context, store ledger.FuturesBarStore, cfg cliConfig, bars []domain.FuturesBar) (int, error) {
	var events []domain.FuturesRollover
	for _, contract := range cfg.contracts {
		subset := make([]domain.FuturesBar, 0, len(bars))
		for _, b := range bars {
			if b.Contract == contract {
				subset = append(subset, b)
			}
		}
		_, rollovers, err := marketdata.BuildContinuousSeries(subset, domain.AdjustPriceDiff)
		if err != nil {
			return 0, fmt.Errorf("derive rollovers for %s: %w", contract, err)
		}
		events = append(events, rollovers...)
	}
	if len(events) == 0 {
		return 0, nil
	}
	n, err := store.RecordFuturesRollovers(ctx, events)
	if err != nil {
		return 0, fmt.Errorf("record futures rollovers: %w", err)
	}
	return n, nil
}

// resolveStore 依 flag/環境決定後端；**不降級**。
func resolveStore(cfg cliConfig, appCfg config.Config) (ledger.FuturesBarStore, error) {
	if cfg.backend != "" {
		store, err := ledger.NewFuturesBarStoreForBackend(cfg.backend, appCfg)
		if err != nil {
			return nil, err
		}
		return store, nil
	}
	store, err := ledger.NewFuturesBarStore(appCfg)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func backendName(cfg cliConfig, appCfg config.Config) string {
	if cfg.backend != "" {
		return cfg.backend
	}
	name, err := ledger.ResolveStoreBackend(appCfg.StoreBackend)
	if err != nil {
		return appCfg.StoreBackend
	}
	return name
}
