package risk

import (
	"math"
	"sort"
	"testing"
)

func TestCalculateVaR(t *testing.T) {
	returns := []float64{
		0.01, 0.02, -0.01, 0.015, -0.005,
		0.008, -0.02, 0.012, -0.015, 0.005,
		-0.008, 0.003, -0.012, 0.007, -0.003,
		0.004, -0.006, 0.009, -0.009, 0.011,
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
	returns := []float64{
		0.01, 0.02, -0.01, 0.015, -0.005,
		0.008, -0.02, 0.012, -0.015, 0.005,
		-0.008, 0.003, -0.012, 0.007, -0.003,
		0.004, -0.006, 0.009, -0.009, 0.011,
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
	returns := []float64{
		0.01, 0.02, -0.01, 0.015, -0.005,
		0.008, -0.02, 0.012, -0.015, 0.005,
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

func TestVaRPercentileAccuracy(t *testing.T) {
	returns := make([]float64, 100)
	for i := range returns {
		returns[i] = float64(i-50) / 1000.0
	}
	sort.Float64s(returns)

	var95 := CalculateVaR(returns, 0.95)
	expectedIndex := int(math.Floor(0.05 * 100))
	expected := returns[expectedIndex]

	if math.Abs(var95-expected) > 0.0001 {
		t.Errorf("VaR95 percentile accuracy: expected %.4f, got %.4f", expected, var95)
	}
}
