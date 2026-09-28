package ledger

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func futPtr(v float64) *float64 { return &v }
func futInt(v int64) *int64     { return &v }

func sampleFuturesBars(t *testing.T) []domain.FuturesBar {
	t.Helper()
	day := func(s string) time.Time {
		ts, err := time.ParseInLocation("2006-01-02", s, time.UTC)
		if err != nil {
			t.Fatalf("parse %s: %v", s, err)
		}
		return ts
	}
	return []domain.FuturesBar{
		{
			Contract: "TX", ContractMonth: "202610", TradeDate: day("2026-09-21"),
			Session: domain.SessionRegular, Open: futPtr(47607), High: futPtr(48091),
			Low: futPtr(47496), Close: futPtr(48077), Volume: futInt(37750),
			SettlementPrice: futPtr(48053), OpenInterest: futInt(101502), Source: "taifex_csv",
		},
		{
			// 盤後列：OI 與結算價缺值 ⇒ 必須以 NULL 存取，不可變成 0。
			Contract: "TX", ContractMonth: "202610", TradeDate: day("2026-09-21"),
			Session: domain.SessionAfterHours, Open: futPtr(47494), High: futPtr(47584),
			Low: futPtr(47208), Close: futPtr(47405), Volume: futInt(21514), Source: "taifex_csv",
		},
	}
}

// findBar 以契約月＋時段找 bar。
func findBar(t *testing.T, bars []domain.FuturesBar, month string, session domain.FuturesSession) domain.FuturesBar {
	t.Helper()
	for _, b := range bars {
		if b.ContractMonth == month && b.Session == session {
			return b
		}
	}
	t.Fatalf("bar not found: month=%s session=%s (have %d)", month, session, len(bars))
	return domain.FuturesBar{}
}

// TestNewFuturesBarStore_PostgresRequiresPool 是 #2107 的護欄：
// 組態為 postgres 但沒有連線池 ⇒ **必須**回錯誤，不得靜默降級到 sqlite/jsonl。
func TestNewFuturesBarStore_PostgresRequiresPool(t *testing.T) {
	prev := postgresPool
	postgresPool = nil
	t.Cleanup(func() { postgresPool = prev })

	cfg := config.Config{StoreBackend: "postgres", SQLitePath: filepath.Join(t.TempDir(), "atlas.db"), LedgerDir: t.TempDir()}
	store, err := NewFuturesBarStore(cfg)
	if err == nil {
		t.Fatalf("want error when postgres pool is missing, got store %T", store)
	}
	if store != nil {
		t.Fatal("must not return a store on backend resolution failure")
	}
}

// TestNewFuturesBarStore_UnknownBackendFailsLoudly 未知後端不得 fallback。
func TestNewFuturesBarStore_UnknownBackendFailsLoudly(t *testing.T) {
	if _, err := NewFuturesBarStore(config.Config{StoreBackend: "mysql"}); err == nil {
		t.Fatal("want error for unsupported backend")
	}
}

// TestNewFuturesBarStore_SQLiteRequiresPath sqlite 後端缺 path ⇒ 錯誤（不可硬編路徑）。
func TestNewFuturesBarStore_SQLiteRequiresPath(t *testing.T) {
	if _, err := NewFuturesBarStore(config.Config{StoreBackend: "sqlite"}); err == nil {
		t.Fatal("want error when SQLitePath is empty")
	}
}

// TestNewFuturesBarStore_BackendDispatch 三個後端各自建立對應實作。
func TestNewFuturesBarStore_BackendDispatch(t *testing.T) {
	dir := t.TempDir()

	sqliteStore, err := NewFuturesBarStore(config.Config{StoreBackend: "sqlite", SQLitePath: filepath.Join(dir, "atlas.db")})
	if err != nil {
		t.Fatalf("sqlite backend: %v", err)
	}
	if _, ok := sqliteStore.(*SQLiteFuturesBarStore); !ok {
		t.Fatalf("sqlite backend produced %T", sqliteStore)
	}

	jsonlStore, err := NewFuturesBarStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("jsonl backend: %v", err)
	}
	if _, ok := jsonlStore.(*JSONLFuturesBarStore); !ok {
		t.Fatalf("jsonl backend produced %T", jsonlStore)
	}
}

