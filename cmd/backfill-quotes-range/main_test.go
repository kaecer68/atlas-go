package main

// backfill-quotes-range 的契約測試。
//
// 這一組測試守三件事：
//  1. 成本形狀：一段窗口只能是「每檔一次」呼叫（range 形式）；改回 per-day 必紅。
//  2. 資料形狀：只補缺少的交易日、只寫交易日、日期是 UTC 午夜、source 標籤正確。
//  3. 後端形狀：#2107 —— 宣告 postgres 而沒有 DSN 時必須失敗，**不得**落到 sqlite
//     （連「sqlite 檔有沒有被建出來」都一起斷言）。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// ─── fakes ──────────────────────────────────────────────────────────────────

// fakeRangeFetcher 讓 CLI 的核心路徑可以在不連網的情況下被測。
type fakeRangeFetcher struct {
	// bars 依序回傳（呼叫端通常是「每檔一組」）；不足時沿用最後一組。
	bars [][]domain.DailyBar
	// errFor 指定某檔回傳的錯誤；nil 時全部成功。
	errFor map[string]error
	// calls 記錄每次呼叫的 "symbol start..end"，突變（改回 per-day）會讓它變長。
	calls []string
	// afterCall 在每次呼叫後被叫（帶目前累計次數）；測試用它模擬 SIGINT。
	afterCall func(n int)
}

func (f *fakeRangeFetcher) GetStockPriceRange(_ context.Context, symbol, start, end string) ([]domain.DailyBar, error) {
	f.calls = append(f.calls, fmt.Sprintf("%s %s..%s", symbol, start, end))
	if f.afterCall != nil {
		f.afterCall(len(f.calls))
	}
	if err, ok := f.errFor[symbol]; ok {
		return nil, err
	}
	if len(f.bars) == 0 {
		return nil, fmt.Errorf("no fixture bars for %s: %w", symbol, marketdata.ErrNoDataForSymbol)
	}
	out := f.bars[0]
	if len(f.bars) > 1 {
		f.bars = f.bars[1:]
	}
	return out, nil
}

// barsFor 造一組「每個指定日期一列」的 fixture。
func barsFor(symbol string, dates ...string) []domain.DailyBar {
	out := make([]domain.DailyBar, 0, len(dates))
	for i, ds := range dates {
		d, err := parseExchangeDay(ds)
		if err != nil {
			panic(err)
		}
		out = append(out, domain.DailyBar{
			Date: d, Symbol: symbol, Open: 10 + float64(i), High: 11 + float64(i),
			Low: 9 + float64(i), Close: 10.5 + float64(i), Volume: int64(1000 + i), Source: "fake",
		})
	}
	return out
}

// newTempSQLiteStore 開一個臨時 sqlite quotes store。
func newTempSQLiteStore(t *testing.T) *ledger.SQLiteQuoteStore {
	t.Helper()
	store, err := ledger.NewSQLiteQuoteStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open sqlite quote store: %v", err)
	}
	return store
}

// ─── 純函式 ─────────────────────────────────────────────────────────────────

