package ledger

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/futures"
)

func shadowDay(s string) time.Time {
	d, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return d
}

func shadowPtrF(v float64) *float64 { return &v }
func shadowPtrI(v int64) *int64     { return &v }
func shadowPtrB(v bool) *bool       { return &v }

// shadowSamples 是一組含「有標籤」與「無標籤」的影子列。
func shadowSamples() []FuturesShadowRow {
	const mv = "test-shadow-v1"
	return []FuturesShadowRow{
		{
			ModelVersion: mv,
			Sample: futures.ShadowSample{
				TradeDate: shadowDay("2026-01-05"), Contract: "TX", NearMonth: "202601",
				TermStructure: "contango", SpreadPoints: 10, SpreadBP: 1000,
				OINearTrend: "up", OINearChangePct: 10, NearReturnPct: 2,
				ForeignOIChange: shadowPtrI(1000), PCROIRatio: shadowPtrF(0.75), PCRDelta: shadowPtrF(0.125),
				Score: -1, TermsUsed: 2, Predicted: futures.DirectionDown, Confidence: 1,
				LabelReturnPct: shadowPtrF(2), ActualDirection: futures.DirectionUp, Hit: shadowPtrB(false),
			},
		},
		{
			// 序列末端：沒有標籤 ⇒ 三個標籤欄位必須是 SQL NULL（不是 0 / 'neutral'）。
			ModelVersion: mv,
			Sample: futures.ShadowSample{
				TradeDate: shadowDay("2026-01-07"), Contract: "TX", NearMonth: "202601",
				TermStructure: "contango", SpreadPoints: 11, SpreadBP: 1089,
				OINearTrend: "up", OINearChangePct: 4.5, NearReturnPct: -0.98,
				Score: -1, TermsUsed: 2, Predicted: futures.DirectionDown, Confidence: 1,
			},
		},
	}
}

// TestNewFuturesShadowStore_PostgresRequiresPool 是 #2107 形狀的釘子（影子儲存亦適用）。
func TestNewFuturesShadowStore_PostgresRequiresPool(t *testing.T) {
	prev := postgresPool
	postgresPool = nil
	t.Cleanup(func() { postgresPool = prev })

	dir := t.TempDir()
	cfg := config.Config{StoreBackend: "postgres", SQLitePath: filepath.Join(dir, "atlas.db"), LedgerDir: dir}
	store, err := NewFuturesShadowStore(cfg)
	if err == nil {
		t.Fatalf("want error when postgres pool is missing, got store %T", store)
	}
	if store != nil {
		t.Fatal("must not return a store when postgres was declared but unavailable")
	}
}

func TestNewFuturesShadowStore_EnvPostgresNeverFallsBackToSQLite(t *testing.T) {
	prev := postgresPool
	postgresPool = nil
	t.Cleanup(func() { postgresPool = prev })

	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))
	t.Setenv("ATLAS_LEDGER_DIR", dir)

	cfg := config.Load()
	if cfg.StoreBackend != "postgres" {
		t.Fatalf("env not honoured: %q", cfg.StoreBackend)
	}
	if _, err := NewFuturesShadowStore(cfg); err == nil {
		t.Fatal("declared postgres without a pool must fail, not silently write sqlite")
	}
	// 證明同環境下 sqlite 可用（排除假陽性）。
	sqliteStore, err := NewFuturesShadowStore(config.Config{StoreBackend: "sqlite", SQLitePath: cfg.SQLitePath})
	if err != nil {
		t.Fatalf("sqlite backend should work in the same environment: %v", err)
	}
	if _, ok := sqliteStore.(*SQLiteFuturesShadowStore); !ok {
		t.Fatalf("got %T, want *SQLiteFuturesShadowStore", sqliteStore)
	}
}

func TestNewFuturesShadowStore_UnknownBackendAndEmptySQLitePath(t *testing.T) {
	if _, err := NewFuturesShadowStore(config.Config{StoreBackend: "mysql"}); err == nil {
		t.Fatal("unknown backend must fail loudly")
	}
	if _, err := NewFuturesShadowStore(config.Config{StoreBackend: "sqlite"}); err == nil {
		t.Fatal("sqlite backend without a path must fail")
	}
}

