package main

import (
	"math"
	"testing"
)

func TestPercentile_Empty(t *testing.T) {
	if got := percentile(nil, 0.5); got != 0 {
		t.Fatalf("expected 0 for empty, got %f", got)
	}
}

func TestPercentile_Boundaries(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	if got := percentile(sorted, 0); got != 1 {
		t.Fatalf("p=0 should be first element, got %f", got)
	}
	if got := percentile(sorted, 1); got != 5 {
		t.Fatalf("p=1 should be last element, got %f", got)
	}
}

func TestPercentile_Median(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	if got := percentile(sorted, 0.5); got != 3 {
		t.Fatalf("p=0.5 of [1..5] should be 3, got %f", got)
	}
}

func TestPercentile_Interpolation(t *testing.T) {
	sorted := []float64{0, 100}
	if got := percentile(sorted, 0.25); got != 25 {
		t.Fatalf("p=0.25 of [0,100] should be 25, got %f", got)
	}
}

func TestMean_Empty(t *testing.T) {
	if got := mean(nil); got != 0 {
		t.Fatalf("expected 0 for empty, got %f", got)
	}
}

func TestMean_Values(t *testing.T) {
	if got := mean([]float64{1, 2, 3, 4, 5}); got != 3 {
		t.Fatalf("expected mean 3, got %f", got)
	}
	if got := mean([]float64{-2, 2}); got != 0 {
		t.Fatalf("expected mean 0, got %f", got)
	}
}

func TestStddev_BelowTwoSamples(t *testing.T) {
	if got := stddev(nil); got != 0 {
		t.Fatalf("expected 0 for empty, got %f", got)
	}
	if got := stddev([]float64{5}); got != 0 {
		t.Fatalf("expected 0 for single sample, got %f", got)
	}
}

func TestStddev_PopulationFormula(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5}
	expected := math.Sqrt(2)
	if got := stddev(vals); math.Abs(got-expected) > 1e-9 {
		t.Fatalf("expected sqrt(2)=%f (population stddev), got %f", expected, got)
	}
}

func TestComputeDistribution_Empty(t *testing.T) {
	stats := computeDistribution(nil)
	if stats.Samples != nil {
		t.Fatalf("expected nil samples for empty input, got %v", stats.Samples)
	}
	if stats.Mean != 0 {
		t.Fatalf("expected mean 0 for empty input, got %f", stats.Mean)
	}
}

func TestComputeDistribution_FullStats(t *testing.T) {
	stats := computeDistribution([]float64{1, 2, 3, 4, 5})
	if stats.Min != 1 {
		t.Fatalf("expected min 1, got %f", stats.Min)
	}
	if stats.Max != 5 {
		t.Fatalf("expected max 5, got %f", stats.Max)
	}
	if stats.P50 != 3 {
		t.Fatalf("expected P50 3, got %f", stats.P50)
	}
	if math.Abs(stats.Mean-3) > 1e-9 {
		t.Fatalf("expected mean 3, got %f", stats.Mean)
	}
	if len(stats.Samples) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(stats.Samples))
	}
}