func TestNormalizeQuoteSymbol(t *testing.T) {
	cases := map[string]string{
		"2330":     "2330.TW", // 裸代碼 → 生產的鍵形式
		" 2330 ":   "2330.TW",
		"2330.TW":  "2330.TW",
		"2330.tw":  "2330.TW",
		"6488.TWO": "6488.TWO", // 已帶後綴者原樣保留
		"":         "",
		"   ":      "",
	}
	for in, want := range cases {
		if got := normalizeQuoteSymbol(in); got != want {
			t.Errorf("normalizeQuoteSymbol(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseExchangeDay_IsUTCMidnight(t *testing.T) {
	d, err := parseExchangeDay("2026-03-02")
	if err != nil {
		t.Fatalf("parseExchangeDay: %v", err)
	}
	if d.UTC() != d {
		t.Fatalf("date = %v (%v), want UTC midnight（postgres store 用 Date.UTC().Format 寫入）", d, d.Location())
	}
	if _, err := parseExchangeDay("2026-3-2"); err == nil {
		t.Fatal("want error for a non YYYY-MM-DD date")
	}
}

// TestExchangeToday_UsesTaipeiCalendarDay 釘住預設迄日的日曆日來源：
// 容器 TZ-unset（UTC）時，UTC 的「今天」在台北 00:00–08:00 之間會少一天。
func TestExchangeToday_UsesTaipeiCalendarDay(t *testing.T) {
	got := exchangeToday()
	if got.UTC() != got {
		t.Fatalf("exchangeToday = %v, want UTC midnight representation", got.UTC())
	}
	if h, m, s := got.Clock(); h != 0 || m != 0 || s != 0 {
		t.Fatalf("exchangeToday = %v, want midnight", got)
	}
	want := time.Now().In(marketdata.TaiwanLocation()).Format("2006-01-02")
	if got.Format("2006-01-02") != want {
		t.Fatalf("exchangeToday = %s, want the Asia/Taipei calendar day %s", got.Format("2006-01-02"), want)
	}
}

// TestTradingDaysBetween_SkipsWeekendsAndHolidays：2026-01-01（元旦，週四）
// 不是交易日；同週則是。這道守門與 cmd/backfill-quotes 的 filterTradingDays 一致。
func TestTradingDaysBetween_SkipsWeekendsAndHolidays(t *testing.T) {
	start, _ := parseExchangeDay("2026-01-01")
	end, _ := parseExchangeDay("2026-01-05")
	got := tradingDaysBetween(start, end)
	want := []string{"2026-01-02", "2026-01-05"}
	var have []string
	for _, d := range got {
		have = append(have, d.Format("2006-01-02"))
		if w := d.Weekday(); w == time.Saturday || w == time.Sunday {
			t.Fatalf("週末進入了交易日清單: %s", have[len(have)-1])
		}
	}
	if strings.Join(have, ",") != strings.Join(want, ",") {
		t.Fatalf("tradingDaysBetween = %v, want %v", have, want)
	}
}

// TestPlanSymbol 是「只補缺、不覆寫」的核心：計畫完全由（既有日期 × 交易日）決定。
func TestPlanSymbol(t *testing.T) {
	days := func(from, to string) []time.Time {
		s, _ := parseExchangeDay(from)
		e, _ := parseExchangeDay(to)
		return tradingDaysBetween(s, e)
	}
	// 2026-03-02(一) 03-03(二) 03-04(三) 03-05(四) 03-06(五)
	all := days("2026-03-02", "2026-03-06")
	if len(all) != 5 {
		t.Fatalf("fixture expects 5 trading days, got %d", len(all))
	}

	t.Run("missing_only", func(t *testing.T) {
		existing := barsFor("2330.TW", "2026-03-02", "2026-03-03")
		plan := planSymbol(existing, all, false)
		if plan.fetchStart != "2026-03-04" || plan.fetchEnd != "2026-03-06" {
			t.Fatalf("window = %s..%s, want 2026-03-04..2026-03-06（只抓缺的）", plan.fetchStart, plan.fetchEnd)
		}
		if len(plan.wanted) != 3 || !plan.wanted["2026-03-04"] || plan.wanted["2026-03-02"] {
			t.Fatalf("wanted = %v, want the 3 missing trading days only", plan.wanted)
		}
	})

	t.Run("complete_makes_no_call", func(t *testing.T) {
		existing := barsFor("2330.TW", "2026-03-02", "2026-03-03", "2026-03-04", "2026-03-05", "2026-03-06")
		plan := planSymbol(existing, all, false)
		if plan.fetchStart != "" || plan.fetchEnd != "" {
			t.Fatalf("window = %s..%s, want empty (no upstream call)", plan.fetchStart, plan.fetchEnd)
		}
		if len(plan.wanted) != 0 {
			t.Fatalf("wanted = %v, want empty", plan.wanted)
		}
	})

	t.Run("force_takes_whole_window", func(t *testing.T) {
		existing := barsFor("2330.TW", "2026-03-02")
		plan := planSymbol(existing, all, true)
		if plan.fetchStart != "2026-03-02" || plan.fetchEnd != "2026-03-06" {
			t.Fatalf("window = %s..%s, want the whole window in force mode", plan.fetchStart, plan.fetchEnd)
		}
		if len(plan.wanted) != 5 {
			t.Fatalf("wanted = %d days, want 5 in force mode", len(plan.wanted))
		}
	})

	t.Run("existing_rows_outside_window_are_not_wanted", func(t *testing.T) {
		// store 內有窗口外的舊列 ⇒ 不影響計畫，也不會被寫回去。
		existing := append(barsFor("2330.TW", "2026-01-06"), barsFor("2330.TW", "2026-03-02")...)
		plan := planSymbol(existing, all, false)
		if plan.wanted["2026-01-06"] {
			t.Fatal("窗口外的日期不得進入 wanted")
		}
	})
}

func TestResolveSymbols_ExplicitListNormalized(t *testing.T) {
	store := newTempSQLiteStore(t)
	got, err := resolveSymbols(context.Background(), store, "2317, 2330 ,2330,6488.TWO")
	if err != nil {
		t.Fatalf("resolveSymbols: %v", err)
	}
	want := []string{"2317.TW", "2330.TW", "6488.TWO"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("resolveSymbols = %v, want %v（裸代碼補 .TW、去重、排序）", got, want)
	}
}

// TestResolveSymbols_StoreKeysAreUsedVerbatim：省略 -symbols 時用 store 內**既有
// 的鍵**，不做形式轉換（否則會補到另一個鍵、查詢層看不到）。
func TestResolveSymbols_StoreKeysAreUsedVerbatim(t *testing.T) {
	store := newTempSQLiteStore(t)
	if err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "2330.TW", Date: mustDay(t, "2026-09-01"), Close: 1},
		{Symbol: "6488.TWO", Date: mustDay(t, "2026-09-01"), Close: 2},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for _, raw := range []string{"", " ", "all"} {
		got, err := resolveSymbols(context.Background(), store, raw)
		if err != nil {
			t.Fatalf("resolveSymbols(%q): %v", raw, err)
		}
		if strings.Join(got, ",") != "2330.TW,6488.TWO" {
			t.Fatalf("resolveSymbols(%q) = %v, want the stored keys verbatim", raw, got)
		}
	}
}

// TestResolveSymbols_StoreWithoutListingFailsLoudly：jsonl store 不能列舉 ⇒ 明確
// 要求 -symbols，不得靜默回空清單（那會變成「跑了但什麼都沒做」）。
func TestResolveSymbols_StoreWithoutListingFailsLoudly(t *testing.T) {
	store := ledger.NewJSONLQuoteStore(t.TempDir())
	_, err := resolveSymbols(context.Background(), store, "")
	if err == nil {
		t.Fatal("want error when the store cannot list symbols")
	}
	if !strings.Contains(err.Error(), "-symbols") {
		t.Fatalf("error %q must tell the operator to pass -symbols", err)
	}
}

// ─── 後端決策（#2107 形狀） ─────────────────────────────────────────────────

// recordingDeps 是「記錄並真的注入」的 wiring：斷言池真的被開、真的被注入。
func recordingDeps(t *testing.T) (storeDeps, *poolRecorder) {
	t.Helper()
	rec := &poolRecorder{fakePool: testdb.UnconnectedPool(t, "postgres://unit:unit@127.0.0.1:1/atlas_unit")}
	deps := storeDeps{
		initPool: func(_ context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error) {
			rec.initCalls++
			rec.dsn = dsn
			rec.migrationsPath = migrationsPath
			return rec.fakePool, nil
		},
		injectPool: func(pool *pgxpool.Pool) {
			rec.injected = append(rec.injected, pool)
			ledger.SetPostgresPool(pool)
		},
	}
	t.Cleanup(func() { ledger.SetPostgresPool(nil) })
	return deps, rec
}

type poolRecorder struct {
	fakePool       *pgxpool.Pool
	initCalls      int
	dsn            string
	migrationsPath string
	injected       []*pgxpool.Pool
}

// failingDeps 是「不該被呼叫」的 wiring：任何呼叫都讓測試紅燈。
func failingDeps(t *testing.T) storeDeps {
	t.Helper()
	return storeDeps{
		initPool: func(_ context.Context, dsn, migrationsPath string) (*pgxpool.Pool, error) {
			t.Errorf("initPool must not be called (dsn=%q migrations=%q)", dsn, migrationsPath)
			return nil, errors.New("initPool must not be called")
		},
		injectPool: func(*pgxpool.Pool) {
			t.Error("injectPool must not be called")
		},
	}
}

// TestResolveStore_EnvPostgresNeverFallsBackToSQLite 是 #2107 形狀的釘子：
// ATLAS_STORE_BACKEND=postgres 且沒有 DSN ⇒ 必須直接失敗，不得寫到 sqlite。
func TestResolveStore_EnvPostgresNeverFallsBackToSQLite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))
	t.Setenv("ATLAS_LEDGER_DIR", dir)
	t.Setenv("DATABASE_URL", "")

	appCfg := config.Load()
	store, _, err := resolveStore(context.Background(), cliConfig{}, appCfg, failingDeps(t))
	if err == nil {
		t.Fatal("want error: declared postgres without a DSN must not silently write sqlite")
	}
	if store != nil {
		t.Fatalf("want nil store on error, got %T", store)
	}
}