// TestSQLiteFuturesShadowStore_RoundTrip 驗證冪等、NULL 語意、版本過濾。
func TestSQLiteFuturesShadowStore_RoundTrip(t *testing.T) {
	store, err := NewSQLiteFuturesShadowStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	rows := shadowSamples()

	n, err := store.RecordFuturesShadow(ctx, rows)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if n != 2 {
		t.Fatalf("recorded %d, want 2", n)
	}
	// 冪等：同鍵重寫不得產生重複列（改分數確認是覆寫）。
	rows[0].Sample.Score = -0.5
	if _, err := store.RecordFuturesShadow(ctx, rows); err != nil {
		t.Fatalf("re-record: %v", err)
	}

	loaded, err := store.LoadFuturesShadow(ctx, "TX", "", shadowDay("2026-01-01"), shadowDay("2026-01-31"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("want 2 rows after idempotent upsert, got %d", len(loaded))
	}
	if loaded[0].Sample.Score != -0.5 {
		t.Fatalf("first row score = %v, want -0.5 (upsert must overwrite)", loaded[0].Sample.Score)
	}
	if loaded[0].Sample.ForeignOIChange == nil || *loaded[0].Sample.ForeignOIChange != 1000 {
		t.Fatalf("foreign OI change = %v, want 1000", loaded[0].Sample.ForeignOIChange)
	}
	if loaded[0].Sample.PCROIRatio == nil || *loaded[0].Sample.PCROIRatio != 0.75 {
		t.Fatalf("PCR ratio = %v, want 0.75", loaded[0].Sample.PCROIRatio)
	}
	if loaded[0].Sample.Hit == nil || *loaded[0].Sample.Hit {
		t.Fatalf("hit = %v, want false", loaded[0].Sample.Hit)
	}

	// 無標籤列：三個欄位必須是 nil（NULL），不得變成 0 / neutral。
	end := loaded[1]
	if end.Sample.LabelReturnPct != nil || end.Sample.Hit != nil || end.Sample.ActualDirection != "" {
		t.Fatalf("unlabeled row must keep NULLs: label=%v hit=%v dir=%q",
			end.Sample.LabelReturnPct, end.Sample.Hit, end.Sample.ActualDirection)
	}
	if end.Sample.ForeignOIChange != nil || end.Sample.PCROIRatio != nil {
		t.Fatal("absent external inputs must stay NULL")
	}

	// 版本過濾：未知版本 ⇒ 0 列。
	other, err := store.LoadFuturesShadow(ctx, "TX", "other-v9", shadowDay("2026-01-01"), shadowDay("2026-01-31"))
	if err != nil {
		t.Fatalf("load other version: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("version filter failed: %d rows", len(other))
	}

	hr := futures.SummarizeHitRate(samplesOf(loaded))
	if hr.Total != 1 || hr.Hits != 0 || hr.Skipped != 1 {
		t.Fatalf("hit rate = %+v, want {Total:1 Hits:0 Skipped:1}", hr)
	}
}

func samplesOf(rows []FuturesShadowRow) []futures.ShadowSample {
	out := make([]futures.ShadowSample, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Sample)
	}
	return out
}

// TestFuturesShadowStore_RejectsEmptyModelVersion 版本標記不可為空（否則無法與 live 校準區隔）。
func TestFuturesShadowStore_RejectsEmptyModelVersion(t *testing.T) {
	store, err := NewSQLiteFuturesShadowStoreFromPath(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	row := shadowSamples()[0]
	row.ModelVersion = ""
	if _, err := store.RecordFuturesShadow(context.Background(), []FuturesShadowRow{row}); err == nil {
		t.Fatal("empty model version must be rejected")
	}
}

func TestJSONLFuturesShadowStore_RoundTrip(t *testing.T) {
	store := NewJSONLFuturesShadowStore(t.TempDir())
	ctx := context.Background()
	rows := shadowSamples()
	if _, err := store.RecordFuturesShadow(ctx, rows); err != nil {
		t.Fatalf("record: %v", err)
	}
	rows[0].Sample.Score = -0.5
	if _, err := store.RecordFuturesShadow(ctx, rows); err != nil {
		t.Fatalf("re-record: %v", err)
	}
	loaded, err := store.LoadFuturesShadow(ctx, "TX", "", shadowDay("2026-01-01"), shadowDay("2026-01-31"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("want 2 rows, got %d", len(loaded))
	}
	if loaded[0].Sample.Score != -0.5 {
		t.Fatalf("score = %v, want -0.5", loaded[0].Sample.Score)
	}
	if loaded[1].Sample.LabelReturnPct != nil || loaded[1].Sample.Hit != nil {
		t.Fatal("unlabeled row must keep nil label/hit")
	}
}
