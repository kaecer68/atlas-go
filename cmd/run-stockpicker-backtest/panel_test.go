//go:build integration

package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// compile-time assertions
var _ ledger.QuoteSymbolLister = (*ledger.SQLiteQuoteStore)(nil)
var _ ledger.QuoteSymbolLister = (*ledger.PostgresQuoteStore)(nil)

// fakeQuoteStore is an in-memory QuoteStore for unit tests.
type fakeQuoteStore struct {
	quotes map[string][]domain.DailyBar
}

func (f *fakeQuoteStore) RecordQuotes(quotes []domain.DailyBar) error { return nil }

func (f *fakeQuoteStore) LoadQuotes(symbol string, start, end time.Time) ([]domain.DailyBar, error) {
	bars := f.quotes[symbol]
	var out []domain.DailyBar
	for _, b := range bars {
		if (b.Date.Equal(start) || b.Date.After(start)) && (b.Date.Equal(end) || b.Date.Before(end)) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (f *fakeQuoteStore) LoadLatestQuotes(symbols []string) (map[string]domain.DailyBar, error) {
	return nil, nil
}

func newFakeQuoteStore() *fakeQuoteStore {
	return &fakeQuoteStore{quotes: make(map[string][]domain.DailyBar)}
}

// asQuoteStore returns the fake store as the interface type (used for type
// assertions in tests).
func asQuoteStore(f *fakeQuoteStore) ledger.QuoteStore { return f }

// TestBars_SymbolSuffixFallback verifies that Bars prefers symbol.TW,
// falls back to bare symbol, then symbol.TWO.
func TestBars_SymbolSuffixFallback(t *testing.T) {
	ctx := context.Background()
	store := newFakeQuoteStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store.quotes["2330.TW"] = []domain.DailyBar{
		{Symbol: "2330.TW", Date: base, Close: 100, Volume: 1000},
		{Symbol: "2330.TW", Date: base.AddDate(0, 0, 1), Close: 101, Volume: 1100},
	}
	store.quotes["2330"] = []domain.DailyBar{
		{Symbol: "2330", Date: base, Close: 999, Volume: 9999},
	}
	store.quotes["2330.TWO"] = []domain.DailyBar{
		{Symbol: "2330.TWO", Date: base, Close: 888, Volume: 8888},
	}

	panel := &realPanel{quoteStore: store}
	bars, err := panel.Bars(ctx, "2330")
	if err != nil {
		t.Fatalf("Bars: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 bars from .TW, got %d", len(bars))
	}
	if bars[0].Close != 100 {
		t.Fatalf("expected .TW close 100, got %v", bars[0].Close)
	}

	// Only bare symbol exists -> fallback to bare.
	delete(store.quotes, "2330.TW")
	panel = &realPanel{quoteStore: store}
	bars, err = panel.Bars(ctx, "2330")
	if err != nil {
		t.Fatalf("Bars bare fallback: %v", err)
	}
	if len(bars) != 1 || bars[0].Close != 999 {
		t.Fatalf("expected bare fallback, got %+v", bars)
	}

	// Only .TWO exists -> fallback to .TWO.
	delete(store.quotes, "2330")
	panel = &realPanel{quoteStore: store}
	bars, err = panel.Bars(ctx, "2330")
	if err != nil {
		t.Fatalf("Bars .TWO fallback: %v", err)
	}
	if len(bars) != 1 || bars[0].Close != 888 {
		t.Fatalf("expected .TWO fallback, got %+v", bars)
	}
}

// TestBars_NoSuffixMatch returns empty when no variant exists.
func TestBars_NoSuffixMatch(t *testing.T) {
	ctx := context.Background()
	store := newFakeQuoteStore()
	panel := &realPanel{quoteStore: store}
	bars, err := panel.Bars(ctx, "9999")
	if err != nil {
		t.Fatalf("Bars: %v", err)
	}
	if len(bars) != 0 {
		t.Fatalf("expected empty bars, got %d", len(bars))
	}
}

// TestQuoteSymbols_SQLite tests the optional QuoteSymbolLister on SQLite.
func TestQuoteSymbols_SQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteQuoteStore(db)
	if err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "2330.TW", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Close: 100, Volume: 1000},
		{Symbol: "2317.TW", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Close: 200, Volume: 2000},
		{Symbol: "2330.TW", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Close: 101, Volume: 1100},
	}); err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}

	var qs ledger.QuoteStore = store
	lister, ok := qs.(ledger.QuoteSymbolLister)
	if !ok {
		t.Fatalf("SQLiteQuoteStore does not implement QuoteSymbolLister")
	}
	syms, err := lister.QuoteSymbols(context.Background())
	if err != nil {
		t.Fatalf("QuoteSymbols: %v", err)
	}
	want := map[string]bool{"2330.TW": true, "2317.TW": true}
	if len(syms) != len(want) {
		t.Fatalf("expected %d symbols, got %d: %v", len(want), len(syms), syms)
	}
	for _, s := range syms {
		if !want[s] {
			t.Fatalf("unexpected symbol %q", s)
		}
	}
}

