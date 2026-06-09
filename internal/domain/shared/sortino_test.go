package shared

import (
	"math"
	"testing"
)

func TestComputeSortino_EmptyInput(t *testing.T) {
	got := ComputeSortino([]float64{}, SortinoConfig{
		Frequency:  FrequencyPerDay,
		MinSamples: 2,
	})
	if got != 0 {
		t.Errorf("ComputeSortino(empty) = %f, want 0", got)
	}
}

func TestComputeSortino_NoDownside(t *testing.T) {
	// All returns positive → no downside observations → downside deviation is 0.
	returns := []float64{0.01, 0.02, 0.03}
	got := ComputeSortino(returns, SortinoConfig{
		Frequency:  FrequencyPerDay,
		MinSamples: 2,
	})
	if got != 0 {
		t.Errorf("ComputeSortino(no downside) = %f, want 0", got)
	}
}

func TestComputeSortino_MixedReturnsPerDay(t *testing.T) {
	// Mirrors the legacy TestCalculateSortinoRatio "normal" case to
	// guarantee byte-identical semantics for the canonical migration.
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	got := ComputeSortino(returns, SortinoConfig{
		Frequency:  FrequencyPerDay,
		MinSamples: 2,
	})
	want := 19.049
	if math.Abs(got-want) > 0.001 {
		t.Errorf("ComputeSortino(per_day) = %f, want %f", got, want)
	}
}

func TestComputeSortino_TWSEFrequency(t *testing.T) {
	// Same input as the per_day case, but annualization uses sqrt(243)
	// instead of sqrt(252). Expected ratio scales by sqrt(243)/sqrt(252).
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	perDay := ComputeSortino(returns, SortinoConfig{
		Frequency:  FrequencyPerDay,
		MinSamples: 2,
	})
	twse := ComputeSortino(returns, SortinoConfig{
		Frequency:  FrequencyTWSE,
		MinSamples: 2,
	})

	wantRatio := math.Sqrt(243) / math.Sqrt(252)
	gotRatio := twse / perDay
	if math.Abs(gotRatio-wantRatio) > 1e-9 {
		t.Errorf("TWSE/perDay ratio = %f, want %f", gotRatio, wantRatio)
	}

	// And the absolute TWSE value should match the scaled figure.
	want := perDay * wantRatio
	if math.Abs(twse-want) > 1e-9 {
		t.Errorf("ComputeSortino(twse) = %f, want %f", twse, want)
	}
}

func TestComputeSortino_MinSamplesGuard(t *testing.T) {
	// One sample is below MinSamples=2 → must return 0 even though there
	// is a downside observation.
	returns := []float64{-0.05}
	got := ComputeSortino(returns, SortinoConfig{
		Frequency:  FrequencyPerDay,
		MinSamples: 2,
	})
	if got != 0 {
		t.Errorf("ComputeSortino(1 sample) = %f, want 0", got)
	}
}

func TestComputeSortino_PerOutcomeFrequency(t *testing.T) {
	// PerOutcome applies no annualization — the raw ratio is returned.
	returns := []float64{0.01, -0.005, 0.02, -0.01, 0.015}
	got := ComputeSortino(returns, SortinoConfig{
		Frequency:  FrequencyPerOutcome,
		MinSamples: 2,
	})
	// Legacy raw ratio for this series ≈ 0.0796 (per_day 19.049 / sqrt(252))
	rawWant := 19.049 / math.Sqrt(252)
	if math.Abs(got-rawWant) > 0.001 {
		t.Errorf("ComputeSortino(per_outcome) = %f, want %f", got, rawWant)
	}
}
