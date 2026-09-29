// Command backfill-quotes-range 以 FinMind TaiwanStockPrice 的**區間**查詢，
// 把股票日線（quotes）補進 backend-aware 的 store。
//
// 為什麼要有這一支（#2126 的後續）：生產 quotes 表在 2026-06-25 之前只有 114 檔
// （FinMind 全市場回填自該日起），任何 per-stock 研究在此之前都缺標籤地基。
// 既有唯一入口是 per-day 的 GetStockPrice(symbol, date)：對「2026-03-02→06-24
// （約 80 個交易日）× 約 1,600 檔」= 約 128k 次呼叫 ≈ 64 小時，不可行。
// 本 CLI 走 range 形式（internal/marketdata 的 GetStockPriceRange）：**每檔一次
// 呼叫覆蓋整段** ⇒ 約 1,600 次呼叫 ≈ 2.7 小時（免費 600/hr）；付費速率更快。
//
// 儲存後端**一律**由 -backend 或 ATLAS_STORE_BACKEND 決定（經
// ledger.ResolveStoreBackend）。刻意不提供「預設 sqlite、Postgres 需明示 flag」的
// 形狀 —— 那正是 #2107 的事故形狀：生產設了 postgres 卻靜默寫到本機 sqlite
// artifact，查詢層看到空表。決策規則與 cmd/backfill-futures-bars（#2118）、
// cmd/backfill-period-history-range 相同：
//
//  1. 解析後端（resolveBackend，單一決策路徑）
//  2. postgres 時取 DSN（-pg-dsn，預設 $DATABASE_URL）—— 沒有就**明確**報錯，
//     絕不退回 sqlite
//  3. atlasdb.Init（ping ＋ 套用 migrations）建立連線池
//  4. 注入 store factory（ledger.SetPostgresPool），**再**建 store
//
// 資料紀律（三條）：
//
//   - **每次寫入的窗口只包含缺少的交易日**。計畫階段先算「窗口內的交易日」，
//     再扣掉 store 已有的日期；沒有任何缺日就完全不呼叫上游。這讓重跑（例如
//     撞到配額中止後）自動續傳，也不會覆蓋既有列（既有列的 name 不會被空字串蓋掉）。
//     -force 才會重抓整個窗口並覆寫。
//   - 日期一律是 **UTC 午夜**。quotes 的三個 store 格式化日期的方式不一致
//     （sqlite/jsonl 用 Date.Format，postgres 用 Date.UTC().Format），只有 UTC
//     午夜能讓同一份資料在三個後端寫出同一天。同理，LoadQuotes 的區間界也用
//     UTC 午夜，否則 postgres 的文字日期比較會整整差一天。
//   - 只寫「台灣交易日」的列（marketdata.IsTaiwanTradingDay）。休市日的列會污染
//     下游 ForwardReturn 的重複偵測（2026-08-23 replay 事故），
//     cmd/backfill-quotes、cmd/daily-replay-sync、cmd/cron-quote-backfill 都已有
//     同一道守門，本 CLI 不新增例外。
//
// 節奏：由 FinMindClient 的**共用** rate limiter 決定（FINMIND_RATE_LIMIT_PER_HOUR，
// 未設 = 免費 600/hr ≈ 6s/req），本 CLI 不另外加 sleep（加了會變成兩層節流，
// 反而把 2.7 小時變成 5+ 小時）。每日上限見 marketdata.FinMindDailyLimit；
// 撞到配額／IP 封鎖／breaker open 時本 CLI **立即中止**（見 fatalUpstreamError），
// 剩餘檔數留給下次執行續傳。
//
// 用法：
//
//	backfill-quotes-range -start 2026-03-02 -end 2026-06-24              # 全 store 既有標的
//	backfill-quotes-range -start 2026-03-02 -end 2026-06-24 -symbols 2330,2317
//	backfill-quotes-range -start 2026-03-02 -end 2026-06-24 -dry-run    # 只抓與統計，不寫入
//	backfill-quotes-range -start 2026-03-02 -end 2026-06-24 -pg-dsn "$DATABASE_URL" -workdir /path/to/atlas
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	atlasdb "github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

