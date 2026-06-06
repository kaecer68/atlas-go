package experiment

import (
	"math"
	"testing"
)

func TestSharpeStabilityCheck_InsufficientData(t *testing.T) {
	_, _, err := SharpeStabilityCheck([]float64{}, 0.5)
	if err == nil {
		t.Error("expected error for empty series")
	}

	_, _, err = SharpeStabilityCheck([]float64{1.0}, 0.5)
	if err == nil {
		t.Error("expected error for single data point")
	}

	// 29 points should still fail (threshold is 30)
	short := make([]float64, 29)
	for i := range short {
		short[i] = 1.0 + float64(i)*0.01
	}
	_, _, err = SharpeStabilityCheck(short, 0.5)
	if err == nil {
		t.Error("expected error for 29 data points (<30 threshold)")
	}
}

func TestSharpeStabilityCheck_StableSeries(t *testing.T) {
	series := make([]float64, 30)
	for i := range series {
		series[i] = 1.0 + float64(i%5)*0.02
	}
	stable, stderr, err := SharpeStabilityCheck(series, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stable {
		t.Errorf("expected stable=true, got false; stderr=%v", stderr)
	}
}

func TestSharpeStabilityCheck_UnstableSeries(t *testing.T) {
	series := make([]float64, 30)
	for i := range 30 {
		series[i] = float64(i%2) * 10.0
	}
	stable, stderr, err := SharpeStabilityCheck(series, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// stderr ≈ 1.095, which is NOT < 0.5 → unstable
	if stable {
		t.Errorf("expected stable=false, got true; stderr=%v", stderr)
	}
}

func TestSharpeStabilityCheck_BoundaryThreshold(t *testing.T) {
	// Nearly-constant series (30 pts) → very low stderr, passes stderr < 0.01
	series := make([]float64, 30)
	for i := range series {
		series[i] = 1.0 + float64(i)*0.0001
	}
	_, stderrHigh, _ := SharpeStabilityCheck(series, 0.01)
	if stderrHigh >= 0.01 {
		t.Errorf("expected stderr < 0.01, got %v", stderrHigh)
	}

	// Same series with generous threshold → still stable
	_, stderrLow, _ := SharpeStabilityCheck(series, 10.0)
	if stderrLow >= 10.0 {
		t.Errorf("expected stderr < 10.0, got %v", stderrLow)
	}
	_ = math.Sqrt
}
