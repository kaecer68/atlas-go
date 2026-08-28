package stockpicker

// real_panel_test.go — tests for the RealPanel production PanelSource
// (moved from cmd/run-stockpicker-backtest/panel_test.go, PR 2e). The
// postgres integration tests live in real_panel_integration_test.go.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

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

	panel := &RealPanel{quoteStore: store}
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
	panel = &RealPanel{quoteStore: store}
	bars, err = panel.Bars(ctx, "2330")
	if err != nil {
		t.Fatalf("Bars bare fallback: %v", err)
	}
	if len(bars) != 1 || bars[0].Close != 999 {
		t.Fatalf("expected bare fallback, got %+v", bars)
	}

	// Only .TWO exists -> fallback to .TWO.
	delete(store.quotes, "2330")
	panel = &RealPanel{quoteStore: store}
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
	panel := &RealPanel{quoteStore: store}
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

// TestNewRealPanel_UsesQuoteSymbolLister verifies that NewRealPanel lists
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
	panel, err := NewRealPanel(ctx, store, t.TempDir())
	if err != nil {
		t.Fatalf("NewRealPanel: %v", err)
	}
	if len(panel.symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d: %v", len(panel.symbols), panel.symbols)
	}
	want := map[string]bool{"2330": true, "2317": true}
	for _, s := range panel.symbols {
		if !want[s] {
			t.Fatalf("unexpected normalized symbol %q", s)
		}
	}
}

// TestNewRealPanel_WithoutListerRequiresUniverse verifies that a panel built
// from a QuoteStore without QuoteSymbolLister has no symbols; the caller
// must then pass an explicit universe.
func TestNewRealPanel_WithoutListerRequiresUniverse(t *testing.T) {
	store := newFakeQuoteStore()
	ctx := context.Background()
	panel, err := NewRealPanel(ctx, store, t.TempDir())
	if err != nil {
		t.Fatalf("NewRealPanel: %v", err)
	}
	if len(panel.symbols) != 0 {
		t.Fatalf("expected 0 symbols for non-lister store, got %d", len(panel.symbols))
	}
}

// TestRealPanel_FlowsFromFile pins the per-symbol flow JSON parsing used by
// the production panel (foreign_net must unmarshal into FlowPoint.ForeignNet).
func TestRealPanel_FlowsFromFile(t *testing.T) {
	dir := t.TempDir()
	flowsDir := filepath.Join(dir, "stock_flows")
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := `{"symbol":"2330","flows":[{"date":"2026-01-05","foreign_net":1500},{"date":"2026-01-06","foreign_net":1200}]}`
	if err := os.WriteFile(filepath.Join(flowsDir, "2330.json"), []byte(file), 0o644); err != nil {
		t.Fatalf("write flows: %v", err)
	}
	panel := &RealPanel{flowsDir: flowsDir}
	flows, err := panel.Flows(context.Background(), "2330")
	if err != nil {
		t.Fatalf("Flows: %v", err)
	}
	if len(flows) != 2 {
		t.Fatalf("len(flows) = %d, want 2", len(flows))
	}
	if flows[0].ForeignNet != 1500 || flows[0].Date != "2026-01-05" {
		t.Fatalf("flow[0] = %+v, want foreign_net 1500 on 2026-01-05", flows[0])
	}
	// Missing symbol file → empty (no error), the flow condition stays silent.
	missing, err := panel.Flows(context.Background(), "9999")
	if err != nil || len(missing) != 0 {
		t.Fatalf("missing symbol: err=%v flows=%v, want empty nil", err, missing)
	}
}
