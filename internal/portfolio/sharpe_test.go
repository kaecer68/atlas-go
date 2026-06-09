package portfolio

import (
	"math"
	"testing"
)

// TestComputeSharpe_PerOutcome_NoAnnualization verifies per-outcome mode
// returns mean/stdDev WITHOUT annualization. This matches the original
// internal/ledger.sharpeRatio behavior to preserve the existing scorecard
// values that the test fixtures rely on.
func TestComputeSharpe_PerOutcome_NoAnnualization(t *testing.T) {
	returns := []float64{0.01, 0.02, -0.01, 0.03, -0.02, 0.01, 0.04, -0.01, 0.02, 0.01}
	cfg := SharpeConfig{Frequency: FrequencyPerOutcome, MinSamples: 2}
	got := ComputeSharpe(returns, cfg)
	mean, stdDev := meanSampleVariance(returns)
	want := mean / stdDev
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("per-outcome Sharpe = %v, want %v (no annualization)", got, want)
	}
}

// TestComputeSharpe_PerDay_Annualized verifies per-day mode returns
// mean/stdDev * sqrt(252). This matches the original
// internal/portfolio.calculateSharpe behavior used by Darwinian.
func TestComputeSharpe_PerDay_Annualized(t *testing.T) {
	returns := make([]float64, 60)
	for i := range returns {
		returns[i] = 0.001 * float64(i%5-2)
	}
	cfg := SharpeConfig{Frequency: FrequencyPerDay, MinSamples: 60}
	got := ComputeSharpe(returns, cfg)
	mean, stdDev := meanSampleVariance(returns)
	want := (mean / stdDev) * math.Sqrt(252)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("per-day Sharpe = %v, want %v (annualized)", got, want)
	}
}

// TestComputeSharpe_InsufficientSamples returns 0 when below MinSamples.
func TestComputeSharpe_InsufficientSamples(t *testing.T) {
	cfg := SharpeConfig{Frequency: FrequencyPerDay, MinSamples: 60}
	got := ComputeSharpe([]float64{0.01, 0.02, 0.03}, cfg)
	if got != 0 {
		t.Fatalf("insufficient samples Sharpe = %v, want 0", got)
	}
}

// TestComputeSharpe_ZeroStdDev returns 0 to avoid division-by-zero and
// IEEE 754 precision edge cases. The IEEE guard must be active (threshold>0).
func TestComputeSharpe_ZeroStdDev(t *testing.T) {
	returns := make([]float64, 60)
	for i := range returns {
		returns[i] = 0.02
	}
	cfg := SharpeConfig{Frequency: FrequencyPerDay, MinSamples: 60, StdDevMeanRatioThreshold: 0.001}
	got := ComputeSharpe(returns, cfg)
	if got != 0 {
		t.Fatalf("zero-variance Sharpe = %v, want 0 (with IEEE guard)", got)
	}
}

// TestComputeSharpe_NegativeSharpe verifies negative values are returned
// (below risk-free rate is meaningful info, not an error).
func TestComputeSharpe_NegativeSharpe(t *testing.T) {
	returns := []float64{-0.05, -0.03, -0.04, -0.02, -0.06, -0.01, -0.04, -0.05}
	cfg := SharpeConfig{Frequency: FrequencyPerDay, MinSamples: 2}
	got := ComputeSharpe(returns, cfg)
	if got >= 0 {
		t.Fatalf("expected negative Sharpe for consistently negative returns, got %v", got)
	}
}

// TestComputeSharpe_EmptyReturns returns 0 (no data = no signal).
func TestComputeSharpe_EmptyReturns(t *testing.T) {
	cfg := SharpeConfig{Frequency: FrequencyPerDay, MinSamples: 60}
	if got := ComputeSharpe(nil, cfg); got != 0 {
		t.Fatalf("empty returns Sharpe = %v, want 0", got)
	}
	if got := ComputeSharpe([]float64{}, cfg); got != 0 {
		t.Fatalf("empty slice Sharpe = %v, want 0", got)
	}
}

// TestComputeSharpe_IEEEGuard: identical non-zero values with finite mean
// must return 0 (avoids IEEE 754 precision trap in old darwinian code).
func TestComputeSharpe_IEEEGuard(t *testing.T) {
	returns := make([]float64, 60)
	for i := range returns {
		returns[i] = 0.02
	}
	cfg := SharpeConfig{Frequency: FrequencyPerDay, MinSamples: 60, StdDevMeanRatioThreshold: 0.001}
	got := ComputeSharpe(returns, cfg)
	if got != 0 {
		t.Fatalf("identical-values Sharpe = %v, want 0 (IEEE guard)", got)
	}
}

// TestComputeSharpeWithRiskFreeRate verifies that a non-zero RiskFreeRate
// reduces Sharpe and that rfr=0 preserves existing behavior.
func TestComputeSharpeWithRiskFreeRate(t *testing.T) {
	returns := []float64{0.01, 0.02, -0.01, 0.015, 0.005}
	cfg := SharpeConfig{Frequency: FrequencyPerDay, MinSamples: 2}
	sharpeZeroRFR := ComputeSharpe(returns, cfg)
	cfg.RiskFreeRate = 0.005
	sharpeWithRFR := ComputeSharpe(returns, cfg)
	if sharpeWithRFR >= sharpeZeroRFR {
		t.Fatalf("Sharpe with rfr=0.005 (%f) should be < Sharpe with rfr=0.0 (%f)", sharpeWithRFR, sharpeZeroRFR)
	}
}

// TestComputeSharpeTWSEFrequency verifies that FrequencyTWSE uses sqrt(243)
// instead of sqrt(252), producing a measurably smaller ratio.
func TestComputeSharpeTWSEFrequency(t *testing.T) {
	returns := make([]float64, 60)
	for i := range returns {
		returns[i] = 0.001 * float64(i%5-1) // positive bias: -1, 0, 1, 2, 3 pattern
	}
	cfgDay := SharpeConfig{Frequency: FrequencyPerDay, MinSamples: 2}
	cfgTWSE := SharpeConfig{Frequency: FrequencyTWSE, MinSamples: 2}
	daySharpe := ComputeSharpe(returns, cfgDay)
	twseSharpe := ComputeSharpe(returns, cfgTWSE)
	if twseSharpe >= daySharpe {
		t.Fatalf("TWSE Sharpe (%f) should be < PerDay Sharpe (%f) since sqrt(243) < sqrt(252)", twseSharpe, daySharpe)
	}
	ratio := twseSharpe / daySharpe
	expectedRatio := math.Sqrt(243) / math.Sqrt(252)
	if math.Abs(ratio-expectedRatio) > 1e-9 {
		t.Fatalf("TWSE/PerDay ratio = %f, want %f (sqrt(243)/sqrt(252))", ratio, expectedRatio)
	}
}

// TestMeanSampleVariance verifies sample stddev (N-1 denominator).
// This is the convention both old implementations used.
func TestMeanSampleVariance(t *testing.T) {
	v := []float64{1, 2, 3, 4, 5}
	m, sd := meanSampleVariance(v)
	if math.Abs(m-3.0) > 1e-9 {
		t.Fatalf("mean = %v, want 3.0", m)
	}
	want := math.Sqrt(10.0 / 4.0)
	if math.Abs(sd-want) > 1e-9 {
		t.Fatalf("sample stddev = %v, want %v", sd, want)
	}
}