// TestSQLiteFuturesBarStore_RoundTrip 驗證 upsert 冪等、缺值 NULL 語意、範圍查詢。
func TestSQLiteFuturesBarStore_RoundTrip(t *testing.T) {
	store, err := NewSQLiteFuturesBarStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	bars := sampleFuturesBars(t)

	n, err := store.RecordFuturesBars(ctx, bars)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if n != len(bars) {
		t.Fatalf("recorded %d, want %d", n, len(bars))
	}

	// 再一次：upsert 必須冪等（不是產生重複列）。
	bars[0].Close = futPtr(48080)
	if _, err := store.RecordFuturesBars(ctx, bars); err != nil {
		t.Fatalf("re-record: %v", err)
	}

	day := bars[0].TradeDate
	loaded, err := store.LoadFuturesBars(ctx, "TX", "", "", day, day)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("want 2 rows (upsert must be idempotent), got %d", len(loaded))
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
		t.Fatalf("after_hours OI = %d, want NULL/nil (must not become 0)", *after.OpenInterest)
	}
	if after.SettlementPrice != nil {
		t.Fatalf("after_hours settlement = %v, want NULL/nil", *after.SettlementPrice)
	}
	if after.Volume == nil || *after.Volume != 21514 {
		t.Fatalf("after_hours volume = %v, want 21514", after.Volume)
	}

	// 時段過濾。
	onlyRegular, err := store.LoadFuturesBars(ctx, "TX", "", domain.SessionRegular, day, day)
	if err != nil {
		t.Fatalf("load filtered: %v", err)
	}
	if len(onlyRegular) != 1 || onlyRegular[0].Session != domain.SessionRegular {
		t.Fatalf("session filter failed: %+v", onlyRegular)
	}

	// 範圍外不得回傳。
	other := day.AddDate(0, 0, 1)
	out, err := store.LoadFuturesBars(ctx, "TX", "", "", other, other)
	if err != nil {
		t.Fatalf("load out of range: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("range filter failed: %d rows outside window", len(out))
	}

	latest, err := store.LoadLatestFuturesBars(ctx, "TX")
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	if len(latest) == 0 {
		t.Fatal("load latest returned nothing")
	}
}

// TestJSONLFuturesBarStore_RoundTrip 同一組語意在 JSONL 後端也成立。
func TestJSONLFuturesBarStore_RoundTrip(t *testing.T) {
	store := NewJSONLFuturesBarStore(t.TempDir())
	ctx := context.Background()
	bars := sampleFuturesBars(t)

	if _, err := store.RecordFuturesBars(ctx, bars); err != nil {
		t.Fatalf("record: %v", err)
	}
	bars[0].Close = futPtr(48080)
	if _, err := store.RecordFuturesBars(ctx, bars); err != nil {
		t.Fatalf("re-record: %v", err)
	}

	day := bars[0].TradeDate
	loaded, err := store.LoadFuturesBars(ctx, "TX", "", "", day, day)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("want 2 rows after upsert, got %d", len(loaded))
	}
	regular := findBar(t, loaded, "202610", domain.SessionRegular)
	if regular.Close == nil || *regular.Close != 48080 {
		t.Fatalf("regular close = %v, want 48080", regular.Close)
	}
	after := findBar(t, loaded, "202610", domain.SessionAfterHours)
	if after.OpenInterest != nil {
		t.Fatalf("after_hours OI = %d, want nil", *after.OpenInterest)
	}
}

// ─── 後端判定釘子（#2107 教訓）──────────────────────────────────────────────

// TestNewFuturesBarStore_EnvPostgresNeverFallsBackToSQLite 是最直接的 #2107 釘子：
// 環境宣告 postgres、但沒有注入連線池時，**即使 SQLitePath 指向一個可用檔案**，
// 也必須回錯誤而不是靜默寫 sqlite。
func TestNewFuturesBarStore_EnvPostgresNeverFallsBackToSQLite(t *testing.T) {
	prev := postgresPool
	postgresPool = nil
	t.Cleanup(func() { postgresPool = prev })

	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))
	t.Setenv("ATLAS_LEDGER_DIR", dir)

	cfg := config.Load()
	if cfg.StoreBackend != "postgres" {
		t.Fatalf("env not honoured: StoreBackend = %q", cfg.StoreBackend)
	}
	store, err := NewFuturesBarStore(cfg)
	if err == nil {
		t.Fatalf("must refuse to serve a store when postgres was declared but unavailable (got %T)", store)
	}
	// 證明「sqlite 檔案確實可寫」—— 也就是說上面的錯誤不是因為環境壞掉，
	// 而是因為我們**刻意**不降級。
	sqliteStore, sqliteErr := NewFuturesBarStore(config.Config{StoreBackend: "sqlite", SQLitePath: cfg.SQLitePath})
	if sqliteErr != nil {
		t.Fatalf("sqlite backend should be usable in the same environment: %v", sqliteErr)
	}
	if _, ok := sqliteStore.(*SQLiteFuturesBarStore); !ok {
		t.Fatalf("sqlite backend produced %T", sqliteStore)
	}
}

