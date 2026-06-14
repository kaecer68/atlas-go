package shared

import (
	"math"
	"testing"
)

func TestComputeSharpe_EmptyInput(t *testing.T) {
	got := ComputeSharpe([]float64{}, SharpeConfig{
		Frequency:    FrequencyPerDay,
		RiskFreeRate: 0.0,
		MinSamples:   2,
	})
	if got != 0 {
		t.Errorf("ComputeSharpe(empty) = %f, want 0", got)
	}
}

func TestComputeSharpe_MinSamplesGuard(t *testing.T) {
	got := ComputeSharpe([]float64{0.01}, SharpeConfig{
		Frequency:    FrequencyPerDay,
		RiskFreeRate: 0.0,
		MinSamples:   2,
	})
	if got != 0 {
		t.Errorf("ComputeSharpe(1 sample) = %f, want 0", got)
	}
}

func TestComputeSharpe_ZeroStdDev(t *testing.T) {
	got := ComputeSharpe([]float64{0.02, 0.02, 0.02}, SharpeConfig{
		Frequency:    FrequencyPerDay,
		RiskFreeRate: 0.0,
		MinSamples:   2,
	})
	if got != 0 {
		t.Errorf("ComputeSharpe(constant) = %f, want 0", got)
	}
}

func TestComputeSharpe_StdDevMeanRatioThreshold(t *testing.T) {
	cfg := SharpeConfig{
		Frequency:                FrequencyPerDay,
		RiskFreeRate:             0.0,
		MinSamples:               2,
		StdDevMeanRatioThreshold: 1e-3,
	}
	// Near-constant series with tiny stdDev relative to mean should be guarded.
	got := ComputeSharpe([]float64{0.020000001, 0.02, 0.019999999}, cfg)
	if got != 0 {
		t.Errorf("ComputeSharpe(near-constant) = %f, want 0", got)
	}
}

func TestComputeSharpe_PerDay(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	got := ComputeSharpe(returns, SharpeConfig{
		Frequency:    FrequencyPerDay,
		RiskFreeRate: 0.0,
		MinSamples:   2,
	})
	want := 0.4636 * math.Sqrt(252)
	if math.Abs(got-want) > 0.01 {
		t.Errorf("ComputeSharpe(per_day) = %f, want %f", got, want)
	}
}

func TestComputeSharpe_TWSEFrequency(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	perDay := ComputeSharpe(returns, SharpeConfig{
		Frequency:    FrequencyPerDay,
		RiskFreeRate: 0.0,
		MinSamples:   2,
	})
	twse := ComputeSharpe(returns, SharpeConfig{
		Frequency:    FrequencyTWSE,
		RiskFreeRate: 0.0,
		MinSamples:   2,
	})

	wantRatio := math.Sqrt(243) / math.Sqrt(252)
	gotRatio := twse / perDay
	if math.Abs(gotRatio-wantRatio) > 1e-9 {
		t.Errorf("TWSE/perDay ratio = %f, want %f", gotRatio, wantRatio)
	}
}

func TestComputeSharpe_PerOutcomeFrequency(t *testing.T) {
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	got := ComputeSharpe(returns, SharpeConfig{
		Frequency:    FrequencyPerOutcome,
		RiskFreeRate: 0.0,
		MinSamples:   2,
	})
	want := 0.4636
	if math.Abs(got-want) > 0.001 {
		t.Errorf("ComputeSharpe(per_outcome) = %f, want %f", got, want)
	}
}

func TestComputeSharpe_Negative(t *testing.T) {
	returns := []float64{-0.01, -0.02, -0.015}
	got := ComputeSharpe(returns, SharpeConfig{
		Frequency:    FrequencyPerOutcome,
		RiskFreeRate: 0.0,
		MinSamples:   2,
	})
	if got >= 0 {
		t.Errorf("ComputeSharpe(negative returns) = %f, want negative", got)
	}
}

func TestMeanSampleVariance_Empty(t *testing.T) {
	mean, stdDev := MeanSampleVariance([]float64{})
	if mean != 0 || stdDev != 0 {
		t.Errorf("MeanSampleVariance(empty) = (%f, %f), want (0, 0)", mean, stdDev)
	}
}

func TestMeanSampleVariance_SingleValue(t *testing.T) {
	mean, stdDev := MeanSampleVariance([]float64{5.0})
	if mean != 5.0 {
		t.Errorf("MeanSampleVariance(single) mean = %f, want 5.0", mean)
	}
	if stdDev != 0 {
		t.Errorf("MeanSampleVariance(single) stdDev = %f, want 0", stdDev)
	}
}

func TestMeanSampleVariance_Multiple(t *testing.T) {
	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	mean, stdDev := MeanSampleVariance(values)
	wantMean := 3.0
	if math.Abs(mean-wantMean) > 1e-9 {
		t.Errorf("MeanSampleVariance mean = %f, want %f", mean, wantMean)
	}
	// Sample stdDev of 1..5 = sqrt(2.5)
	wantStdDev := math.Sqrt(2.5)
	if math.Abs(stdDev-wantStdDev) > 1e-9 {
		t.Errorf("MeanSampleVariance stdDev = %f, want %f", stdDev, wantStdDev)
	}
}