// TestResolveStore_ExplicitBackendWins 是上一條的**反假陽性對照**：同一組
// wiring／同一個斷言點，只要拿得到合法的 sqlite 設定就必須成功。
//
// 若上一條的紅燈是來自壞掉的 harness（而不是缺 DSN），這裡就會一起紅。
func TestResolveStore_ExplicitBackendWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")
	appCfg := config.Load()
	appCfg.SQLitePath = filepath.Join(dir, "atlas.db")

	store, closeStore, err := resolveStore(context.Background(), cliConfig{backend: "sqlite"}, appCfg, failingDeps(t))
	if err != nil {
		t.Fatalf("explicit -backend sqlite should work: %v", err)
	}
	defer closeStore()
	if _, ok := store.(*ledger.SQLiteQuoteStore); !ok {
		t.Fatalf("got %T, want *ledger.SQLiteQuoteStore", store)
	}
}

// TestResolveStore_PostgresWithoutDSNFailsWithDiagnosticMessage 是「無 DSN」的釘子。
//
// 為什麼斷言訊息內容：事故時只留下 “requires SetPostgresPool” 無法診斷（不知道
// 要補哪個 flag）。訊息必須指出 -pg-dsn 與 DATABASE_URL。
func TestResolveStore_PostgresWithoutDSNFailsWithDiagnosticMessage(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	appCfg := config.Load()

	store, closeStore, err := resolveStore(context.Background(), cliConfig{backend: "postgres"}, appCfg, failingDeps(t))
	if err == nil {
		t.Fatal("want error when backend=postgres has no DSN")
	}
	if store != nil {
		t.Fatalf("want nil store, got %T", store)
	}
	if closeStore != nil {
		t.Fatal("want nil closer on error")
	}
	msg := err.Error()
	for _, want := range []string{"postgres", "-pg-dsn", "DATABASE_URL"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q must mention %q (diagnosable, not just \"requires pool\")", msg, want)
		}
	}
	if strings.Contains(msg, "requires SetPostgresPool") {
		t.Fatalf("error %q still blames the pool instead of naming the missing DSN", msg)
	}
}

// TestResolveStore_PostgresWithDSNOpensAndInjectsPool：後端=postgres 且拿得到 DSN
// ⇒ 真的開池（seam）→ 真的注入 factory → 才建 store。突變（拿掉 injectPool 或
// initPool）必紅。
func TestResolveStore_PostgresWithDSNOpensAndInjectsPool(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	appCfg := config.Load()
	deps, rec := recordingDeps(t)

	store, closeStore, err := resolveStore(context.Background(),
		cliConfig{backend: "postgres", pgDSN: "postgres://u:p@localhost:5432/atlas", workDir: "/srv/atlas"},
		appCfg, deps)
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if rec.initCalls != 1 {
		t.Fatalf("initPool called %d times, want 1 (pool must actually be opened)", rec.initCalls)
	}
	if rec.dsn != "postgres://u:p@localhost:5432/atlas" {
		t.Fatalf("initPool dsn = %q, want the -pg-dsn value", rec.dsn)
	}
	if got, want := rec.migrationsPath, filepath.Join("/srv/atlas", "sql", "migrations"); got != want {
		t.Fatalf("migrations path = %q, want %q (<workdir>/sql/migrations)", got, want)
	}
	if len(rec.injected) != 1 || rec.injected[0] != rec.fakePool {
		t.Fatalf("injected pools = %v, want exactly the pool returned by initPool", rec.injected)
	}
	if _, ok := store.(*ledger.PostgresQuoteStore); !ok {
		t.Fatalf("got %T, want *ledger.PostgresQuoteStore (pool injection must reach the factory)", store)
	}
	if closeStore == nil {
		t.Fatal("want a closer that closes the pool")
	}
	closeStore()
}

