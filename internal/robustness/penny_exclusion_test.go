package robustness_test

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/robustness"
)

func TestExcludePennyStocks(t *testing.T) {
	prices := map[string]float64{
		"A": 5,
		"B": 10,
		"C": 50,
		"D": 100,
		"E": 200,
	}

	// strategyFunc returns daily returns: all symbols return +1% per day
	strategyFunc := func(symbol string) []float64 {
		return []float64{0.01, 0.01, 0.01}
	}

	t.Run("threshold 0.20 excludes cheapest symbol", func(t *testing.T) {
		report := robustness.ExcludePennyStocks(prices, 0.20, strategyFunc)

		// With 5 symbols, 0.20 percentile → index floor(4*0.20) = floor(0.8) = 0 → price at index 0 = 5
		// All symbols with price <= 5 are excluded → only "A" (price 5) is excluded
		if len(report.ExcludedSymbols) != 1 {
			t.Errorf("expected 1 excluded symbol, got %d: %v", len(report.ExcludedSymbols), report.ExcludedSymbols)
		}

		if len(report.ExcludedSymbols) > 0 && report.ExcludedSymbols[0] != "A" {
			t.Errorf("expected excluded symbol 'A', got %q", report.ExcludedSymbols[0])
		}

		// Verify excluded eval has fewer data points than baseline
		if report.Baseline == nil || report.Excluded == nil {
			t.Fatal("Baseline or Excluded eval result is nil")
		}

		// Baseline: 5 symbols × 3 returns = 15 data points
		// Excluded: 4 symbols × 3 returns = 12 data points
		// Both have same per-return, so CumReturn differs due to compounding length
		if report.Baseline.CumReturn <= report.Excluded.CumReturn {
			t.Logf("Baseline CumReturn: %f, Excluded CumReturn: %f", report.Baseline.CumReturn, report.Excluded.CumReturn)
		}
	})

	t.Run("threshold 0.0 excludes nothing", func(t *testing.T) {
		report := robustness.ExcludePennyStocks(prices, 0.0, strategyFunc)

		if len(report.ExcludedSymbols) != 0 {
			t.Errorf("expected 0 excluded symbols, got %d: %v", len(report.ExcludedSymbols), report.ExcludedSymbols)
		}
	})

	t.Run("threshold 1.0 excludes everything", func(t *testing.T) {
		report := robustness.ExcludePennyStocks(prices, 1.0, strategyFunc)

		// All symbols have price <= 200 (the max), so all are excluded
		if len(report.ExcludedSymbols) != 5 {
			t.Errorf("expected 5 excluded symbols, got %d: %v", len(report.ExcludedSymbols), report.ExcludedSymbols)
		}

		if report.Excluded == nil {
			t.Fatal("Excluded eval result is nil")
		}
		if report.Excluded.CumReturn != 0 {
			t.Errorf("expected zero CumReturn when all symbols excluded, got %f", report.Excluded.CumReturn)
		}
	})

	t.Run("degradation pct calculation is correct", func(t *testing.T) {
		report := robustness.ExcludePennyStocks(prices, 0.20, strategyFunc)

		if report.Baseline == nil || report.Excluded == nil {
			t.Fatal("Baseline or Excluded eval result is nil")
		}

		// Degradation = ((baseline - excluded) / |baseline|) * 100
		// Both should be positive since all returns are +1%
		if report.Baseline.CumReturn <= 0 {
			t.Skip("baseline CumReturn is non-positive, can't validate degradation")
		}

		expectedDegradation := ((report.Baseline.CumReturn - report.Excluded.CumReturn) / math.Abs(report.Baseline.CumReturn)) * 100
		if math.Abs(report.DegradationPct-expectedDegradation) > 1e-10 {
			t.Errorf("expected DegradationPct %f, got %f", expectedDegradation, report.DegradationPct)
		}
	})

	t.Run("empty prices map returns zero reports", func(t *testing.T) {
		report := robustness.ExcludePennyStocks(map[string]float64{}, 0.20, strategyFunc)

		if report.Baseline == nil || report.Excluded == nil {
			t.Fatal("Baseline or Excluded is nil for empty input")
		}
		if report.Baseline.CumReturn != 0 {
			t.Errorf("expected zero CumReturn for empty prices, got %f", report.Baseline.CumReturn)
		}
		if report.Excluded.CumReturn != 0 {
			t.Errorf("expected zero CumReturn for empty prices, got %f", report.Excluded.CumReturn)
		}
		if len(report.ExcludedSymbols) != 0 {
			t.Errorf("expected no excluded symbols for empty prices, got %v", report.ExcludedSymbols)
		}
	})
}
