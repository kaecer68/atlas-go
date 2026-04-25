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
}

func TestSharpeStabilityCheck_StableSeries(t *testing.T) {
	series := []float64{1.0, 1.1, 0.9, 1.05, 0.95}
	stable, stderr, err := SharpeStabilityCheck(series, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stable {
		t.Errorf("expected stable=true, got false; stderr=%v", stderr)
	}
}

func TestSharpeStabilityCheck_UnstableSeries(t *testing.T) {
	series := make([]float64, 20)
	for i := 0; i < 20; i++ {
		series[i] = float64(i%2) * 10.0
	}
	stable, stderr, err := SharpeStabilityCheck(series, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// stderr ≈ 1.124, which is NOT < 0.5 → unstable
	if stable {
		t.Errorf("expected stable=false, got true; stderr=%v", stderr)
	}
}

func TestSharpeStabilityCheck_BoundaryThreshold(t *testing.T) {
	// Nearly-constant series → very low stderr ≈ 0.00014, passes stderr < 0.01
	series := []float64{1.0, 1.0005, 0.9995, 1.0002, 0.9998}
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
