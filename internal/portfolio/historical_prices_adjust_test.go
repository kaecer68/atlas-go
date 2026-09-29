package portfolio

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func adjDay(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return parsed
}

func hpWithSeries(t *testing.T, symbol string, dates []string, closes []float64) *HistoricalPrices {
	t.Helper()
	hp := NewHistoricalPrices()
	for i, d := range dates {
		hp.prices[symbol] = append(hp.prices[symbol], pricePoint{Date: adjDay(t, d), Close: closes[i]})
	}
	return hp
}

// TestStockDividendFactorUsesShareCountRatio pins the corrected stock-dividend
// factor. A "2 shares per 10 shares" stock dividend multiplies the share count
// by 1.2, so pre-event prices must be divided by 1.2 (10/(10+2) = 0.8333...).
// The previous implementation used (10-2)/10 = 0.8, which is 4% too small and
// only coincides with the correct value for a vanishing stock dividend.
func TestStockDividendFactorUsesShareCountRatio(t *testing.T) {
	hp := hpWithSeries(t, "2330.TW", []string{"2026-01-02", "2026-01-05", "2026-01-06"}, []float64{120, 118, 100})
	action := domain.CorporateAction{
		Symbol:        "2330.TW",
		ExDate:        adjDay(t, "2026-01-06"),
		StockDividend: 2,
	}
	if err := hp.AdjustForCorporateActions([]domain.CorporateAction{action}); err != nil {
		t.Fatalf("AdjustForCorporateActions: %v", err)
	}
	series := hp.GetCloseSeries("2330.TW")
	want := 120.0 * (10.0 / 12.0)
	if math.Abs(series[0]-want) > 1e-9 {
		t.Errorf("pre-event close = %v, want %v (old formula would give %v)", series[0], want, 120.0*0.8)
	}
	// The post-event bar keeps its raw value.
	if series[2] != 100 {
		t.Errorf("post-event close = %v, want unchanged 100", series[2])
	}
}

// TestStockDividendLargerThanTenPerTenDocuments6669 is the fingerprint pin for
// the real case that motivated the fix: 6669.TW paid a stock dividend of
// 19.827946 shares per 10 shares on 2026-09-02. The old formula produced
// (10-19.827946)/10 = -0.9828 and AdjustForCorporateActions rejected the action;
// the corrected formula yields 10/(10+19.827946) = 0.33528, and the official
// adjusted series (2614.997 official vs 7800 raw on 2026-09-01) implies
// 0.33526.
func TestStockDividendLargerThanTenPerTenDocuments6669(t *testing.T) {
	hp := hpWithSeries(t, "6669.TW", []string{"2026-09-01", "2026-09-02"}, []float64{7800, 2610})
	action := domain.CorporateAction{
		Symbol:        "6669.TW",
		ExDate:        adjDay(t, "2026-09-02"),
		StockDividend: 19.827946,
	}
	if err := hp.AdjustForCorporateActions([]domain.CorporateAction{action}); err != nil {
		t.Fatalf("AdjustForCorporateActions must accept the 6669 stock dividend, got: %v", err)
	}
	got := hp.GetCloseSeries("6669.TW")[0]
	wantFactor := 10.0 / (10.0 + 19.827946)
	if math.Abs(got-7800*wantFactor) > 1e-6 {
		t.Errorf("adjusted pre-event close = %v, want %v", got, 7800*wantFactor)
	}
	// The official series implies 2614.997 for that day; allow the published
	// rounding (0.2%).
	if math.Abs(got-2614.997)/2614.997 > 0.002 {
		t.Errorf("adjusted pre-event close = %v, official series says ~2614.997", got)
	}
}