func TestResolveStore_PgDSNFlagBeatsEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://env@localhost:5432/env")
	appCfg := config.Load()
	deps, rec := recordingDeps(t)

	_, closeStore, err := resolveStore(context.Background(),
		cliConfig{backend: "postgres", pgDSN: "postgres://flag@localhost:5432/flag"}, appCfg, deps)
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	defer closeStore()
	if rec.dsn != "postgres://flag@localhost:5432/flag" {
		t.Fatalf("dsn = %q, want the -pg-dsn value to win over DATABASE_URL", rec.dsn)
	}
}

func TestResolveStore_PostgresFallsBackToEnvDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://env@localhost:5432/atlas")
	appCfg := config.Load()
	deps, rec := recordingDeps(t)

	_, closeStore, err := resolveStore(context.Background(), cliConfig{backend: "postgres"}, appCfg, deps)
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	defer closeStore()
	if rec.dsn != "postgres://env@localhost:5432/atlas" {
		t.Fatalf("dsn = %q, want $DATABASE_URL", rec.dsn)
	}
	if got, want := rec.migrationsPath, filepath.Join(".", "sql", "migrations"); got != want {
		t.Fatalf("migrations path = %q, want %q", got, want)
	}
}

func TestResolveStore_NonPostgresBackendsNeverTouchDSN(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	dir := t.TempDir()

	for _, backend := range []string{"jsonl", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			appCfg := config.Config{LedgerDir: dir, SQLitePath: filepath.Join(dir, "atlas.db")}
			store, closeStore, err := resolveStore(context.Background(), cliConfig{backend: backend}, appCfg, failingDeps(t))
			if err != nil {
				t.Fatalf("backend %s must not require a DSN: %v", backend, err)
			}
			defer closeStore()
			if store == nil {
				t.Fatalf("backend %s: nil store", backend)
			}
		})
	}
}

func TestResolveStore_UnknownBackendFailsLoudly(t *testing.T) {
	appCfg := config.Load()
	if _, _, err := resolveStore(context.Background(), cliConfig{backend: "mysql"}, appCfg, failingDeps(t)); err == nil {
		t.Fatal("unknown backend must fail loudly, not fall back")
	}
}

func mustDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := parseExchangeDay(s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return d
}

func dayCfg(t *testing.T, start, end string) cliConfig {
	t.Helper()
	return cliConfig{start: mustDay(t, start), end: mustDay(t, end), resolvedBackend: "sqlite"}
}

// ─── runWith：寫入行為 ──────────────────────────────────────────────────────

// TestRunWith_WritesOnlyMissingTradingDays 是資料紀律的主測試：
//   - 已有列（2026-03-02）不被覆寫（name／source 原樣保留）
//   - 上游回的非交易日列（2026-03-07，週六）不寫入
//   - 只寫缺少的交易日，且 source 標籤是本 CLI 的 producer 標籤
func TestRunWith_WritesOnlyMissingTradingDays(t *testing.T) {
	store := newTempSQLiteStore(t)
	if err := store.RecordQuotes([]domain.DailyBar{{
		Symbol: "2330.TW", Name: "台積電", Date: mustDay(t, "2026-03-02"),
		Open: 500, High: 510, Low: 495, Close: 505, Volume: 1234, Source: "twse_open_data_csv",
	}}); err != nil {
		t.Fatalf("seed existing row: %v", err)
	}

	cfg := dayCfg(t, "2026-03-02", "2026-03-06")
	cfg.symbols = []string{"2330.TW"}
	fetcher := &fakeRangeFetcher{bars: [][]domain.DailyBar{
		barsFor("2330.TW", "2026-03-02", "2026-03-03", "2026-03-04", "2026-03-05", "2026-03-06", "2026-03-07"),
	}}
	var out bytes.Buffer
	if err := runWith(context.Background(), cfg, fetcher, store, &out); err != nil {
		t.Fatalf("runWith: %v\n%s", err, out.String())
	}

	// 呼叫窗口必須只涵蓋缺少的交易日（03-02 已有 ⇒ 從 03-03 起）。
	if len(fetcher.calls) != 1 {
		t.Fatalf("upstream calls = %v, want exactly 1 (range form)", fetcher.calls)
	}
	if fetcher.calls[0] != "2330.TW 2026-03-03..2026-03-06" {
		t.Fatalf("call = %q, want the missing-days window only", fetcher.calls[0])
	}

	loaded, err := store.LoadQuotes("2330.TW", cfg.start, cfg.end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	byDate := make(map[string]domain.DailyBar, len(loaded))
	for _, b := range loaded {
		byDate[b.Date.UTC().Format("2006-01-02")] = b
	}
	if len(loaded) != 5 {
		t.Fatalf("rows = %d, want 5 (03-02..03-06 trading days): %v", len(loaded), byDate)
	}
	if _, ok := byDate["2026-03-07"]; ok {
		t.Fatal("週六的列被寫入了（會污染下游 ForwardReturn 的重複偵測）")
	}
	kept := byDate["2026-03-02"]
	if kept.Name != "台積電" || kept.Source != "twse_open_data_csv" || kept.Close != 505 {
		t.Fatalf("既有列被覆寫了: %+v", kept)
	}
	for _, ds := range []string{"2026-03-03", "2026-03-04", "2026-03-05", "2026-03-06"} {
		if got := byDate[ds].Source; got != quotesRangeSource {
			t.Errorf("%s source = %q, want %q", ds, got, quotesRangeSource)
		}
	}

	for _, want := range []string{
		"requests=1", "rows_fetched=6", "rows_written=4", "skipped_not_wanted=2",
		"skipped_complete=0", "failures=0", "no_data=0", "backend=sqlite",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary missing %q:\n%s", want, out.String())
		}
	}
}

