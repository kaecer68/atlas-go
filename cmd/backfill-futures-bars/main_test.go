package main

import (
	"bytes"
	"context"
	"errors"
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
)

// fakeFetcher 讓 CLI 的核心路徑可以在不連網的情況下被測。
type fakeFetcher struct {
	bars  []domain.FuturesBar
	err   error
	calls int
}

func (f *fakeFetcher) FetchFuturesBars(_ context.Context, _ []string, _, _ time.Time) ([]domain.FuturesBar, error) {
	f.calls++
	return f.bars, f.err
}

func (f *fakeFetcher) FetchLatestFuturesBars(_ context.Context, _ []string) ([]domain.FuturesBar, error) {
	f.calls++
	return f.bars, f.err
}

// SetObserver 記錄鉤子有被接上（provider 的 SetObserver 其生產消費者就是 CLI）。
func (f *fakeFetcher) SetObserver(o marketdata.FuturesBarsObserver) {
	if o == nil {
		panic("CLI must install an observer")
	}
	o.ObserveFuturesBarsFetch("taifex_csv", nil, len(f.bars))
}

func TestParseContracts(t *testing.T) {
	got, err := parseContracts("tx, MTX ,TX")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 2 || got[0] != "TX" || got[1] != "MTX" {
		t.Fatalf("parseContracts = %v, want [TX MTX] (dedup + upper)", got)
	}
	if _, err := parseContracts("NOPE"); err == nil {
		t.Fatal("unknown contract must fail loudly, not silently default")
	}
	if _, err := parseContracts(" , "); err == nil {
		t.Fatal("empty contract list must fail")
	}
}

// newUnconnectedPool 造一個「真的 pgxpool、但不會連線」的池：pgxpool.ParseConfig 只
// 解析 DSN，MinConns=0 時 NewWithConfig 不會建立任何連線。單元測試因此能驗證
// 注入與關閉（pool.Close）的真實行為，而不需要一台 PostgreSQL。
func newUnconnectedPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse fake dsn %q: %v", dsn, err)
	}
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new unconnected pool: %v", err)
	}
	return pool
}

// recordingDeps 回傳可觀測的 fake wiring：記錄 initPool／injectPool 的呼叫。
//
// 需要注入真 factory 的測試（要拿到 *ledger.PostgresFuturesBarStore）用
// recordingDeps：它在記錄之餘**真的**呼叫 ledger.SetPostgresPool，並在測試結束時
// 還原成 nil（避免全域狀態外洩到同 package 的其他測試）。
func recordingDeps(t *testing.T) (storeDeps, *poolRecorder) {
	t.Helper()
	rec := &poolRecorder{fakePool: newUnconnectedPool(t, "postgres://unit:unit@127.0.0.1:1/atlas_unit")}
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

// poolRecorder 記錄 wiring 依賴被怎麼呼叫。
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
// ATLAS_STORE_BACKEND=postgres 且沒有 DSN ⇒ CLI 必須直接失敗，
// **不得**因為「沒帶 -backend」就寫到 sqlite。
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

// TestResolveStore_ExplicitBackendWins -backend 覆寫環境宣告。
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
	if _, ok := store.(*ledger.SQLiteFuturesBarStore); !ok {
		t.Fatalf("got %T, want *ledger.SQLiteFuturesBarStore", store)
	}
}

