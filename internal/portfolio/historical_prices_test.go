package portfolio

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// createTestPrices builds a HistoricalPrices with simple ascending prices
// for a single symbol over n days, starting from baseDate.
func createTestPrices(symbol string, n int, basePrice float64, startDate time.Time) *HistoricalPrices {
	hp := NewHistoricalPrices()
	for i := range n {
		hp.prices[symbol] = append(hp.prices[symbol], pricePoint{
			Date:  startDate.AddDate(0, 0, i),
			Close: basePrice + float64(i),
		})
	}
	return hp
}

// TestAdjustForCorporateActions_WithReferencePrice verifies that when
// ReferencePrice is provided, it takes priority as the anchor price.
// Pre-event raw=100, ReferencePrice=95, post-event raw=100. factor=95/100=0.95.
func TestAdjustForCorporateActions_WithReferencePrice(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 10 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}
	actions := []domain.CorporateAction{
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5),
			CashDividend:   5.0,
			ReferencePrice: 95.0, // TWSE published reference price
		},
	}
	err := hp.AdjustForCorporateActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pts := hp.prices["2330"]
	for i, p := range pts {
		if i < 5 {
			if math.Abs(p.Close-95.0) > 1e-9 {
				t.Errorf("pre-event price[%d] = %f, want 95.0", i, p.Close)
			}
		} else {
			if math.Abs(p.Close-100.0) > 1e-9 {
				t.Errorf("post-event price[%d] = %f, want 100.0", i, p.Close)
			}
		}
	}
}

// TestAdjustForCorporateActions_StockDividend verifies stock dividend adjustment.
// Stock dividend 3 TWD/share: factor = (10-3)/10 = 0.70.
func TestAdjustForCorporateActions_StockDividend(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 10 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}
	actions := []domain.CorporateAction{
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5),
			StockDividend:  3.0,
			ReferencePrice: 0,
		},
	}
	err := hp.AdjustForCorporateActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pts := hp.prices["2330"]
	for i, p := range pts {
		if i < 5 {
			if math.Abs(p.Close-70.0) > 1e-9 {
				t.Errorf("pre-event price[%d] = %f, want 70.0", i, p.Close)
			}
		} else {
			if math.Abs(p.Close-100.0) > 1e-9 {
				t.Errorf("post-event price[%d] = %f, want 100.0", i, p.Close)
			}
		}
	}
}

// TestAdjustForCorporateActions_CapitalReduction verifies capital reduction adjustment.
// Capital reduction 0.10: factor = 1 - 0.10 = 0.90.
func TestAdjustForCorporateActions_CapitalReduction(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 10 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}
	actions := []domain.CorporateAction{
		{
			Symbol:                "2330",
			ExDate:                baseDate.AddDate(0, 0, 5),
			CapitalReductionRatio: 0.10,
			ReferencePrice:        0,
		},
	}
	err := hp.AdjustForCorporateActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pts := hp.prices["2330"]
	for i, p := range pts {
		if i < 5 {
			if math.Abs(p.Close-90.0) > 1e-9 {
				t.Errorf("pre-event price[%d] = %f, want 90.0", i, p.Close)
			}
		} else {
			if math.Abs(p.Close-100.0) > 1e-9 {
				t.Errorf("post-event price[%d] = %f, want 100.0", i, p.Close)
			}
		}
	}
}

