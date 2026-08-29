package backtest

import (
	"math"
	"testing"
)

// makeFactorReturns generates deterministic factor return data for n trading
// days using a simple linear congruential scheme. Returns all six canonical
// factors: Mkt, SMB, HML, RMW, CMA, MOM.
func makeFactorReturns(n int) map[string][]float64 {
	// Deterministic pseudo-random generator (LCG).
	// seed and constants chosen to produce realistic daily factor values
	// (roughly in the range [-0.03, +0.03]).
	seed := uint64(42)
	a := uint64(6364136223846793005)
	c := uint64(1442695040888963407)
	next := func() float64 {
		seed = a*seed + c
		// Map to [-0.03, +0.03]
		return (float64(seed%10000)/10000.0 - 0.5) * 0.06
	}

	factors := []string{"Mkt", "SMB", "HML", "RMW", "CMA", "MOM"}
	result := make(map[string][]float64, len(factors))
	for _, f := range factors {
		arr := make([]float64, n)
		for i := range arr {
			arr[i] = next()
		}
		result[f] = arr
	}
	return result
}

func TestComputeFF5Alpha_PureAlpha(t *testing.T) {
	// Portfolio = alpha + noise, no factor exposure.
	// Daily alpha = 0.001 → annualised = 0.252.
	const n = 252
	const dailyAlpha = 0.001
	const noiseStd = 0.01

	fr := makeFactorReturns(n)
	y := make([]float64, n)

	// Deterministic noise.
	seed := uint64(99)
	a := uint64(6364136223846793005)
	c := uint64(1442695040888963407)
	for i := range y {
		seed = a*seed + c
		noise := (float64(seed%10000)/10000.0 - 0.5) * 2.0 * noiseStd
		y[i] = dailyAlpha + noise
	}

	result, err := ComputeFF5Alpha(y, fr)
	if err != nil {
		t.Fatalf("ComputeFF5Alpha failed: %v", err)
	}

	// Alpha should be close to 0.001 * 252 = 0.252.
	expectedAlpha := dailyAlpha * 252
	if math.Abs(result.Alpha-expectedAlpha) > 0.15 {
		t.Errorf("Alpha = %.4f, expected ≈ %.4f (tolerance 0.15)", result.Alpha, expectedAlpha)
	}

	// R² should be low because there is no factor signal.
	if result.R2 > 0.2 {
		t.Errorf("R² = %.4f, expected < 0.2 for pure-alpha portfolio", result.R2)
	}

	// All factor exposures should be near zero.
	for _, fk := range []string{"Mkt", "SMB", "HML", "RMW", "CMA", "MOM"} {
		if math.Abs(result.Exposures[fk]) > 0.3 {
			t.Errorf("Exposures[%s] = %.4f, expected ≈ 0 (tolerance 0.3)", fk, result.Exposures[fk])
		}
	}

	// N should match input.
	if result.N != n {
		t.Errorf("N = %d, expected %d", result.N, n)
	}
}

func TestComputeFF5Alpha_StrongMktExposure(t *testing.T) {
	// Portfolio = 1.0 × Mkt + noise.
	const n = 252
	const noiseStd = 0.005

	fr := makeFactorReturns(n)
	y := make([]float64, n)

	seed := uint64(77)
	a := uint64(6364136223846793005)
	c := uint64(1442695040888963407)
	for i := range y {
		seed = a*seed + c
		noise := (float64(seed%10000)/10000.0 - 0.5) * 2.0 * noiseStd
		y[i] = 1.0*fr["Mkt"][i] + noise
	}

	result, err := ComputeFF5Alpha(y, fr)
	if err != nil {
		t.Fatalf("ComputeFF5Alpha failed: %v", err)
	}

	// Mkt exposure should be near 1.0.
	if math.Abs(result.Exposures["Mkt"]-1.0) > 0.2 {
		t.Errorf("Exposures[Mkt] = %.4f, expected ≈ 1.0 (tolerance 0.2)", result.Exposures["Mkt"])
	}

	// Alpha should be near 0.
	if math.Abs(result.Alpha) > 0.1 {
		t.Errorf("Alpha = %.4f, expected ≈ 0 (tolerance 0.1)", result.Alpha)
	}

	// R² should be high (Mkt explains most variance).
	if result.R2 < 0.5 {
		t.Errorf("R² = %.4f, expected > 0.5 for strong factor exposure", result.R2)
	}
}

func TestComputeFF5Alpha_MixedExposures(t *testing.T) {
	// Portfolio = 0.5×Mkt + 0.3×HML - 0.2×SMB + noise.
	const n = 252
	const noiseStd = 0.005

	fr := makeFactorReturns(n)
	y := make([]float64, n)

	seed := uint64(123)
	a := uint64(6364136223846793005)
	c := uint64(1442695040888963407)
	for i := range y {
		seed = a*seed + c
		noise := (float64(seed%10000)/10000.0 - 0.5) * 2.0 * noiseStd
		y[i] = 0.5*fr["Mkt"][i] + 0.3*fr["HML"][i] - 0.2*fr["SMB"][i] + noise
	}

	result, err := ComputeFF5Alpha(y, fr)
	if err != nil {
		t.Fatalf("ComputeFF5Alpha failed: %v", err)
	}

	// Check each exposure within tolerance.
	expected := map[string]float64{
		"Mkt": 0.5,
		"HML": 0.3,
		"SMB": -0.2,
	}
	for fk, want := range expected {
		got := result.Exposures[fk]
		if math.Abs(got-want) > 0.25 {
			t.Errorf("Exposures[%s] = %.4f, expected ≈ %.4f (tolerance 0.25)", fk, got, want)
		}
	}

	// Alpha should be near 0.
	if math.Abs(result.Alpha) > 0.1 {
		t.Errorf("Alpha = %.4f, expected ≈ 0 (tolerance 0.1)", result.Alpha)
	}
}

