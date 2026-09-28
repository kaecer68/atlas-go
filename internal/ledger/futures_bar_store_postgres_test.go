//go:build integration

package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestPostgresFuturesBarStore_RoundTrip 是 futures_bars / futures_rollovers 的
// Postgres 實測（需 DATABASE_URL；本機沒有 PG 時 skip，CI 有 PG 時必須跑）。
//
// 為什麼一定要有這條：生產是 Postgres-first，而 SQLite 路徑與 pgx 路徑對 NULL 的
// 表示法不同（database/sql.NullFloat64 vs pgx 原生 *float64）。只有 sqlite 測試會
// 讓 pgx 路徑的缺值語意（nil ↔ NULL）在真實 PG 上才第一次被驗證。
func TestPostgresFuturesBarStore_RoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupFuturesTables(t, pool)
	store := NewPostgresFuturesBarStore(pool)
	ctx := context.Background()

	day := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	price := func(v float64) *float64 { return &v }
	size := func(v int64) *int64 { return &v }
	bars := []domain.FuturesBar{
		{
			Contract: "PGFUT", ContractMonth: "202610", TradeDate: day, Session: domain.SessionRegular,
			Open: price(47607), High: price(48091), Low: price(47496), Close: price(48077),
			Volume: size(37750), SettlementPrice: price(48053), OpenInterest: size(101502), Source: "integration",
		},
		{
			// 盤後列：結算價與 OI 缺值 ⇒ 必須是 SQL NULL（不是 0）。
			Contract: "PGFUT", ContractMonth: "202610", TradeDate: day, Session: domain.SessionAfterHours,
			Open: price(47494), High: price(47584), Low: price(47208), Close: price(47405),
			Volume: size(21514), Source: "integration",
		},
	}
	if n, err := store.RecordFuturesBars(ctx, bars); err != nil || n != 2 {
		t.Fatalf("RecordFuturesBars = (%d, %v), want (2, nil)", n, err)
	}
	// 冪等：改價後重寫不得產生重複列。
	bars[0].Close = price(48080)
	if _, err := store.RecordFuturesBars(ctx, bars); err != nil {
		t.Fatalf("re-record: %v", err)
	}

	loaded, err := store.LoadFuturesBars(ctx, "PGFUT", "", "", day, day)
	if err != nil {
		t.Fatalf("LoadFuturesBars: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("want 2 rows after idempotent upsert, got %d", len(loaded))
	}
	regular := findBar(t, loaded, "202610", domain.SessionRegular)
	if regular.Close == nil || *regular.Close != 48080 {
		t.Fatalf("regular close = %v, want 48080 (upsert must overwrite)", regular.Close)
	}
	if regular.OpenInterest == nil || *regular.OpenInterest != 101502 {
		t.Fatalf("regular OI = %v, want 101502", regular.OpenInterest)
	}
	after := findBar(t, loaded, "202610", domain.SessionAfterHours)
	if after.OpenInterest != nil {
		t.Fatalf("after_hours OI = %d, want NULL/nil", *after.OpenInterest)
	}
	if after.SettlementPrice != nil {
		t.Fatalf("after_hours settlement = %v, want NULL/nil", *after.SettlementPrice)
	}

	latest, err := store.LoadLatestFuturesBars(ctx, "PGFUT")
	if err != nil {
		t.Fatalf("LoadLatestFuturesBars: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("want 2 latest rows (one per session), got %d", len(latest))
	}

	// splice 事件（含一個無效 splice：price_diff NULL）。
	diff := 7.5
	events := []domain.FuturesRollover{
		{Contract: "PGFUT", RollDate: day, FromMonth: "202609", ToMonth: "202610", PriceDiff: &diff},
		{Contract: "PGFUT", RollDate: day.AddDate(0, -1, 0), FromMonth: "202608", ToMonth: "202609"},
	}
	if _, err := store.RecordFuturesRollovers(ctx, events); err != nil {
		t.Fatalf("RecordFuturesRollovers: %v", err)
	}
	if _, err := store.RecordFuturesRollovers(ctx, events); err != nil {
		t.Fatalf("re-record rollovers: %v", err)
	}
	rollovers, err := store.LoadFuturesRollovers(ctx, "PGFUT")
	if err != nil {
		t.Fatalf("LoadFuturesRollovers: %v", err)
	}
	if len(rollovers) != 2 {
		t.Fatalf("want 2 rollovers, got %d", len(rollovers))
	}
	var valid, invalid int
	for _, r := range rollovers {
		if r.PriceDiff == nil {
			invalid++
			continue
		}
		valid++
		if *r.PriceDiff != diff {
			t.Fatalf("price diff = %v, want %v", *r.PriceDiff, diff)
		}
	}
	if valid != 1 || invalid != 1 {
		t.Fatalf("want 1 valid + 1 invalid splice, got valid=%d invalid=%d", valid, invalid)
	}
}

func cleanupFuturesTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	del := func() {
		_, _ = pool.Exec(ctx, "DELETE FROM futures_bars WHERE contract = 'PGFUT'")
		_, _ = pool.Exec(ctx, "DELETE FROM futures_rollovers WHERE contract = 'PGFUT'")
	}
	del()
	t.Cleanup(del)
}