const (
	// defaultWorkDir 是 atlas repo root 的預設值；postgres 後端讀
	// <workDir>/sql/migrations（與 cmd/backfill-futures-bars 同慣例）。
	defaultWorkDir = "."
	// migrationsRelDir 是相對 workDir 的 migration 目錄。
	migrationsRelDir = "sql/migrations"
	// quotesRangeSource 是本 CLI 寫入 quotes 的 producer 標籤。
	//
	// 比 marketdata 的 provider 身分（"finmind"）精確一級：事後稽核要能分辨
	// 某一列是 per-day 補的（"finmind_backfill"，internal/monitoring）還是
	// 本 CLI 的 range 補的。
	quotesRangeSource = "finmind_quotes_range"
	// allSymbolsKeyword 讓 -symbols all 與「省略 -symbols」同義
	// （shell wrapper 傳空字串時比較不容易出錯）。
	allSymbolsKeyword = "all"
	// defaultSuffix 用於**裸代碼**（2330）→ 生產 quotes 的鍵形式（2330.TW）。
	// 生產的 FinMind 路徑一律用 <code>.TW（internal/monitoring/quote_backfill_task.go
	// 的 qsSymbol），本 CLI 沿用，否則補出來的列會是另一個鍵、查詢層看不到。
	defaultSuffix = ".TW"
)

type cliConfig struct {
	start   time.Time // UTC 午夜（見檔頭「日期一律是 UTC 午夜」）
	end     time.Time // UTC 午夜
	symbols []string  // 空 = 由 store 既有鍵推導
	backend string
	// resolvedBackend 是 resolveBackend 的結果（run() 填入；供摘要輸出）。
	// 空 = 只有 -backend（測試直接建 cfg 時）。
	resolvedBackend string
	workDir         string
	pgDSN           string
	force           bool
	dryRun          bool
}

// stockPriceRangeFetcher 是 CLI 對 provider 的最小需求（測試可注入 fake）。
type stockPriceRangeFetcher interface {
	GetStockPriceRange(ctx context.Context, symbol, startDate, endDate string) ([]domain.DailyBar, error)
}

// runStats 是單次回補的摘要（同時作為本次回補的稽核紀錄）。
type runStats struct {
	symbols         int
	requests        int
	skippedComplete int
	rowsFetched     int
	rowsWritten     int
	rowsNotWanted   int
	noData          int
	failures        int
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatalf("backfill-quotes-range: %v", err)
	}
}