func TestComputeFF5Alpha_EmptyReturns(t *testing.T) {
	fr := makeFactorReturns(10)
	_, err := ComputeFF5Alpha([]float64{}, fr)
	if err == nil {
		t.Fatal("expected error for empty portfolio returns, got nil")
	}
	if err.Error() != "empty portfolio returns" {
		t.Errorf("error = %q, want %q", err.Error(), "empty portfolio returns")
	}
}

func TestComputeFF5Alpha_NoFactors(t *testing.T) {
	_, err := ComputeFF5Alpha([]float64{0.01, 0.02, 0.03}, nil)
	if err == nil {
		t.Fatal("expected error for nil factor data, got nil")
	}
	if err.Error() != "no factor data provided" {
		t.Errorf("error = %q, want %q", err.Error(), "no factor data provided")
	}

	// Also test empty map.
	_, err = ComputeFF5Alpha([]float64{0.01, 0.02, 0.03}, map[string][]float64{})
	if err == nil {
		t.Fatal("expected error for empty factor map, got nil")
	}
}

func TestComputeFF5Alpha_LengthMismatch(t *testing.T) {
	fr := map[string][]float64{
		"Mkt": make([]float64, 15),
		"SMB": make([]float64, 15),
	}
	for i := range fr["Mkt"] {
		fr["Mkt"][i] = 0.01
		fr["SMB"][i] = 0.01
	}
	y := make([]float64, 12) // 12 != 15, both >= 10 so it passes the insufficient check

	_, err := ComputeFF5Alpha(y, fr)
	if err == nil {
		t.Fatal("expected error for length mismatch, got nil")
	}
	// Error should mention the mismatch details.
	if !contains(err.Error(), "length mismatch") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "length mismatch")
	}
}

func TestComputeFF5Alpha_InsufficientData(t *testing.T) {
	fr := map[string][]float64{
		"Mkt": {0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08, 0.09},
	}
	y := []float64{0.01, 0.02, 0.03, 0.04, 0.05, 0.06, 0.07, 0.08, 0.09} // 9 points

	_, err := ComputeFF5Alpha(y, fr)
	if err == nil {
		t.Fatal("expected error for insufficient observations, got nil")
	}
	if !contains(err.Error(), "insufficient observations") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "insufficient observations")
	}
}

func TestComputeFF5Alpha_MissingFactors(t *testing.T) {
	// Only Mkt and SMB available; other factors are missing.
	// Should still work with available factors only.
	const n = 100
	fr := map[string][]float64{
		"Mkt": make([]float64, n),
		"SMB": make([]float64, n),
	}

	// Fill with deterministic data.
	seed := uint64(55)
	a := uint64(6364136223846793005)
	c := uint64(1442695040888963407)
	for i := range n {
		seed = a*seed + c
		fr["Mkt"][i] = (float64(seed%10000)/10000.0 - 0.5) * 0.06
		seed = a*seed + c
		fr["SMB"][i] = (float64(seed%10000)/10000.0 - 0.5) * 0.06
	}

	// Portfolio = 0.8 × Mkt + noise.
	y := make([]float64, n)
	seed = uint64(66)
	for i := range y {
		seed = a*seed + c
		noise := (float64(seed%10000)/10000.0 - 0.5) * 2.0 * 0.005
		y[i] = 0.8*fr["Mkt"][i] + noise
	}

	result, err := ComputeFF5Alpha(y, fr)
	if err != nil {
		t.Fatalf("ComputeFF5Alpha with partial factors failed: %v", err)
	}

	// Only Mkt and SMB should be in exposures.
	if len(result.Exposures) != 2 {
		t.Errorf("expected 2 exposures, got %d", len(result.Exposures))
	}
	if _, ok := result.Exposures["Mkt"]; !ok {
		t.Error("expected Mkt in exposures")
	}
	if _, ok := result.Exposures["SMB"]; !ok {
		t.Error("expected SMB in exposures")
	}

	// Mkt exposure should be near 0.8.
	if math.Abs(result.Exposures["Mkt"]-0.8) > 0.2 {
		t.Errorf("Exposures[Mkt] = %.4f, expected ≈ 0.8", result.Exposures["Mkt"])
	}

	// Verify the result struct fields are populated.
	if result.N != n {
		t.Errorf("N = %d, expected %d", result.N, n)
	}
	if result.AlphaTStat == 0 && result.Alpha != 0 {
		t.Error("AlphaTStat should be computed even if near zero alpha")
	}
	if result.AnnualizedVol <= 0 {
		t.Errorf("AnnualizedVol = %.4f, expected > 0", result.AnnualizedVol)
	}
}

// contains checks whether substr appears in s.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