// TestRunWith_OneCallPerSymbolForWholeWindow 是**成本形狀**的釘子：一段 80 天的
// 窗口對 3 檔只能是 3 次呼叫。突變（改回 per-day 迴圈）⇒ 呼叫次數變成 3×交易日數。
func TestRunWith_OneCallPerSymbolForWholeWindow(t *testing.T) {
	store := newTempSQLiteStore(t)
	cfg := dayCfg(t, "2026-03-02", "2026-06-24")
	cfg.symbols = []string{"2330.TW", "2317.TW", "2454.TW"}

	fetcher := &fakeRangeFetcher{bars: [][]domain.DailyBar{
		barsFor("2330.TW", "2026-03-02", "2026-03-03"),
		barsFor("2317.TW", "2026-03-02"),
		barsFor("2454.TW", "2026-03-02"),
	}}
	var out bytes.Buffer
	if err := runWith(context.Background(), cfg, fetcher, store, &out); err != nil {
		t.Fatalf("runWith: %v\n%s", err, out.String())
	}
	tradingDays := len(tradingDaysBetween(cfg.start, cfg.end))
	if tradingDays < 60 {
		t.Fatalf("fixture expects a long window, got %d trading days", tradingDays)
	}
	if len(fetcher.calls) != len(cfg.symbols) {
		t.Fatalf("upstream calls = %d, want %d (= symbols × 1); per-day form would need %d",
			len(fetcher.calls), len(cfg.symbols), len(cfg.symbols)*tradingDays)
	}
	for _, want := range []string{"2330.TW 2026-03-02..2026-06-24", "2317.TW 2026-03-02..2026-06-24", "2454.TW 2026-03-02..2026-06-24"} {
		if !slicesContains(fetcher.calls, want) {
			t.Errorf("missing call %q (got %v)", want, fetcher.calls)
		}
	}
	if !strings.Contains(out.String(), fmt.Sprintf("requests=%d", len(cfg.symbols))) {
		t.Errorf("summary must report one request per symbol:\n%s", out.String())
	}
}

// TestRunWith_CompleteSymbolMakesNoUpstreamCall：store 已經有整個窗口的交易日 ⇒
// 不呼叫上游（重跑近乎免費，也是配額中止後續傳的機制）。
func TestRunWith_CompleteSymbolMakesNoUpstreamCall(t *testing.T) {
	store := newTempSQLiteStore(t)
	cfg := dayCfg(t, "2026-03-02", "2026-03-06")
	cfg.symbols = []string{"2330.TW"}
	if err := store.RecordQuotes(barsFor("2330.TW", "2026-03-02", "2026-03-03", "2026-03-04", "2026-03-05", "2026-03-06")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	fetcher := &fakeRangeFetcher{bars: [][]domain.DailyBar{barsFor("2330.TW", "2026-03-02")}}
	var out bytes.Buffer
	if err := runWith(context.Background(), cfg, fetcher, store, &out); err != nil {
		t.Fatalf("runWith: %v\n%s", err, out.String())
	}
	if len(fetcher.calls) != 0 {
		t.Fatalf("upstream calls = %v, want none (symbol already complete)", fetcher.calls)
	}
	for _, want := range []string{"requests=0", "skipped_complete=1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary missing %q:\n%s", want, out.String())
		}
	}
}

// TestRunWith_DryRunWritesNothing：dry-run 仍會抓（成本可估），但不落地。
func TestRunWith_DryRunWritesNothing(t *testing.T) {
	store := newTempSQLiteStore(t)
	cfg := dayCfg(t, "2026-03-02", "2026-03-04")
	cfg.symbols = []string{"2330.TW"}
	cfg.dryRun = true

	fetcher := &fakeRangeFetcher{bars: [][]domain.DailyBar{barsFor("2330.TW", "2026-03-02", "2026-03-03", "2026-03-04")}}
	var out bytes.Buffer
	if err := runWith(context.Background(), cfg, fetcher, store, &out); err != nil {
		t.Fatalf("runWith: %v\n%s", err, out.String())
	}
	if len(fetcher.calls) != 1 {
		t.Fatalf("dry-run must still fetch for the cost estimate, calls = %v", fetcher.calls)
	}
	loaded, err := store.LoadQuotes("2330.TW", cfg.start, cfg.end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("dry-run wrote %d rows", len(loaded))
	}
	if !strings.Contains(out.String(), "rows_written=0") || !strings.Contains(out.String(), "dry_run=true") {
		t.Errorf("summary must show a dry run:\n%s", out.String())
	}
}