func TestReferencePriceTakesPrecedenceOverFields(t *testing.T) {
	hp := hpWithSeries(t, "X.TW", []string{"2026-01-02", "2026-01-05"}, []float64{100, 50})
	action := domain.CorporateAction{
		Symbol:         "X.TW",
		ExDate:         adjDay(t, "2026-01-05"),
		CashDividend:   1,
		StockDividend:  5,
		ReferencePrice: 45, // factor = 45/50 = 0.9
	}
	if err := hp.AdjustForCorporateActions([]domain.CorporateAction{action}); err != nil {
		t.Fatalf("AdjustForCorporateActions: %v", err)
	}
	if got := hp.GetCloseSeries("X.TW")[0]; math.Abs(got-90) > 1e-9 {
		t.Errorf("pre-event close = %v, want 90 (raw 100 * 0.9)", got)
	}
}

func TestAdjustForCorporateActionsSkipsAndErrors(t *testing.T) {
	hp := hpWithSeries(t, "X.TW", []string{"2026-01-02", "2026-01-05"}, []float64{100, 50})
	// Unknown symbol: silently ignored.
	if err := hp.AdjustForCorporateActions([]domain.CorporateAction{{
		Symbol: "NOPE.TW", ExDate: adjDay(t, "2026-01-05"), ReferencePrice: 1,
	}}); err != nil {
		t.Errorf("unknown symbol must be ignored, got %v", err)
	}
	// Empty action list: no-op.
	if err := hp.AdjustForCorporateActions(nil); err != nil {
		t.Errorf("empty actions must be a no-op, got %v", err)
	}
	// Event on the first bar: nothing to adjust.
	if err := hp.AdjustForCorporateActions([]domain.CorporateAction{{
		Symbol: "X.TW", ExDate: adjDay(t, "2026-01-02"), ReferencePrice: 10,
	}}); err != nil {
		t.Errorf("event on the first bar must be a no-op, got %v", err)
	}
	// Non-positive post-event anchor: error, and the series stays untouched.
	bad := hpWithSeries(t, "Y.TW", []string{"2026-01-02", "2026-01-05"}, []float64{100, 0})
	if err := bad.AdjustForCorporateActions([]domain.CorporateAction{{
		Symbol: "Y.TW", ExDate: adjDay(t, "2026-01-05"), ReferencePrice: 90,
	}}); err == nil {
		t.Error("want an error when the post-event anchor is non-positive")
	}
}

func TestAdjustForCorporateActionsIsIdempotent(t *testing.T) {
	dates := []string{"2026-01-02", "2026-01-05", "2026-01-06"}
	closes := []float64{120, 118, 100}
	actions := []domain.CorporateAction{{
		Symbol: "2330.TW", ExDate: adjDay(t, "2026-01-06"), StockDividend: 2,
	}}
	once := hpWithSeries(t, "2330.TW", dates, closes)
	twice := hpWithSeries(t, "2330.TW", dates, closes)
	if err := once.AdjustForCorporateActions(actions); err != nil {
		t.Fatal(err)
	}
	if err := twice.AdjustForCorporateActions(actions); err != nil {
		t.Fatal(err)
	}
	if err := twice.AdjustForCorporateActions(actions); err != nil {
		t.Fatal(err)
	}
	a, b := once.GetCloseSeries("2330.TW"), twice.GetCloseSeries("2330.TW")
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-12 {
			t.Fatalf("apply twice changed the series: %v vs %v", a, b)
		}
	}
	if len(once.ActionEffects("2330.TW")) != 1 {
		t.Errorf("want 1 recorded effect, got %d", len(once.ActionEffects("2330.TW")))
	}
	if len(twice.ActionEffects("2330.TW")) != 1 {
		t.Errorf("an identical re-application must be skipped, so exactly one effect is recorded; got %d", len(twice.ActionEffects("2330.TW")))
	}
}

func TestCloseSeriesPreservesDates(t *testing.T) {
	hp := hpWithSeries(t, "2330.TW", []string{"2026-01-02", "2026-01-05"}, []float64{1, 2})
	got := hp.CloseSeries("2330.TW")
	if len(got) != 2 {
		t.Fatalf("want 2 points, got %d", len(got))
	}
	if !got[0].Date.Equal(adjDay(t, "2026-01-02")) || got[0].Close != 1 {
		t.Errorf("first point = %+v", got[0])
	}
	if hp.CloseSeries("MISSING.TW") != nil {
		t.Error("want nil for an unknown symbol")
	}
}