// TestQuoteSymbols_FakeStoreWithoutLister confirms that fakeQuoteStore does
// not implement the optional lister.
func TestQuoteSymbols_FakeStoreWithoutLister(t *testing.T) {
	store := newFakeQuoteStore()
	if _, ok := asQuoteStore(store).(ledger.QuoteSymbolLister); ok {
		t.Fatalf("fakeQuoteStore should not implement QuoteSymbolLister")
	}
}

// TestNewRealPanel_UsesQuoteSymbolLister verifies that newRealPanel lists
// symbols from stores that implement QuoteSymbolLister and normalizes them.
func TestNewRealPanel_UsesQuoteSymbolLister(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteQuoteStore(db)
	if err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "2330.TW", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Close: 100, Volume: 1000},
		{Symbol: "2317.TWO", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Close: 200, Volume: 2000},
	}); err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}

	ctx := context.Background()
	panel, err := newRealPanel(ctx, store, t.TempDir())
	if err != nil {
		t.Fatalf("newRealPanel: %v", err)
	}
	rp := panel.(*realPanel)
	if len(rp.symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d: %v", len(rp.symbols), rp.symbols)
	}
	want := map[string]bool{"2330": true, "2317": true}
	for _, s := range rp.symbols {
		if !want[s] {
			t.Fatalf("unexpected normalized symbol %q", s)
		}
	}
}

// TestNewRealPanel_WithoutListerRequiresUniverse verifies that a panel built
// from a QuoteStore without QuoteSymbolLister has no symbols; the CLI will
// then require -universe.
func TestNewRealPanel_WithoutListerRequiresUniverse(t *testing.T) {
	store := newFakeQuoteStore()
	ctx := context.Background()
	panel, err := newRealPanel(ctx, store, t.TempDir())
	if err != nil {
		t.Fatalf("newRealPanel: %v", err)
	}
	rp := panel.(*realPanel)
	if len(rp.symbols) != 0 {
		t.Fatalf("expected 0 symbols for non-lister store, got %d", len(rp.symbols))
	}
}

// TestBars_PostgresBackend exercises the backend-aware Bars path against a
// real PostgreSQL quotes table when DATABASE_URL is available. It skips
// cleanly when postgres is unreachable, keeping CI green on sqlite-only runs.
func TestBars_PostgresBackend(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t, "../../sql/migrations")

	// Clean up test rows on both sides of the run.
	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM quotes WHERE symbol LIKE 'rspgtest-%'")
	}()
	_, _ = pool.Exec(ctx, "DELETE FROM quotes WHERE symbol LIKE 'rspgtest-%'")

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	store := ledger.NewPostgresQuoteStore(pool)
	if err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "rspgtest-2330.TW", Date: base, Close: 100, Volume: 1000},
		{Symbol: "rspgtest-2330.TW", Date: base.AddDate(0, 0, 1), Close: 101, Volume: 1100},
	}); err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}

	panel := &realPanel{quoteStore: store}
	bars, err := panel.Bars(ctx, "rspgtest-2330")
	if err != nil {
		t.Fatalf("Bars: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("expected 2 postgres bars, got %d", len(bars))
	}
	if bars[0].Close != 100 || bars[1].Close != 101 {
		t.Fatalf("unexpected closes: %+v", bars)
	}
}

// TestQuoteSymbols_Postgres exercises QuoteSymbols on a real PostgreSQL quotes
// table when DATABASE_URL is available.
func TestQuoteSymbols_Postgres(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t, "../../sql/migrations")

	defer func() {
		_, _ = pool.Exec(ctx, "DELETE FROM quotes WHERE symbol LIKE 'rspgtest-%'")
	}()
	_, _ = pool.Exec(ctx, "DELETE FROM quotes WHERE symbol LIKE 'rspgtest-%'")

	store := ledger.NewPostgresQuoteStore(pool)
	if err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "rspgtest-2330.TW", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Close: 100, Volume: 1000},
		{Symbol: "rspgtest-2317.TW", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Close: 200, Volume: 2000},
	}); err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}

	var qs ledger.QuoteStore = store
	lister, ok := qs.(ledger.QuoteSymbolLister)
	if !ok {
		t.Fatalf("PostgresQuoteStore does not implement QuoteSymbolLister")
	}
	syms, err := lister.QuoteSymbols(ctx)
	if err != nil {
		t.Fatalf("QuoteSymbols: %v", err)
	}
	want := map[string]bool{"rspgtest-2330.TW": true, "rspgtest-2317.TW": true}
	if len(syms) != len(want) {
		t.Fatalf("expected %d symbols, got %d: %v", len(want), len(syms), syms)
	}
	for _, s := range syms {
		if !want[s] {
			t.Fatalf("unexpected symbol %q", s)
		}
	}
}