// TestNewFuturesBarStore_EnvSQLiteHonoursPath 環境宣告 sqlite 時走 cfg.SQLitePath（不得硬編）。
func TestNewFuturesBarStore_EnvSQLiteHonoursPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-atlas.db")
	t.Setenv("ATLAS_STORE_BACKEND", "sqlite")
	t.Setenv("ATLAS_SQLITE_PATH", path)

	cfg := config.Load()
	store, err := NewFuturesBarStore(cfg)
	if err != nil {
		t.Fatalf("sqlite backend: %v", err)
	}
	if _, ok := store.(*SQLiteFuturesBarStore); !ok {
		t.Fatalf("got %T, want *SQLiteFuturesBarStore", store)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sqlite store did not create the configured path %s: %v", path, err)
	}
}

// ─── rollover splice 事件落庫（規格 §6.3 配套 2）─────────────────────────────

func TestSQLiteFuturesBarStore_RolloversRoundTrip(t *testing.T) {
	store, err := NewSQLiteFuturesBarStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	day, _ := time.ParseInLocation("2006-01-02", "2026-07-16", time.UTC)
	diff := 7.5
	events := []domain.FuturesRollover{
		{Contract: "TX", RollDate: day, FromMonth: "202607", ToMonth: "202608", PriceDiff: &diff},
		// 無效 splice：價差 nil 必須以 NULL 存取（不得變成 0）。
		{Contract: "TX", RollDate: day.AddDate(0, -1, 0), FromMonth: "202606", ToMonth: "202607"},
	}
	if _, err := store.RecordFuturesRollovers(ctx, events); err != nil {
		t.Fatalf("record rollovers: %v", err)
	}
	// 冪等：再寫一次不得產生重複列。
	if _, err := store.RecordFuturesRollovers(ctx, events); err != nil {
		t.Fatalf("re-record rollovers: %v", err)
	}

	loaded, err := store.LoadFuturesRollovers(ctx, "TX")
	if err != nil {
		t.Fatalf("load rollovers: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("want 2 rollovers after idempotent upsert, got %d", len(loaded))
	}
	var valid, invalid int
	for _, r := range loaded {
		if r.PriceDiff != nil {
			valid++
			if *r.PriceDiff != diff {
				t.Errorf("price diff = %v, want %v", *r.PriceDiff, diff)
			}
		} else {
			invalid++
		}
	}
	if valid != 1 || invalid != 1 {
		t.Fatalf("want 1 valid + 1 invalid splice, got valid=%d invalid=%d", valid, invalid)
	}
}

func TestJSONLFuturesBarStore_RolloversRoundTrip(t *testing.T) {
	store := NewJSONLFuturesBarStore(t.TempDir())
	ctx := context.Background()
	day, _ := time.ParseInLocation("2006-01-02", "2026-07-16", time.UTC)
	diff := 7.5
	events := []domain.FuturesRollover{{Contract: "TX", RollDate: day, FromMonth: "202607", ToMonth: "202608", PriceDiff: &diff}}
	if _, err := store.RecordFuturesRollovers(ctx, events); err != nil {
		t.Fatalf("record: %v", err)
	}
	if _, err := store.RecordFuturesRollovers(ctx, events); err != nil {
		t.Fatalf("re-record: %v", err)
	}
	loaded, err := store.LoadFuturesRollovers(ctx, "TX")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("want 1 rollover, got %d", len(loaded))
	}
	if loaded[0].PriceDiff == nil || *loaded[0].PriceDiff != diff {
		t.Fatalf("price diff = %v, want %v", loaded[0].PriceDiff, diff)
	}
}