// TestRunWith_ForceRefetchesWholeWindow：-force 才重抓整個窗口（含已存在的日期）。
func TestRunWith_ForceRefetchesWholeWindow(t *testing.T) {
	store := newTempSQLiteStore(t)
	cfg := dayCfg(t, "2026-03-02", "2026-03-04")
	cfg.symbols = []string{"2330.TW"}
	cfg.force = true
	if err := store.RecordQuotes(barsFor("2330.TW", "2026-03-02", "2026-03-03", "2026-03-04")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	fetcher := &fakeRangeFetcher{bars: [][]domain.DailyBar{barsFor("2330.TW", "2026-03-02", "2026-03-03", "2026-03-04")}}
	var out bytes.Buffer
	if err := runWith(context.Background(), cfg, fetcher, store, &out); err != nil {
		t.Fatalf("runWith: %v\n%s", err, out.String())
	}
	if len(fetcher.calls) != 1 {
		t.Fatalf("force must refetch the whole window, calls = %v", fetcher.calls)
	}
	if fetcher.calls[0] != "2330.TW 2026-03-02..2026-03-04" {
		t.Fatalf("call = %q, want the whole window in force mode", fetcher.calls[0])
	}
	if !strings.Contains(out.String(), "rows_written=3") {
		t.Errorf("force must rewrite every trading day:\n%s", out.String())
	}
}

// TestRunWith_AbortsOnQuotaExhausted：配額用完是「今天不能再打」⇒ 立即中止、
// 印出部分摘要、剩餘檔數不再呼叫上游（否則只是把 quota 燒在保證失敗的請求上）。
func TestRunWith_AbortsOnQuotaExhausted(t *testing.T) {
	store := newTempSQLiteStore(t)
	cfg := dayCfg(t, "2026-03-02", "2026-03-04")
	cfg.symbols = []string{"2330.TW", "2317.TW", "2454.TW"}

	fetcher := &fakeRangeFetcher{
		bars:   [][]domain.DailyBar{barsFor("2330.TW", "2026-03-02", "2026-03-03", "2026-03-04")},
		errFor: map[string]error{"2317.TW": fmt.Errorf("wrap: %w", marketdata.ErrQuotaExhausted)},
	}
	var out bytes.Buffer
	err := runWith(context.Background(), cfg, fetcher, store, &out)
	if err == nil {
		t.Fatalf("want an error on quota exhaustion\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "aborting") {
		t.Fatalf("err = %v, want an explicit abort (不繼續燒 quota)", err)
	}
	if !errors.Is(err, marketdata.ErrQuotaExhausted) {
		t.Fatalf("err = %v, want the typed sentinel preserved", err)
	}
	if len(fetcher.calls) != 2 {
		t.Fatalf("calls = %v, want 2 (第三檔必須在配額中止後不再呼叫)", fetcher.calls)
	}
	// 中止前已完成的檔數必須已經落地（部分成功是預期行為）。
	loaded, err2 := store.LoadQuotes("2330.TW", cfg.start, cfg.end)
	if err2 != nil {
		t.Fatalf("load: %v", err2)
	}
	if len(loaded) != 3 {
		t.Fatalf("first symbol rows = %d, want 3 (already written before the abort)", len(loaded))
	}
	// 部分摘要必須印出來（否則中止現場沒有帳）。
	if !strings.Contains(out.String(), "requests=2") {
		t.Errorf("partial summary missing:\n%s", out.String())
	}
}

// TestRunWith_NoDataIsWarningNotFailure：整段窗口沒有資料（下市／代碼錯誤）是
// 警告，不是失敗 —— 1,600 檔裡有一檔下市不該讓整批回補看起來「失敗」。
func TestRunWith_NoDataIsWarningNotFailure(t *testing.T) {
	store := newTempSQLiteStore(t)
	cfg := dayCfg(t, "2026-03-02", "2026-03-04")
	cfg.symbols = []string{"2330.TW", "9999.TW"}

	fetcher := &fakeRangeFetcher{
		bars:   [][]domain.DailyBar{barsFor("2330.TW", "2026-03-02")},
		errFor: map[string]error{"9999.TW": fmt.Errorf("finmind: %w", marketdata.ErrNoDataForSymbol)},
	}
	var out bytes.Buffer
	if err := runWith(context.Background(), cfg, fetcher, store, &out); err != nil {
		t.Fatalf("no-data must not fail the run: %v\n%s", err, out.String())
	}
	for _, want := range []string{"no_data=1", "failures=0"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("summary missing %q:\n%s", want, out.String())
		}
	}
}

// TestRunWith_FetchErrorContinuesAndFailsAtTheEnd：單檔傳輸錯誤 ⇒ 跳過並在最後
// 以非零回報（不掩蓋）。
func TestRunWith_FetchErrorContinuesAndFailsAtTheEnd(t *testing.T) {
	store := newTempSQLiteStore(t)
	cfg := dayCfg(t, "2026-03-02", "2026-03-04")
	cfg.symbols = []string{"2330.TW", "2317.TW", "2454.TW"}

	fetcher := &fakeRangeFetcher{
		bars: [][]domain.DailyBar{
			barsFor("2330.TW", "2026-03-02"),
			barsFor("2454.TW", "2026-03-02"),
		},
		errFor: map[string]error{"2317.TW": errors.New("boom")},
	}
	var out bytes.Buffer
	err := runWith(context.Background(), cfg, fetcher, store, &out)
	if err == nil {
		t.Fatalf("want a non-zero result when a symbol failed\n%s", out.String())
	}
	if len(fetcher.calls) != 3 {
		t.Fatalf("calls = %v, want 3 (one bad symbol must not stop the rest)", fetcher.calls)
	}
	if !strings.Contains(out.String(), "failures=1") {
		t.Errorf("summary missing failures=1:\n%s", out.String())
	}
}

// TestRunWith_CancelledContextStopsEarly：Ctrl-C ⇒ 印出部分摘要後回錯。
func TestRunWith_CancelledContextStopsEarly(t *testing.T) {
	store := newTempSQLiteStore(t)
	cfg := dayCfg(t, "2026-03-02", "2026-03-04")
	cfg.symbols = []string{"2330.TW", "2317.TW"}

	ctx, cancel := context.WithCancel(context.Background())
	fetcher := &fakeRangeFetcher{bars: [][]domain.DailyBar{barsFor("2330.TW", "2026-03-02")}}
	fetcher.afterCall = func(n int) {
		if n == 1 {
			cancel()
		}
	}

	var out bytes.Buffer
	err := runWith(ctx, cfg, fetcher, store, &out)
	if err == nil {
		t.Fatalf("want an error on cancellation\n%s", out.String())
	}
	if len(fetcher.calls) != 1 {
		t.Fatalf("calls = %v, want 1 (cancellation must stop the loop)", fetcher.calls)
	}
	if !strings.Contains(out.String(), "symbols=2") {
		t.Errorf("partial summary missing:\n%s", out.String())
	}
}

// ─── run()：真 flag 解析 ＋ 後端決策 ────────────────────────────────────────

// TestRun_DeclaredPostgresWithoutDSNWritesNoSQLiteFile 是 #2107 在 **CLI 邊界**的
// 釘子：環境宣告 postgres、沒有 DSN ⇒ run() 必須失敗，而且**本機 sqlite 檔不得
// 被建出來**（「不得降級寫 sqlite」的檔案級證據，不只是錯誤訊息）。
func TestRun_DeclaredPostgresWithoutDSNWritesNoSQLiteFile(t *testing.T) {
	dir := t.TempDir()
	sqlitePath := filepath.Join(dir, "atlas.db")
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("ATLAS_SQLITE_PATH", sqlitePath)
	t.Setenv("ATLAS_LEDGER_DIR", dir)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("FINMIND_API_KEY", "unit-test-key")

	var out bytes.Buffer
	// -workdir 也指向 temp：即使未來這裡改成建得出 provider，配額狀態檔也不會
	// 落進套件目錄（2026-09-29 誤入版控事故的形狀）。
	err := run([]string{"-start", "2026-03-02", "-end", "2026-03-04", "-symbols", "2330", "-workdir", dir}, &out)
	if err == nil {
		t.Fatalf("want error: declared postgres without a DSN\n%s", out.String())
	}
	for _, want := range []string{"postgres", "-pg-dsn", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, must mention %q", err, want)
		}
	}
	if _, statErr := os.Stat(sqlitePath); statErr == nil {
		t.Fatal("sqlite artifact was created although postgres was declared (降級寫 sqlite)")
	}
}

