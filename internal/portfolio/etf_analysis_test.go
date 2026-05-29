package portfolio

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestETFAnalyzer_RefreshNAV(t *testing.T) {
	ea := NewETFAnalyzer()
	ea.AddMetadata("0050.TW", ETFMetadata{Name: "元大台灣50", NAV: 0, ExpenseRatio: 0.0032, Benchmark: "TW50"})

	if !ea.RefreshNAV("0050.TW", 195.50) {
		t.Fatal("RefreshNAV returned false for existing symbol")
	}

	nav := ea.GetNAV("0050.TW")
	if nav != 195.50 {
		t.Fatalf("expected NAV=195.50, got %.2f", nav)
	}

	if ea.RefreshNAV("9999.TW", 100) {
		t.Fatal("RefreshNAV should return false for unknown symbol")
	}
}

func TestETFAnalyzer_SymbolsWithZeroNAV(t *testing.T) {
	ea := NewETFAnalyzer()
	ea.LoadMetadata(map[string]ETFMetadata{
		"0050.TW":  {Name: "元大台灣50", NAV: 195.50, ExpenseRatio: 0.0032, Benchmark: "TW50"},
		"0056.TW":  {Name: "元大高股息", NAV: 0, ExpenseRatio: 0.0043, Benchmark: "TWHDividend"},
		"00878.TW": {Name: "國泰永續高股息", NAV: 0, ExpenseRatio: 0.0045, Benchmark: "MSCITWESG"},
	})

	zeroSym := ea.SymbolsWithZeroNAV()
	if len(zeroSym) != 2 {
		t.Fatalf("expected 2 zero-NAV symbols, got %d: %v", len(zeroSym), zeroSym)
	}
}

func TestETFAnalyzer_UpdateNAVFromQuotes(t *testing.T) {
	ea := NewETFAnalyzer()
	ea.LoadMetadata(map[string]ETFMetadata{
		"0050.TW":  {Name: "元大台灣50", NAV: 0, ExpenseRatio: 0.0032, Benchmark: "TW50"},
		"0056.TW":  {Name: "元大高股息", NAV: 0, ExpenseRatio: 0.0043, Benchmark: "TWHDividend"},
		"00878.TW": {Name: "國泰永續高股息", NAV: 0, ExpenseRatio: 0.0045, Benchmark: "MSCITWESG"},
	})

	quotes := []domain.Quote{
		{Symbol: "0050.TW", Last: 195.50},
		{Symbol: "0056.TW", Last: 42.80},
		{Symbol: "00878.TW", Last: 25.30},
	}

	updated := ea.UpdateNAVFromQuotes(quotes)
	if updated != 3 {
		t.Fatalf("expected 3 updates, got %d", updated)
	}

	if ea.GetNAV("0050.TW") != 195.50 {
		t.Errorf("0050.TW NAV not updated: %.2f", ea.GetNAV("0050.TW"))
	}
	if ea.GetNAV("0056.TW") != 42.80 {
		t.Errorf("0056.TW NAV not updated: %.2f", ea.GetNAV("0056.TW"))
	}
	if ea.GetNAV("00878.TW") != 25.30 {
		t.Errorf("00878.TW NAV not updated: %.2f", ea.GetNAV("00878.TW"))
	}
}

func TestETFAnalyzer_UpdateNAVFromQuotes_SkipsZeroLast(t *testing.T) {
	ea := NewETFAnalyzer()
	ea.AddMetadata("0050.TW", ETFMetadata{Name: "元大台灣50", NAV: 0, ExpenseRatio: 0.0032, Benchmark: "TW50"})

	quotes := []domain.Quote{
		{Symbol: "0050.TW", Last: 0}, // zero last should be skipped
	}

	updated := ea.UpdateNAVFromQuotes(quotes)
	if updated != 0 {
		t.Fatalf("expected 0 updates (zero Last), got %d", updated)
	}
}

func TestETFAnalyzer_GetNAV_Unknown(t *testing.T) {
	ea := NewETFAnalyzer()
	if ea.GetNAV("9999.TW") != 0 {
		t.Fatal("expected 0 for unknown symbol")
	}
}

func TestETFAnalyzer_AllSymbols(t *testing.T) {
	ea := NewETFAnalyzer()
	ea.AddMetadata("0050.TW", ETFMetadata{Name: "元大台灣50", NAV: 195.50, ExpenseRatio: 0.0032, Benchmark: "TW50"})
	ea.AddMetadata("0056.TW", ETFMetadata{Name: "元大高股息", NAV: 42.80, ExpenseRatio: 0.0043, Benchmark: "TWHDividend"})

	symbols := ea.AllSymbols()
	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
}

type countingFetcher struct {
	count int
	navs  map[string]float64
}

func (f *countingFetcher) FetchNAV(_ context.Context, symbol string) (float64, error) {
	f.count++
	if nav, ok := f.navs[symbol]; ok {
		return nav, nil
	}
	return 0, nil
}

func TestETFAnalyzer_RefreshNAVFromFetcher(t *testing.T) {
	ea := NewETFAnalyzer()
	ea.LoadMetadata(map[string]ETFMetadata{
		"0050.TW":  {Name: "元大台灣50", NAV: 0, ExpenseRatio: 0.0032, Benchmark: "TW50"},
		"0056.TW":  {Name: "元大高股息", NAV: 42.80, ExpenseRatio: 0.0043, Benchmark: "TWHDividend"},
		"00878.TW": {Name: "國泰永續高股息", NAV: 0, ExpenseRatio: 0.0045, Benchmark: "MSCITWESG"},
	})

	f := &countingFetcher{
		navs: map[string]float64{
			"0050.TW":  195.50,
			"0056.TW":  42.80,
			"00878.TW": 25.30,
		},
	}

	ctx := context.Background()
	updated := ea.RefreshNAVFromFetcher(ctx, f, false)
	if updated != 2 {
		t.Fatalf("expected 2 updates (0056 has non-zero NAV), got %d", updated)
	}
	if f.count != 2 {
		t.Fatalf("fetcher called %d times, expected 2 (0056 skipped)", f.count)
	}

	if ea.GetNAV("0050.TW") != 195.50 {
		t.Errorf("0050.TW NAV: %.2f", ea.GetNAV("0050.TW"))
	}
	if ea.GetNAV("0056.TW") != 42.80 {
		t.Errorf("0056.TW NAV should be unchanged: %.2f", ea.GetNAV("0056.TW"))
	}
	if ea.GetNAV("00878.TW") != 25.30 {
		t.Errorf("00878.TW NAV: %.2f", ea.GetNAV("00878.TW"))
	}
}

func TestETFAnalyzer_RefreshNAVFromFetcher_Force(t *testing.T) {
	ea := NewETFAnalyzer()
	ea.AddMetadata("0050.TW", ETFMetadata{Name: "元大台灣50", NAV: 195.50, ExpenseRatio: 0.0032, Benchmark: "TW50"})

	f := &countingFetcher{
		navs: map[string]float64{"0050.TW": 197.00},
	}

	ctx := context.Background()
	updated := ea.RefreshNAVFromFetcher(ctx, f, true)
	if updated != 1 {
		t.Fatalf("expected 1 update with force=true, got %d", updated)
	}
	if ea.GetNAV("0050.TW") != 197.00 {
		t.Errorf("NAV not force-updated: %.2f", ea.GetNAV("0050.TW"))
	}
}
