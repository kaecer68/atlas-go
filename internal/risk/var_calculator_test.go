package risk

import (
	"math"
	"sort"
	"testing"
)

func TestCalculateVaR(t *testing.T) {
	// 252 samples required for VaR calculation (1 year of daily data)
	returns := make([]float64, 252)
	for i := range returns {
		returns[i] = []float64{0.01, 0.02, -0.01, 0.015, -0.005, 0.008, -0.02, 0.012, -0.015, 0.005}[i%10]
	}

	var95 := CalculateVaR(returns, 0.95)
	if var95 >= 0 {
		t.Errorf("VaR95 should be negative (loss), got %f", var95)
	}

	var99 := CalculateVaR(returns, 0.99)
	if var99 > var95 {
		t.Errorf("VaR99 should be worse than VaR95, got VaR99=%f > VaR95=%f", var99, var95)
	}
}

func TestCalculateVaREmpty(t *testing.T) {
	result := CalculateVaR([]float64{}, 0.95)
	if result != 0.0 {
		t.Errorf("expected 0.0 for empty returns, got %f", result)
	}
}

func TestCalculateCVaR(t *testing.T) {
	// 252 samples required for CVaR calculation
	returns := make([]float64, 252)
	for i := range returns {
		returns[i] = []float64{0.01, 0.02, -0.01, 0.015, -0.005, 0.008, -0.02, 0.012, -0.015, 0.005}[i%10]
	}

	cvar95 := CalculateCVaR(returns, 0.95)
	var95 := CalculateVaR(returns, 0.95)

	if cvar95 > var95 {
		t.Errorf("CVaR95 should be worse than VaR95, got CVaR=%f > VaR=%f", cvar95, var95)
	}
}

func TestCalculateMaxDrawdown(t *testing.T) {
	values := []float64{100, 110, 105, 95, 100, 90, 85, 95, 100}
	mdd := CalculateMaxDrawdown(values)
	expected := (110.0 - 85.0) / 110.0
	if math.Abs(mdd-expected) > 0.001 {
		t.Errorf("expected max drawdown %.4f, got %.4f", expected, mdd)
	}
}

func TestCalculateMaxDrawdownNoDecline(t *testing.T) {
	values := []float64{100, 105, 110, 115, 120}
	mdd := CalculateMaxDrawdown(values)
	if mdd != 0.0 {
		t.Errorf("expected 0.0 drawdown for monotonic increase, got %f", mdd)
	}
}

func TestComputeRiskSnapshot(t *testing.T) {
	// 252 samples required for VaR/CVaR calculation
	returns := make([]float64, 252)
	for i := range returns {
		returns[i] = []float64{0.01, 0.02, -0.01, 0.015, -0.005, 0.008, -0.02, 0.012, -0.015, 0.005}[i%10]
	}
	values := []float64{100, 101, 103, 102, 104, 103, 104, 102, 103, 104, 105}

	snap := ComputeRiskSnapshot(returns, values)

	if snap.VaR95 >= 0 {
		t.Errorf("VaR95 should be negative, got %f", snap.VaR95)
	}
	if snap.VaR99 > snap.VaR95 {
		t.Errorf("VaR99 should be worse than VaR95")
	}
	if snap.CVaR95 > snap.VaR95 {
		t.Errorf("CVaR95 should be worse than VaR95")
	}
	if snap.MaxDrawdownPct < 0 {
		t.Errorf("MaxDrawdown should be non-negative")
	}
}

func TestCalculateComponentVaR(t *testing.T) {
	// 252 samples required per asset for ComponentVaR
	baseReturns := []float64{0.01, -0.02, 0.015, 0.005, -0.01, 0.02, -0.015, 0.008, -0.005, 0.012}
	a := make([]float64, 252)
	b := make([]float64, 252)
	c := make([]float64, 252)
	for i := range 252 {
		a[i] = baseReturns[i%10]
		b[i] = baseReturns[(i+3)%10]
		c[i] = baseReturns[(i+7)%10]
	}
	returns := map[string][]float64{
		"2330": a,
		"2454": b,
		"2317": c,
	}
	weights := map[string]float64{
		"2330": 0.5,
		"2454": 0.3,
		"2317": 0.2,
	}

	items := CalculateComponentVaR(returns, weights, 0.95)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	for _, item := range items {
		if item.Weight == 0 {
			t.Errorf("weight for %s should not be 0", item.Symbol)
		}
		if item.PctContribution < 0 || item.PctContribution > 1 {
			t.Errorf("pct_contribution for %s out of [0,1]: %f", item.Symbol, item.PctContribution)
		}
	}

	sumPct := 0.0
	for _, item := range items {
		sumPct += item.PctContribution
	}
	if math.Abs(sumPct-1.0) > 0.0001 {
		t.Errorf("pct_contribution should sum to 1.0, got %f", sumPct)
	}
}