// TestAdjustForCorporateActions_MultipleEvents verifies that two consecutive
// events apply cumulative factors. Cash div 5 (0.95) then capital reduction 0.10 (0.90):
// cumulative = 0.95 * 0.90 = 0.855 for pre-first-event, 0.90 for between.
func TestAdjustForCorporateActions_MultipleEvents(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 15 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}
	actions := []domain.CorporateAction{
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5),
			CashDividend:   5.0,
			ReferencePrice: 0,
		},
		{
			Symbol:                "2330",
			ExDate:                baseDate.AddDate(0, 0, 10),
			CapitalReductionRatio: 0.10,
			ReferencePrice:        0,
		},
	}
	err := hp.AdjustForCorporateActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pts := hp.prices["2330"]
	for i, p := range pts {
		switch {
		case i < 5:
			// Before first event: 100 * 0.95 * 0.90 = 85.5
			if math.Abs(p.Close-85.5) > 1e-9 {
				t.Errorf("pre-event-1 price[%d] = %f, want 85.5", i, p.Close)
			}
		case i >= 5 && i < 10:
			// Between events: 100 * 0.90 = 90.0
			if math.Abs(p.Close-90.0) > 1e-9 {
				t.Errorf("between-events price[%d] = %f, want 90.0", i, p.Close)
			}
		default:
			// After both: unchanged 100.0
			if math.Abs(p.Close-100.0) > 1e-9 {
				t.Errorf("post-event price[%d] = %f, want 100.0", i, p.Close)
			}
		}
	}
}

// TestAdjustForCorporateActions_Idempotent verifies that calling the method
// twice with the same actions produces identical results.
func TestAdjustForCorporateActions_Idempotent(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 10 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}
	actions := []domain.CorporateAction{
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5),
			CashDividend:   5.0,
			ReferencePrice: 0,
		},
	}
	// First call
	if err := hp.AdjustForCorporateActions(actions); err != nil {
		t.Fatalf("first call error: %v", err)
	}
	firstRun := make([]pricePoint, len(hp.prices["2330"]))
	copy(firstRun, hp.prices["2330"])
	// Second call
	if err := hp.AdjustForCorporateActions(actions); err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if !pricesEqual(hp.prices["2330"], firstRun) {
		t.Error("prices changed on second call — not idempotent")
	}
}

// TestAdjustForCorporateActions_UnknownSymbol verifies that actions for symbols
// not in the price map are silently ignored.
func TestAdjustForCorporateActions_UnknownSymbol(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 10 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}
	original := make([]pricePoint, len(hp.prices["2330"]))
	copy(original, hp.prices["2330"])
	actions := []domain.CorporateAction{
		{
			Symbol:         "9999", // not in prices map
			ExDate:         baseDate.AddDate(0, 0, 5),
			CashDividend:   5.0,
			ReferencePrice: 0,
		},
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5),
			CashDividend:   5.0,
			ReferencePrice: 0,
		},
	}
	err := hp.AdjustForCorporateActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2330 should be adjusted; unknown symbol silently ignored
	for i, p := range hp.prices["2330"] {
		if i < 5 {
			if math.Abs(p.Close-95.0) > 1e-9 {
				t.Errorf("2330 pre-event price[%d] = %f, want 95.0", i, p.Close)
			}
		}
	}
}

// TestAdjustForCorporateActions_InvalidPostEventPrice verifies error return
// when post-event price is zero (cannot compute factor).
func TestAdjustForCorporateActions_InvalidPostEventPrice(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 10 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 0.0, // all prices zero
		})
	}
	actions := []domain.CorporateAction{
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5),
			CashDividend:   5.0,
			ReferencePrice: 0,
		},
	}
	err := hp.AdjustForCorporateActions(actions)
	if err == nil {
		t.Fatal("expected error for zero post-event price, got nil")
	}
}

// TestActionEffects verifies that ActionEffects returns the correct list of
// applied adjustments after calling AdjustForCorporateActions.
func TestActionEffects(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 15 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}
	actions := []domain.CorporateAction{
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5),
			CashDividend:   5.0,
			ReferencePrice: 0,
		},
		{
			Symbol:                "2330",
			ExDate:                baseDate.AddDate(0, 0, 10),
			CapitalReductionRatio: 0.10,
			ReferencePrice:        0,
		},
	}
	err := hp.AdjustForCorporateActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	effects := hp.ActionEffects("2330")
	if len(effects) != 2 {
		t.Fatalf("expected 2 effects, got %d", len(effects))
	}
	if effects[0].Type != shared.AdjustCashDividend {
		t.Errorf("effect[0].Type = %v, want AdjustCashDividend", effects[0].Type)
	}
	if effects[0].ExDate != baseDate.AddDate(0, 0, 5) {
		t.Errorf("effect[0].ExDate = %v, want %v", effects[0].ExDate, baseDate.AddDate(0, 0, 5))
	}
	if effects[1].Type != shared.AdjustCapitalReduction {
		t.Errorf("effect[1].Type = %v, want AdjustCapitalReduction", effects[1].Type)
	}
	// Verify unknown symbol returns empty list
	emptyEffects := hp.ActionEffects("9999")
	if len(emptyEffects) != 0 {
		t.Errorf("expected 0 effects for unknown symbol, got %d", len(emptyEffects))
	}
}