// TestRunWith_WritesBarsAndRollovers 端到端（假 provider ＋ 真 sqlite store）：
// bar 進 futures_bars、splice 事件進 futures_rollovers。
func TestRunWith_WritesBarsAndRollovers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "atlas.db")
	store, err := ledger.NewSQLiteFuturesBarStoreFromPath(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	day := func(s string) time.Time {
		d, err := time.ParseInLocation("2006-01-02", s, time.UTC)
		if err != nil {
			t.Fatalf("parse %s: %v", s, err)
		}
		return d
	}
	price := func(v float64) *float64 { return &v }
	size := func(v int64) *int64 { return &v }
	bars := []domain.FuturesBar{
		{Contract: "TX", ContractMonth: "202606", TradeDate: day("2026-06-17"), Session: domain.SessionRegular,
			Open: price(101), High: price(103), Low: price(99), Close: price(102), Volume: size(12), OpenInterest: size(1002), Source: "fake"},
		{Contract: "TX", ContractMonth: "202607", TradeDate: day("2026-06-17"), Session: domain.SessionRegular,
			Open: price(109), High: price(111), Low: price(108), Close: price(110), Volume: size(20), OpenInterest: size(2000), Source: "fake"},
		{Contract: "TX", ContractMonth: "202607", TradeDate: day("2026-07-16"), Session: domain.SessionRegular,
			Open: price(112), High: price(114), Low: price(111), Close: price(113), Volume: size(22), OpenInterest: size(2002), Source: "fake"},
		{Contract: "TX", ContractMonth: "202608", TradeDate: day("2026-07-16"), Session: domain.SessionRegular,
			Open: price(119), High: price(121), Low: price(118), Close: price(120), Volume: size(30), OpenInterest: size(3000), Source: "fake"},
	}
	fake := &fakeFetcher{bars: bars}

	outFile, err := os.Create(filepath.Join(dir, "stdout.txt"))
	if err != nil {
		t.Fatalf("create out: %v", err)
	}
	defer func() { _ = outFile.Close() }()

	cfg := cliConfig{
		contracts: []string{"TX"}, start: day("2026-06-17"), end: day("2026-07-16"),
		source: "csv", rollovers: true,
	}
	var buf bytes.Buffer
	_ = buf
	if err := runWith(context.Background(), cfg, fake, store, config.Config{}, outFile); err != nil {
		t.Fatalf("runWith: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("fetcher called %d times, want 1", fake.calls)
	}

	loaded, err := store.LoadFuturesBars(context.Background(), "TX", "", "", cfg.start, cfg.end)
	if err != nil {
		t.Fatalf("load bars: %v", err)
	}
	if len(loaded) != 4 {
		t.Fatalf("want 4 bars written, got %d", len(loaded))
	}

	rollovers, err := store.LoadFuturesRollovers(context.Background(), "TX")
	if err != nil {
		t.Fatalf("load rollovers: %v", err)
	}
	if len(rollovers) != 1 {
		t.Fatalf("want 1 splice event, got %d: %+v", len(rollovers), rollovers)
	}
	if rollovers[0].PriceDiff == nil || *rollovers[0].PriceDiff != 8 {
		t.Fatalf("splice diff = %v, want 8 (110 − 102)", rollovers[0].PriceDiff)
	}
	if rollovers[0].RollDate.Format("2006-01-02") != "2026-06-17" {
		t.Fatalf("roll date = %s, want 2026-06-17", rollovers[0].RollDate.Format("2006-01-02"))
	}
}

// TestRunWith_DryRunDoesNotWrite dry-run 不得落地任何列。
func TestRunWith_DryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	store, err := ledger.NewSQLiteFuturesBarStoreFromPath(filepath.Join(dir, "atlas.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	d, _ := time.ParseInLocation("2006-01-02", "2026-09-24", time.UTC)
	fake := &fakeFetcher{bars: []domain.FuturesBar{{
		Contract: "TX", ContractMonth: "202610", TradeDate: d, Session: domain.SessionRegular, Source: "fake",
	}}}
	outFile, err := os.Create(filepath.Join(dir, "stdout.txt"))
	if err != nil {
		t.Fatalf("create out: %v", err)
	}
	defer func() { _ = outFile.Close() }()

	cfg := cliConfig{contracts: []string{"TX"}, start: d, end: d, source: "csv", dryRun: true, rollovers: true}
	if err := runWith(context.Background(), cfg, fake, store, config.Config{}, outFile); err != nil {
		t.Fatalf("runWith dry-run: %v", err)
	}
	loaded, err := store.LoadFuturesBars(context.Background(), "TX", "", "", d, d)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("dry-run wrote %d rows", len(loaded))
	}
}

// ---------------------------------------------------------------------------
// Postgres pool wiring（本檔新增；對應 2026-09-29 生產缺口）
// ---------------------------------------------------------------------------

// TestResolveStore_PostgresWithoutDSNFailsWithDiagnosticMessage 是「無 DSN」的釘子。
//
// 為什麼斷言的是**訊息**而非只斷言 err != nil：2026-09-29 的生產事故就是
// futures bars CLI 從不開池，最後只留下 “requires SetPostgresPool” 這種
// 無法診斷（不知道要補哪個 flag）的錯誤。訊息必須指出 -pg-dsn 與 DATABASE_URL。
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

// TestResolveStore_PostgresWithDSNOpensAndInjectsPool 是最關鍵的守門：
// 後端=postgres 且拿得到 DSN ⇒ **真的**呼叫 atlasdb.Init（seam）並把池注入 factory，
// 然後才建 store。突變（拿掉 injectPool 或 initPool）必紅。
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
	// 注入必須真的生效：store 要是 Postgres 實作，而不是「找不到池」的錯誤。
	pgStore, ok := store.(*ledger.PostgresFuturesBarStore)
	if !ok {
		t.Fatalf("got %T, want *ledger.PostgresFuturesBarStore (pool injection must reach the factory)", store)
	}
	if pgStore == nil {
		t.Fatal("nil postgres store")
	}
	if closeStore == nil {
		t.Fatal("want a closer that closes the pool")
	}
	closeStore()
}

// TestResolveStore_PgDSNFlagBeatsEnv -pg-dsn 覆寫 $DATABASE_URL。
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

// TestResolveStore_PostgresFallsBackToEnvDSN 未帶 -pg-dsn 時讀 $DATABASE_URL
// （a2a-dev 在生產就是用環境變數跑，不帶 flag）。
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
	// workDir 未設定 ⇒ 預設 "."（相對路徑）；仍必須是 sql/migrations。
	if got, want := rec.migrationsPath, filepath.Join(".", "sql", "migrations"); got != want {
		t.Fatalf("migrations path = %q, want %q", got, want)
	}
}

// TestResolveStore_NonPostgresBackendsNeverTouchDSN jsonl／sqlite 行為不變：
// 不讀 DSN、不開池、不注入（failingDeps 任何呼叫都會讓本測試紅）。
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
