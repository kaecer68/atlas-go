//go:build integration

package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestFuturesBarStoreFactory_PostgresInjectedPoolWrites 釘住 store factory 的注入契約。
//
// 為什麼需要這一條（2026-09-29 生產缺口）：cmd/backfill-futures-bars 的 postgres 路徑
// 依賴「先 SetPostgresPool，再 NewFuturesBarStore」這個順序。單元測試只驗證了
// 「未注入 ⇒ 回錯」，integration 測試（futures_bar_store_postgres_test.go）則直接
// new 了 PostgresFuturesBarStore，兩者都沒有驗證「注入後 factory 產出的 store
// 能真的寫入 PG」。本檔補上這條，且它位於 internal/ledger ⇒ CI 的 integration job
// path gate 會跑到。
func TestFuturesBarStoreFactory_PostgresInjectedPoolWrites(t *testing.T) {
	pool := connectTestPG(t)
	SetPostgresPool(pool)
	// 全域狀態不可外洩：同 package 的單元測試 TestNewFuturesBarStore_PostgresRequiresPool
	// 依賴 postgresPool == nil。LIFO 讓本 cleanup 先於 pool.Close 執行。
	t.Cleanup(func() { SetPostgresPool(nil) })

	cleanupFuturesTables(t, pool)
	ctx := context.Background()

	store, err := NewFuturesBarStore(config.Config{StoreBackend: "postgres"})
	if err != nil {
		t.Fatalf("NewFuturesBarStore(postgres) after SetPostgresPool: %v", err)
	}
	if _, ok := store.(*PostgresFuturesBarStore); !ok {
		t.Fatalf("got %T, want *PostgresFuturesBarStore (injected pool must reach the factory)", store)
	}

	day := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	price := func(v float64) *float64 { return &v }
	size := func(v int64) *int64 { return &v }
	bars := []domain.FuturesBar{{
		Contract: "PGFUT", ContractMonth: "202610", TradeDate: day, Session: domain.SessionRegular,
		Open: price(47607), High: price(48091), Low: price(47496), Close: price(48077),
		Volume: size(37750), SettlementPrice: price(48053), OpenInterest: size(101502), Source: "integration-factory",
	}}
	if n, err := store.RecordFuturesBars(ctx, bars); err != nil || n != 1 {
		t.Fatalf("RecordFuturesBars = (%d, %v), want (1, nil)", n, err)
	}

	// 用獨立 SQL 查證（不信任 store 自己的讀取路徑）。
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM futures_bars WHERE contract = 'PGFUT'`).Scan(&count); err != nil {
		t.Fatalf("count futures_bars: %v", err)
	}
	if count != 1 {
		t.Fatalf("futures_bars rows = %d, want 1 (factory-created store must really write)", count)
	}

	loaded, err := store.LoadFuturesBars(ctx, "PGFUT", "", "", day, day)
	if err != nil {
		t.Fatalf("LoadFuturesBars: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d bars, want 1", len(loaded))
	}
}
