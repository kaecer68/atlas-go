package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// TestResolveStore_EnvPostgresNeverFallsBackToSQLite 是 #2107 形狀的釘子：
// ATLAS_STORE_BACKEND=postgres 且沒有連線池 ⇒ CLI 必須直接失敗，
// **不得**因為「沒帶 -backend」就寫到 sqlite。
func TestResolveStore_EnvPostgresNeverFallsBackToSQLite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("ATLAS_SQLITE_PATH", filepath.Join(dir, "atlas.db"))
	t.Setenv("ATLAS_LEDGER_DIR", dir)

	appCfg := config.Load()
	if _, err := resolveStore(cliConfig{}, appCfg); err == nil {
		t.Fatal("want error: declared postgres without a pool must not silently write sqlite")
	}
}

// TestResolveStore_ExplicitBackendWins -backend 覆寫環境宣告。
func TestResolveStore_ExplicitBackendWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	appCfg := config.Load()
	appCfg.SQLitePath = filepath.Join(dir, "atlas.db")

	store, err := resolveStore(cliConfig{backend: "sqlite"}, appCfg)
	if err != nil {
		t.Fatalf("explicit -backend sqlite should work: %v", err)
	}
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