// TestRun_ArgumentValidation 先驗證參數，才碰 store／網路。
func TestRun_ArgumentValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"missing_start", nil, "-start"},
		{"bad_start", []string{"-start", "2026/03/02"}, "parse -start"},
		{"bad_end", []string{"-start", "2026-03-02", "-end", "x"}, "parse -end"},
		{"end_before_start", []string{"-start", "2026-03-04", "-end", "2026-03-02"}, "before -start"},
		{"no_trading_day", []string{"-start", "2026-01-01", "-end", "2026-01-01"}, "no Taiwan trading day"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(tc.args, &out)
			if err == nil {
				t.Fatalf("want an error for %v", tc.args)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestRun_HappyPathThroughRealFlagParsing 走完整條路（真 flag 解析 → 真後端決策
// → 真 sqlite 寫入），provider 由 newFetcher seam 換成 fake（不連網、不需 key）。
func TestRun_HappyPathThroughRealFlagParsing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))
	t.Setenv("ATLAS_LEDGER_DIR", dir)

	fake := &fakeRangeFetcher{bars: [][]domain.DailyBar{barsFor("2330.TW", "2026-03-02", "2026-03-03", "2026-03-04")}}
	restore := swapFetcher(t, fake)

	var out bytes.Buffer
	err := run([]string{"-start", "2026-03-02", "-end", "2026-03-04", "-symbols", "2330", "-workdir", dir}, &out)
	restore()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	// 裸代碼 2330 ⇒ 生產鍵形式 2330.TW（否則補出來的列會是另一個鍵）。
	if len(fake.calls) != 1 || !strings.HasPrefix(fake.calls[0], "2330.TW ") {
		t.Fatalf("calls = %v, want the 2330.TW key", fake.calls)
	}
	store, err := ledger.NewSQLiteQuoteStoreFromPath(filepath.Join(dir, "atlas.db"))
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	start := mustDay(t, "2026-03-02")
	end := mustDay(t, "2026-03-04")
	loaded, err := store.LoadQuotes("2330.TW", start, end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("rows = %d, want 3\n%s", len(loaded), out.String())
	}
	for _, b := range loaded {
		if b.Date.UTC() != b.Date {
			t.Fatalf("date %v is not UTC midnight（sqlite 與 postgres 會寫出不同天）", b.Date)
		}
		if b.Source != quotesRangeSource {
			t.Fatalf("source = %q, want %q", b.Source, quotesRangeSource)
		}
	}
	if !strings.Contains(out.String(), "backend=sqlite") {
		t.Errorf("summary must name the resolved backend:\n%s", out.String())
	}
}