func TestAdjustForCorporateActions_SameDayMultipleAdjustments(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	for i := range 10 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}
	actions := []domain.CorporateAction{
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5),
			CashDividend:   4.0,
			StockDividend:  1.0,
			ReferencePrice: 0,
		},
	}
	// factor = (100-4)/100 * (10-1)/10 = 0.96 * 0.90 = 0.864
	err := hp.AdjustForCorporateActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pts := hp.prices["2330"]
	for i, p := range pts {
		if i < 5 {
			if math.Abs(p.Close-86.4) > 1e-9 {
				t.Errorf("pre-event price[%d] = %f, want 86.4", i, p.Close)
			}
		} else {
			if math.Abs(p.Close-100.0) > 1e-9 {
				t.Errorf("post-event price[%d] = %f, want 100.0", i, p.Close)
			}
		}
	}
}

func pricesEqual(a, b []pricePoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Date.Equal(b[i].Date) {
			return false
		}
		if math.Abs(a[i].Close-b[i].Close) > 1e-9 {
			return false
		}
	}
	return true
}

// TestAdjustForCorporateActions_CashDividend_NoReferencePrice verifies that
// a cash dividend without ReferencePrice correctly backward-adjusts pre-event
// prices. Given raw price=100 and CashDividend=5, factor = (100-5)/100 = 0.95.
// Pre-event prices should be multiplied by 0.95; post-event prices unchanged.
func TestAdjustForCorporateActions_CashDividend_NoReferencePrice(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := NewHistoricalPrices()
	// 10 days of prices at 100.0
	for i := range 10 {
		hp.prices["2330"] = append(hp.prices["2330"], pricePoint{
			Date:  baseDate.AddDate(0, 0, i),
			Close: 100.0,
		})
	}

	actions := []domain.CorporateAction{
		{
			Symbol:         "2330",
			ExDate:         baseDate.AddDate(0, 0, 5), // day 5
			CashDividend:   5.0,
			ReferencePrice: 0, // no ReferencePrice available
		},
	}

	err := hp.AdjustForCorporateActions(actions)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pts := hp.prices["2330"]
	for i, p := range pts {
		if i < 5 {
			// Pre-event: should be 100 * 0.95 = 95.0
			if math.Abs(p.Close-95.0) > 1e-9 {
				t.Errorf("pre-event price[%d] = %f, want 95.0", i, p.Close)
			}
		} else {
			// Post-event: should remain 100.0
			if math.Abs(p.Close-100.0) > 1e-9 {
				t.Errorf("post-event price[%d] = %f, want 100.0", i, p.Close)
			}
		}
	}
}

// TestAdjustForCorporateActions_EmptyActions verifies that passing an empty
// actions slice is a no-op: prices remain unchanged and no error is returned.
func TestAdjustForCorporateActions_EmptyActions(t *testing.T) {
	baseDate := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp := createTestPrices("2330", 10, 100.0, baseDate)

	// Capture original prices
	original := make([]pricePoint, len(hp.prices["2330"]))
	copy(original, hp.prices["2330"])

	err := hp.AdjustForCorporateActions([]domain.CorporateAction{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pricesEqual(hp.prices["2330"], original) {
		t.Error("prices changed after empty actions — expected no-op")
	}
}