func TestCalculateComponentVaR_Empty(t *testing.T) {
	items := CalculateComponentVaR(map[string][]float64{}, map[string]float64{}, 0.95)
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty input, got %d", len(items))
	}
}

func TestCalculateComponentVaR_EqualWeights(t *testing.T) {
	// 252 samples required per asset for ComponentVaR
	baseA := []float64{0.01, -0.01, 0.02, -0.02, 0.01, -0.01, 0.02, -0.02, 0.01, -0.01}
	baseB := []float64{0.02, -0.02, 0.01, -0.01, 0.02, -0.02, 0.01, -0.01, 0.02, -0.02}
	a := make([]float64, 252)
	b := make([]float64, 252)
	for i := range 252 {
		a[i] = baseA[i%10]
		b[i] = baseB[i%10]
	}
	returns := map[string][]float64{
		"A": a,
		"B": b,
	}
	weights := map[string]float64{
		"A": 0.5,
		"B": 0.5,
	}

	items := CalculateComponentVaR(returns, weights, 0.95)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if math.Abs(items[0].PctContribution+items[1].PctContribution-1.0) > 0.0001 {
		t.Errorf("pct_contribution should sum to 1.0, got %f + %f", items[0].PctContribution, items[1].PctContribution)
	}
}

func TestCalculateComponentVaR_ShortSeries(t *testing.T) {
	returns := map[string][]float64{
		"A": {0.01},
	}
	weights := map[string]float64{"A": 1.0}

	items := CalculateComponentVaR(returns, weights, 0.95)
	if len(items) != 0 {
		t.Errorf("expected 0 items for short series (<252), got %d", len(items))
	}
}

func TestVaRPercentileAccuracy(t *testing.T) {
	returns := make([]float64, 252)
	for i := range returns {
		returns[i] = float64(i-126) / 1000.0
	}
	sort.Float64s(returns)

	var95 := CalculateVaR(returns, 0.95)
	expectedIndex := int(math.Floor(0.05 * 252))
	expected := returns[expectedIndex]

	if math.Abs(var95-expected) > 0.0001 {
		t.Errorf("VaR95 percentile accuracy: expected %.4f, got %.4f", expected, var95)
	}
}

func TestCalculateComponentVaR_CustomConfidence(t *testing.T) {
	// Use unique returns so sorted[2] ≠ sorted[12] at 252 observations.
	a := make([]float64, MinObservationsForVaR)
	b := make([]float64, MinObservationsForVaR)
	for i := range MinObservationsForVaR {
		a[i] = float64(i-MinObservationsForVaR/2) / 1000.0
		b[i] = float64(MinObservationsForVaR/2-i) / 1000.0
	}
	returns := map[string][]float64{"A": a, "B": b}
	weights := map[string]float64{"A": 0.6, "B": 0.4}

	items95 := CalculateComponentVaR(returns, weights, 0.95)
	items99 := CalculateComponentVaR(returns, weights, 0.99)

	if len(items95) != 2 || len(items99) != 2 {
		t.Fatalf("expected 2 items each, got %d and %d", len(items95), len(items99))
	}

	for i := range items95 {
		if items95[i].ComponentVaR == items99[i].ComponentVaR {
			t.Errorf("ComponentVaR for %s should differ between 0.95 and 0.99 confidence", items95[i].Symbol)
		}
	}
}

func TestCalculateVaR_AtMinObservations(t *testing.T) {
	returns := make([]float64, MinObservationsForVaR)
	for i := range returns {
		returns[i] = float64(i-MinObservationsForVaR/2) / 1000.0
	}
	sort.Float64s(returns)

	var95 := CalculateVaR(returns, 0.95)
	expectedIndex := int(math.Floor((1.0 - 0.95) * float64(MinObservationsForVaR)))
	expected := returns[expectedIndex]

	if math.Abs(var95-expected) > 0.0001 {
		t.Errorf("VaR95 at min observations: expected %.4f (index %d), got %.4f",
			expected, expectedIndex, var95)
	}
}

func TestCalculateVaR_InsufficientObservations(t *testing.T) {
	returns := make([]float64, MinObservationsForVaR-1)
	for i := range returns {
		returns[i] = -0.01
	}

	result := CalculateVaR(returns, 0.95)
	if result != 0.0 {
		t.Errorf("expected 0.0 for insufficient observations (%d < %d), got %f",
			len(returns), MinObservationsForVaR, result)
	}
}
