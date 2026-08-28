//go:build integration

package stockpicker

// real_panel_integration_test.go — RealPanel tests against a real
// PostgreSQL quotes table when DATABASE_URL is available (skipped cleanly
// when postgres is unreachable, keeping CI green on sqlite-only runs).
// Moved from cmd/run-stockpicker-backtest/panel_test.go (PR 2e).

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// compile-time assertions (moved with the panel code from cmd)
var _ ledger.QuoteSymbolLister = (*ledger.SQLiteQuoteStore)(nil)
var _ ledger.QuoteSymbolLister = (*ledger.PostgresQuoteStore)(nil)

// TestBars_PostgresBackend exercises the backend-aware Bars path against a
// real PostgreSQL quotes table.
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

	panel := &RealPanel{quoteStore: store}
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

// TestQuoteSymbols_Postgres exercises QuoteSymbols on a real PostgreSQL
// quotes table.
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