// run 解析參數、決定後端與標的清單，然後執行回補。
//
// args 是**不含程式名**的參數（測試走真 flag parsing：宣告式行為也因此被驗證）。
func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("backfill-quotes-range", flag.ContinueOnError)
	fs.SetOutput(out)
	var (
		startFlag  = fs.String("start", "", "回補起日 YYYY-MM-DD（必填，含頭）")
		endFlag    = fs.String("end", "", "回補迄日 YYYY-MM-DD（預設：今天，以 Asia/Taipei 日曆日為準）")
		symbolsRaw = fs.String("symbols", "", "逗號分隔的標的（2330 → 2330.TW；也可寫 2330.TW）。空或 all = 用 store 內既有的標的鍵")
		backend    = fs.String("backend", "", "儲存後端 jsonl|sqlite|postgres（預設：讀 ATLAS_STORE_BACKEND；不得硬編 sqlite）")
		workDir    = fs.String("workdir", defaultWorkDir, "atlas repo root；postgres 後端讀 <workdir>/sql/migrations，也是 FinMind 配額狀態目錄")
		pgDSN      = fs.String("pg-dsn", "", "PostgreSQL DSN（預設：$DATABASE_URL）；僅 postgres 後端需要")
		force      = fs.Bool("force", false, "重抓整個窗口並覆寫既有列（預設只補缺少的交易日，既有列不動）")
		dryRun     = fs.Bool("dry-run", false, "只抓取與統計，不寫入")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *startFlag == "" {
		return errors.New("-start is required (YYYY-MM-DD)")
	}

	start, err := parseExchangeDay(*startFlag)
	if err != nil {
		return fmt.Errorf("parse -start: %w", err)
	}
	end := start
	if *endFlag != "" {
		if end, err = parseExchangeDay(*endFlag); err != nil {
			return fmt.Errorf("parse -end: %w", err)
		}
	} else {
		end = exchangeToday()
	}
	if end.Before(start) {
		return fmt.Errorf("-end %s is before -start %s", end.Format("2006-01-02"), start.Format("2006-01-02"))
	}
	if len(tradingDaysBetween(start, end)) == 0 {
		return fmt.Errorf("window %s..%s contains no Taiwan trading day",
			start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	appCfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, closeStore, err := resolveStore(ctx, cliConfig{backend: *backend, workDir: *workDir, pgDSN: *pgDSN}, appCfg, defaultStoreDeps())
	if err != nil {
		return err
	}
	defer closeStore()

	symbols, err := resolveSymbols(ctx, store, *symbolsRaw)
	if err != nil {
		return err
	}

	resolved, err := resolveBackend(cliConfig{backend: *backend}, appCfg)
	if err != nil {
		return err
	}

	cfg := cliConfig{
		start: start, end: end, symbols: symbols,
		backend: *backend, resolvedBackend: resolved, workDir: *workDir, pgDSN: *pgDSN,
		force: *force, dryRun: *dryRun,
	}
	fetcher, err := newFetcher(appCfg, *workDir)
	if err != nil {
		return err
	}
	return runWith(ctx, cfg, fetcher, store, out)
}

// newRangeFetcher 建出共用的 FinMind 客戶端（rate limiter ＋ 每日配額 tracker
// 都是該 client 的單一實例，與平台其他 FinMind 消費端同一組常數）。
func newRangeFetcher(appCfg config.Config, workDir string) (stockPriceRangeFetcher, error) {
	if strings.TrimSpace(appCfg.FinMindAPIKey) == "" {
		return nil, errors.New("FinMind API key missing: set FINMIND_API_KEY (the range fetch is authenticated)")
	}
	return marketdata.GetSharedFinMindClient(appCfg.FinMindAPIKey, workDir), nil
}

// newFetcher 是 run() 建構 provider 的 seam（生產 = newRangeFetcher）。
//
// 單元測試換掉它，就能在**不連網、不需要 API key** 的前提下測試「真 flag 解析
// → 真 store 後端決策 → 真的寫入列」這條完整路徑；生產路徑不覆寫本變數。
var newFetcher = newRangeFetcher

// parseExchangeDay 把 YYYY-MM-DD 解析成**交易所日曆日、以 UTC 午夜表示**。
//
// 不用 time.ParseInLocation(..., TaiwanLocation())：那會得到台北午夜，而
// postgres store 用 Date.UTC().Format 寫入，會整整差一天（見檔頭）。
func parseExchangeDay(s string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(s))
}

// exchangeToday 回傳「今天」的交易所日曆日（Asia/Taipei），以 UTC 午夜表示。
//
// 直接用 UTC 的今天會在台北 00:00–08:00 之間少一天（容器 TZ-unset ＝ UTC）。
func exchangeToday() time.Time {
	now := time.Now().In(marketdata.TaiwanLocation())
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// tradingDaysBetween 列出 [start, end] 內的台灣交易日（遞增，UTC 午夜的日期）。
func tradingDaysBetween(start, end time.Time) []time.Time {
	days := make([]time.Time, 0, int(end.Sub(start).Hours()/24)+1)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if !marketdata.IsTaiwanTradingDay(d) {
			continue
		}
		days = append(days, d)
	}
	return days
}

// resolveSymbols 決定本次要回補的標的清單。
//
// 空字串（或 all）＝ 用 store 內既有的標的鍵：**原樣沿用**，不猜測鍵的形式
// （生產是 2330.TW，但別名形式若存在也必須能延續，否則會補到另一個鍵上）。
// 顯式清單則把裸代碼正規化為 <code>.TW（生產 FinMind 路徑的形式）。
func resolveSymbols(ctx context.Context, store ledger.QuoteStore, raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, allSymbolsKeyword) {
		lister, ok := store.(ledger.QuoteSymbolLister)
		if !ok {
			return nil, fmt.Errorf("store %T cannot list symbols: pass -symbols explicitly", store)
		}
		stored, err := lister.QuoteSymbols(ctx)
		if err != nil {
			return nil, fmt.Errorf("list symbols from store: %w", err)
		}
		if len(stored) == 0 {
			return nil, errors.New("store has no symbols; pass -symbols explicitly")
		}
		out := make([]string, 0, len(stored))
		for _, s := range stored {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		sort.Strings(out)
		return out, nil
	}

	seen := make(map[string]bool)
	out := make([]string, 0, 16)
	for _, part := range strings.Split(raw, ",") {
		sym := normalizeQuoteSymbol(part)
		if sym == "" || seen[sym] {
			continue
		}
		seen[sym] = true
		out = append(out, sym)
	}
	if len(out) == 0 {
		return nil, errors.New("-symbols listed no usable symbol")
	}
	sort.Strings(out)
	return out, nil
}

// normalizeQuoteSymbol 把使用者輸入正規化成 quotes 的鍵形式：裸代碼補 .TW，
// 已帶交易所後綴者原樣保留（只做大寫與去空白）。
func normalizeQuoteSymbol(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	if !strings.Contains(s, ".") {
		return s + defaultSuffix
	}
	return s
}

// runWith 是可測的核心：fetcher／store／輸出都可由呼叫端注入。
func runWith(ctx context.Context, cfg cliConfig, fetcher stockPriceRangeFetcher, store ledger.QuoteStore, out io.Writer) error {
	stats := &runStats{symbols: len(cfg.symbols)}
	tradingDays := tradingDaysBetween(cfg.start, cfg.end)
	window := fmt.Sprintf("%s..%s", cfg.start.Format("2006-01-02"), cfg.end.Format("2006-01-02"))
	backend := cfg.resolvedBackend
	if backend == "" {
		backend = cfg.backend
	}

	_, _ = fmt.Fprintf(out, "backfill-quotes-range: plan symbols=%d trading_days=%d window=%s backend=%s source=%s dry_run=%v force=%v (range form: ≤%d upstream calls; per-day form would need ≤%d)\n",
		len(cfg.symbols), len(tradingDays), window, backend, quotesRangeSource, cfg.dryRun, cfg.force,
		len(cfg.symbols), len(cfg.symbols)*len(tradingDays))

	for i, symbol := range cfg.symbols {
		if err := ctx.Err(); err != nil {
			printSummary(out, cfg, stats, window, backend)
			return fmt.Errorf("cancelled after %d/%d symbols: %w", i, len(cfg.symbols), err)
		}

		existing, err := store.LoadQuotes(symbol, cfg.start, cfg.end)
		if err != nil {
			stats.failures++
			_, _ = fmt.Fprintf(out, "[%d/%d] %s load existing failed: %v\n", i+1, len(cfg.symbols), symbol, err)
			continue
		}
		plan := planSymbol(existing, tradingDays, cfg.force)
		if plan.fetchStart == "" {
			stats.skippedComplete++
			_, _ = fmt.Fprintf(out, "[%d/%d] %s already complete for %s (no upstream call)\n", i+1, len(cfg.symbols), symbol, window)
			continue
		}

		stats.requests++
		bars, err := fetcher.GetStockPriceRange(ctx, symbol, plan.fetchStart, plan.fetchEnd)
		if err != nil {
			if fatal := fatalUpstreamError(err); fatal != nil {
				// 配額／封鎖／breaker 是「今天不能再打」的條件：繼續跑只會把
				// 剩餘檔數變成同一個錯誤。立即中止，剩下的留給下次續傳。
				printSummary(out, cfg, stats, window, backend)
				return fmt.Errorf("aborting after %d/%d symbols: %w", i, len(cfg.symbols), fatal)
			}
			if errors.Is(err, marketdata.ErrNoDataForSymbol) {
				stats.noData++
				_, _ = fmt.Fprintf(out, "[%d/%d] %s no data in %s..%s (warning, continuing)\n",
					i+1, len(cfg.symbols), symbol, plan.fetchStart, plan.fetchEnd)
				continue
			}
			stats.failures++
			_, _ = fmt.Fprintf(out, "[%d/%d] %s fetch failed: %v\n", i+1, len(cfg.symbols), symbol, err)
			continue
		}
		stats.rowsFetched += len(bars)

		kept := make([]domain.DailyBar, 0, len(bars))
		for _, bar := range bars {
			if !plan.wanted[bar.Date.UTC().Format("2006-01-02")] {
				stats.rowsNotWanted++
				continue
			}
			bar.Source = quotesRangeSource
			kept = append(kept, bar)
		}

		if cfg.dryRun {
			_, _ = fmt.Fprintf(out, "[%d/%d] %s fetch %s..%s fetched=%d would_write=%d\n",
				i+1, len(cfg.symbols), symbol, plan.fetchStart, plan.fetchEnd, len(bars), len(kept))
			continue
		}
		if len(kept) > 0 {
			if err := store.RecordQuotes(kept); err != nil {
				stats.failures++
				_, _ = fmt.Fprintf(out, "[%d/%d] %s record failed: %v\n", i+1, len(cfg.symbols), symbol, err)
				continue
			}
			stats.rowsWritten += len(kept)
		}
		_, _ = fmt.Fprintf(out, "[%d/%d] %s fetch %s..%s fetched=%d wrote=%d\n",
			i+1, len(cfg.symbols), symbol, plan.fetchStart, plan.fetchEnd, len(bars), len(kept))
	}

	printSummary(out, cfg, stats, window, backend)
	if stats.failures > 0 {
		return fmt.Errorf("completed with %d failed symbol(s)", stats.failures)
	}
	return nil
}

// symbolPlan 是單一標的本次的抓取計畫。
type symbolPlan struct {
	// fetchStart/fetchEnd 是送給上游的窗口（兩端都含）。空字串 = 不需要呼叫上游。
	fetchStart string
	fetchEnd   string
	// wanted 是本次允許寫入的交易日（YYYY-MM-DD）。窗口外的列一律不寫。
	wanted map[string]bool
}

// planSymbol 由「窗口內交易日」減去「store 已有的日期」得出這一檔要抓什麼。
//
// 純函式（不碰網路），因此「不覆寫既有列」「沒有缺日就不呼叫上游」這兩件事
// 可以直接被測試釘住。force=true 時 wanted 就是整個窗口的交易日（覆寫模式）。
func planSymbol(existing []domain.DailyBar, tradingDays []time.Time, force bool) symbolPlan {
	have := make(map[string]bool, len(existing))
	for _, bar := range existing {
		have[bar.Date.UTC().Format("2006-01-02")] = true
	}

	wanted := make(map[string]bool, len(tradingDays))
	for _, day := range tradingDays {
		ds := day.Format("2006-01-02")
		if !force && have[ds] {
			continue
		}
		wanted[ds] = true
	}
	if len(wanted) == 0 {
		return symbolPlan{wanted: wanted}
	}

	days := make([]string, 0, len(wanted))
	for ds := range wanted {
		days = append(days, ds)
	}
	sort.Strings(days)
	return symbolPlan{fetchStart: days[0], fetchEnd: days[len(days)-1], wanted: wanted}
}

// fatalUpstreamError 判斷錯誤是否屬於「今天不能再打上游」的類別。
//
// 回傳非 nil 時 CLI 立即中止：配額用完、IP 被封、breaker open 都會讓後續每一
// 檔得到同一個錯誤，繼續跑只是把 quota/時間燒在保證失敗的請求上
// （2026-09-26 的教訓：backfill 在 402 之後又燒掉幾百次呼叫）。
func fatalUpstreamError(err error) error {
	for _, sentinel := range []error{
		marketdata.ErrQuotaExhausted,
		marketdata.ErrIPBanned,
		marketdata.ErrFinMindBreakerOpen,
	} {
		if errors.Is(err, sentinel) {
			return err
		}
	}
	return nil
}

func printSummary(out io.Writer, cfg cliConfig, stats *runStats, window, backend string) {
	_, _ = fmt.Fprintf(out,
		"backfill-quotes-range: symbols=%d requests=%d skipped_complete=%d rows_fetched=%d rows_written=%d skipped_not_wanted=%d no_data=%d failures=%d backend=%s source=%s window=%s dry_run=%v force=%v\n",
		stats.symbols, stats.requests, stats.skippedComplete, stats.rowsFetched, stats.rowsWritten,
		stats.rowsNotWanted, stats.noData, stats.failures, backend, quotesRangeSource, window, cfg.dryRun, cfg.force)
}

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

// resolveBackend 回傳本次執行實際使用的後端名。
//
// 後端解析**只有這一條路徑**（ledger.ResolveStoreBackend）：-backend 顯式覆寫，
// 否則沿用 ATLAS_STORE_BACKEND。未知值直接回錯誤（不靜默退回 jsonl）。
func resolveBackend(cfg cliConfig, appCfg config.Config) (string, error) {
	if cfg.backend != "" {
		return ledger.ResolveStoreBackend(cfg.backend)
	}
	return ledger.ResolveStoreBackend(appCfg.StoreBackend)
}

// storeForBackend 以**已正規化**的後端名建立 quotes store。
func storeForBackend(backend string, appCfg config.Config) (ledger.QuoteStore, error) {
	appCfg.StoreBackend = backend
	return ledger.NewQuoteStore(appCfg)
}

// resolveStore 依 flag/環境決定後端；**不降級**。
//
// 步驟與 cmd/backfill-futures-bars 相同（見該檔的 resolveStore 註解）：
// 解析後端 → 取 DSN（沒有就明確報錯）→ atlasdb.Init 開池 → 注入 factory → 建 store。
// 回傳的 closer 負責關閉本函式自己開的連線池；非 postgres 後端為 no-op。
func resolveStore(ctx context.Context, cfg cliConfig, appCfg config.Config, deps storeDeps) (ledger.QuoteStore, func(), error) {
	noop := func() {}

	backend, err := resolveBackend(cfg, appCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("quote store: %w", err)
	}

	if backend != "postgres" {
		// jsonl/sqlite 行為不變：完全不碰 DSN。
		store, err := storeForBackend(backend, appCfg)
		if err != nil {
			return nil, nil, err
		}
		return store, noop, nil
	}

	dsn := cfg.pgDSN
	if dsn == "" {
		dsn = appCfg.DatabaseURL
	}
	if dsn == "" {
		// 明確可診斷：說清楚是哪個後端、要補哪個 flag/環境變數。
		return nil, nil, fmt.Errorf(
			"quote store: backend %q requires a PostgreSQL DSN: pass -pg-dsn or set DATABASE_URL (refusing to fall back to sqlite/jsonl)",
			backend)
	}

	workDir := cfg.workDir
	if workDir == "" {
		workDir = defaultWorkDir
	}
	pool, err := deps.initPool(ctx, dsn, filepath.Join(workDir, migrationsRelDir))
	if err != nil {
		return nil, nil, fmt.Errorf("connect postgres: %w", err)
	}

	deps.injectPool(pool)

	store, err := storeForBackend(backend, appCfg)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return store, pool.Close, nil
}
