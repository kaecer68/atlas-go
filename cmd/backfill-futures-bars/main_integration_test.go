//go:build integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// pgFuturesCLIContract 是本檔測試專用的契約代碼（避免與生產 TX/MTX 資料互踩）。
const pgFuturesCLIContract = "PGFUTCLI"

// repoRootFromCmd 回傳 repo 根（cmd/backfill-futures-bars → ../..）。
const repoRootFromCmd = "../.."

// TestResolveStore_PostgresWritesFuturesBars 是 2026-09-29 生產缺口的端到端證據。
//
// 缺口：CLI 從不建立 pgx pool，於是生產（ATLAS_STORE_BACKEND=postgres）一跑就
// 「requires SetPostgresPool」，期貨 bar 的回補在生產不可用。
//
// 這條測試走**生產形狀**：環境宣告 postgres ＋ $DATABASE_URL，且用
// defaultStoreDeps()（真 atlasdb.Init ＋ 真 ledger.SetPostgresPool）。
// 任一處斷線都會紅：
//   - DSN 沒讀到 ⇒ resolveStore 回錯
//   - 池沒開／沒注入 ⇒ 建不出 PostgresFuturesBarStore
//   - migrations 路徑錯（workDir 沒接）⇒ atlasdb.Init 失敗
//   - 寫入沒真的進 PG ⇒ 用**另一條連線**查不到列
func TestResolveStore_PostgresWritesFuturesBars(t *testing.T) {
	dsn := testdb.URL(t)
	// 用既有 testdb seam 套 migrations（CI 缺 PG 硬失敗、本機缺 PG skip 的政策在 testdb）。
	testdb.Pool(t, filepath.Join(repoRootFromCmd, "sql", "migrations"))

	raw := testdb.Connect(t, dsn)
	cleanupPGFuturesCLI(t, raw)

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", dsn)
	appCfg := config.Load()

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
		{Contract: pgFuturesCLIContract, ContractMonth: "202606", TradeDate: day("2026-06-17"), Session: domain.SessionRegular,
			Open: price(101), High: price(103), Low: price(99), Close: price(102), Volume: size(12), OpenInterest: size(1002), Source: "integration"},
		{Contract: pgFuturesCLIContract, ContractMonth: "202607", TradeDate: day("2026-06-17"), Session: domain.SessionRegular,
			Open: price(109), High: price(111), Low: price(108), Close: price(110), Volume: size(20), OpenInterest: size(2000), Source: "integration"},
		{Contract: pgFuturesCLIContract, ContractMonth: "202607", TradeDate: day("2026-07-16"), Session: domain.SessionRegular,
			Open: price(112), High: price(114), Low: price(111), Close: price(113), Volume: size(22), OpenInterest: size(2002), Source: "integration"},
		{Contract: pgFuturesCLIContract, ContractMonth: "202608", TradeDate: day("2026-07-16"), Session: domain.SessionRegular,
			Open: price(119), High: price(121), Low: price(118), Close: price(120), Volume: size(30), OpenInterest: size(3000), Source: "integration"},
	}

	cfg := cliConfig{
		contracts: []string{pgFuturesCLIContract}, start: day("2026-06-17"), end: day("2026-07-16"),
		source: "csv", rollovers: true, workDir: repoRootFromCmd,
	}

	store, closeStore, err := resolveStore(context.Background(), cfg, appCfg, defaultStoreDeps())
	if err != nil {
		t.Fatalf("resolveStore(postgres, env DSN): %v", err)
	}
	defer closeStore()

	outFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatalf("create stdout temp: %v", err)
	}
	defer func() { _ = outFile.Close() }()

	fake := &fakeFetcher{bars: bars}
	if err := runWith(context.Background(), cfg, fake, store, appCfg, outFile); err != nil {
		t.Fatalf("runWith: %v", err)
	}

	// 用**獨立**連線查證（不信任 store 自己的讀取路徑）。
	ctx := context.Background()
	var barCount int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM futures_bars WHERE contract = $1`, pgFuturesCLIContract).Scan(&barCount); err != nil {
		t.Fatalf("count futures_bars: %v", err)
	}
	if barCount != len(bars) {
		t.Fatalf("futures_bars rows = %d, want %d (backfill must really write to PostgreSQL)", barCount, len(bars))
	}

	var closePrice float64
	if err := raw.QueryRow(ctx, `
		SELECT close FROM futures_bars
		WHERE contract = $1 AND contract_month = '202607' AND session = 'regular'
		  AND trade_date = '2026-06-17'`, pgFuturesCLIContract).Scan(&closePrice); err != nil {
		t.Fatalf("select close: %v", err)
	}
	if closePrice != 110 {
		t.Fatalf("close = %v, want 110", closePrice)
	}

	var rolloverCount int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM futures_rollovers WHERE contract = $1`, pgFuturesCLIContract).Scan(&rolloverCount); err != nil {
		t.Fatalf("count futures_rollovers: %v", err)
	}
	if rolloverCount != 1 {
		t.Fatalf("futures_rollovers rows = %d, want 1", rolloverCount)
	}
	var priceDiff float64
	if err := raw.QueryRow(ctx, `SELECT price_diff FROM futures_rollovers WHERE contract = $1`, pgFuturesCLIContract).Scan(&priceDiff); err != nil {
		t.Fatalf("select price_diff: %v", err)
	}
	if priceDiff != 8 {
		t.Fatalf("splice price_diff = %v, want 8 (110 - 102)", priceDiff)
	}
}

// TestResolveStore_PostgresEndToEndClosesPool 驗證 closer 真的把池關掉
// （CLI 的 run() 依賴它；關不掉的池會讓短期 CLI 進程累積連線）。
func TestResolveStore_PostgresEndToEndClosesPool(t *testing.T) {
	dsn := testdb.URL(t)
	testdb.Pool(t, filepath.Join(repoRootFromCmd, "sql", "migrations"))

	ctx := context.Background()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", dsn)
	appCfg := config.Load()

	cfg := cliConfig{
		contracts: []string{pgFuturesCLIContract}, start: time.Now(), end: time.Now(),
		workDir: repoRootFromCmd, pgDSN: dsn,
	}
	store, closeStore, err := resolveStore(ctx, cfg, appCfg, defaultStoreDeps())
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if _, ok := store.(*ledger.PostgresFuturesBarStore); !ok {
		t.Fatalf("got %T, want *ledger.PostgresFuturesBarStore", store)
	}
	closeStore()

	// 關閉後的池不可再查詢。
	if _, err := store.LoadLatestFuturesBars(ctx, pgFuturesCLIContract); err == nil {
		t.Fatal("want error after closeStore(): the pool must actually be closed")
	}
}

func cleanupPGFuturesCLI(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	del := func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, "DELETE FROM futures_bars WHERE contract = $1", pgFuturesCLIContract)
		_, _ = pool.Exec(ctx, "DELETE FROM futures_rollovers WHERE contract = $1", pgFuturesCLIContract)
	}
	del()
	t.Cleanup(del)
}