// TestRun_NoSymbolsUsesStoreKeys：省略 -symbols ⇒ 用 store 內既有的鍵（含後綴
// 形式），並對每個鍵各發一次呼叫。
func TestRun_NoSymbolsUsesStoreKeys(t *testing.T) {
	dir := t.TempDir()
	sqlitePath := filepath.Join(dir, "atlas.db")
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("ATLAS_SQLITE_PATH", sqlitePath)

	seed := newTempSQLiteStoreAt(t, sqlitePath)
	if err := seed.RecordQuotes([]domain.DailyBar{
		{Symbol: "2317.TW", Date: mustDay(t, "2026-01-06"), Close: 1},
		{Symbol: "6488.TWO", Date: mustDay(t, "2026-01-06"), Close: 2},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	fake := &fakeRangeFetcher{bars: [][]domain.DailyBar{
		barsFor("2317.TW", "2026-03-02"),
		barsFor("6488.TWO", "2026-03-02"),
	}}
	restore := swapFetcher(t, fake)
	var out bytes.Buffer
	err := run([]string{"-start", "2026-03-02", "-end", "2026-03-03", "-workdir", dir}, &out)
	restore()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if len(fake.calls) != 2 {
		t.Fatalf("calls = %v, want one per stored key", fake.calls)
	}
	for _, want := range []string{"2317.TW 2026-03-02..2026-03-03", "6488.TWO 2026-03-02..2026-03-03"} {
		if !slicesContains(fake.calls, want) {
			t.Errorf("missing call %q (got %v)", want, fake.calls)
		}
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────

// swapFetcher 換掉 newFetcher seam 並回傳還原函式。
func swapFetcher(t *testing.T, f stockPriceRangeFetcher) func() {
	t.Helper()
	prev := newFetcher
	newFetcher = func(config.Config, string) (stockPriceRangeFetcher, error) { return f, nil }
	return func() { newFetcher = prev }
}

// swapFetcherRecordingStateDir 換掉 newFetcher seam 並記錄它收到的 stateDir
// （用來釘住配額狀態檔的落點），還原由 t.Cleanup 負責。
func swapFetcherRecordingStateDir(t *testing.T, f stockPriceRangeFetcher) *[]string {
	t.Helper()
	prev := newFetcher
	seen := []string{}
	newFetcher = func(_ config.Config, stateDir string) (stockPriceRangeFetcher, error) {
		seen = append(seen, stateDir)
		return f, nil
	}
	t.Cleanup(func() { newFetcher = prev })
	return &seen
}

func newTempSQLiteStoreAt(t *testing.T, path string) *ledger.SQLiteQuoteStore {
	t.Helper()
	store, err := ledger.NewSQLiteQuoteStoreFromPath(path)
	if err != nil {
		t.Fatalf("open sqlite quote store at %s: %v", path, err)
	}
	return store
}

func slicesContains(hay []string, needle string) bool {
	for _, s := range hay {
		if s == needle {
			return true
		}
	}
	return false
}

// ─── 配額狀態檔的落點（2026-09-29 誤入版控事故的釘子） ───────────────────────

// TestStateDirFor 釘住預設值：<workdir>/data/state，**不是 cwd**。
func TestStateDirFor(t *testing.T) {
	if got, want := stateDirFor("/srv/atlas", ""), filepath.Join("/srv/atlas", "data", "state"); got != want {
		t.Fatalf("stateDirFor = %q, want %q", got, want)
	}
	if got := stateDirFor("", ""); got != filepath.Join(".", "data", "state") {
		t.Fatalf("stateDirFor(\"\", \"\") = %q, want ./data/state", got)
	}
	if got := stateDirFor("/srv/atlas", "/tmp/state"); got != "/tmp/state" {
		t.Fatalf("explicit -state-dir must win, got %q", got)
	}
	// 負控制：預設值不得等於 cwd（那正是事故形狀）。
	if got := stateDirFor("/srv/atlas", ""); got == "/srv/atlas" || got == "." {
		t.Fatalf("default state dir must not be the working directory, got %q", got)
	}
}

// TestRun_QuotaStateDirLandsUnderWorkdir 是事故形狀的端到端釘子：
// run() 必須把 stateDir 傳成 <workdir>/data/state（顯式 -state-dir 則覆寫），
// 且**不得**在 cwd 產生 finmind_daily_quota.json。
func TestRun_QuotaStateDirLandsUnderWorkdir(t *testing.T) {
	dir := t.TempDir()
	workdir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))

	seen := swapFetcherRecordingStateDir(t, &fakeRangeFetcher{
		bars: [][]domain.DailyBar{barsFor("2330.TW", "2026-03-02", "2026-03-03")},
	})

	var out bytes.Buffer
	if err := run([]string{
		"-start", "2026-03-02", "-end", "2026-03-03", "-symbols", "2330", "-workdir", workdir,
	}, &out); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if len(*seen) != 1 {
		t.Fatalf("newFetcher called %d times, want 1", len(*seen))
	}
	if want := filepath.Join(workdir, "data", "state"); (*seen)[0] != want {
		t.Fatalf("stateDir = %q, want %q（runtime state 不得落在 cwd）", (*seen)[0], want)
	}

	// 顯式 -state-dir 勝出。
	custom := t.TempDir()
	if err := run([]string{
		"-start", "2026-03-02", "-end", "2026-03-03", "-symbols", "2330",
		"-workdir", workdir, "-state-dir", custom,
	}, &out); err != nil {
		t.Fatalf("run with -state-dir: %v\n%s", err, out.String())
	}
	if (*seen)[1] != custom {
		t.Fatalf("stateDir = %q, want the explicit -state-dir %q", (*seen)[1], custom)
	}

	// 反向對照：cwd（＝測試的套件目錄）不得出現配額狀態檔。
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for _, name := range []string{"finmind_daily_quota.json", "finmind_daily_quota.json.lock"} {
		if _, statErr := os.Stat(filepath.Join(cwd, name)); statErr == nil {
			t.Fatalf("%s 出現在 cwd(%s)：runtime state 汙染了套件目錄", name, cwd)
		}
	}
}
