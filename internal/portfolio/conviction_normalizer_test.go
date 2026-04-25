package portfolio

import (
	"math"
	"testing"
)

func TestConvictionNormalizer_RecordConviction(t *testing.T) {
	cn := NewConvictionNormalizer()

	cn.RecordConviction("agent1", 50)
	cn.RecordConviction("agent1", 52)
	cn.RecordConviction("agent1", 48)

	count, mean, stdDev, min, max := cn.GetStats("agent1")

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
	if math.Abs(mean-50) > 0.01 {
		t.Errorf("expected mean 50, got %f", mean)
	}
	if stdDev <= 0 {
		t.Error("expected positive stdDev")
	}
	if min != 48 {
		t.Errorf("expected min 48, got %f", min)
	}
	if max != 52 {
		t.Errorf("expected max 52, got %f", max)
	}
}

func TestConvictionNormalizer_ZScore(t *testing.T) {
	cn := NewConvictionNormalizer()

	for _, v := range []int{45, 50, 55, 60, 65} {
		cn.RecordConviction("agent1", v)
	}

	norm := cn.Normalize("agent1", 55, ZScore)

	if math.Abs(norm) > 0.01 {
		t.Errorf("expected ZScore for mean to be ~0, got %f", norm)
	}

	normBelow := cn.Normalize("agent1", 45, ZScore)
	if normBelow >= 0 {
		t.Errorf("expected negative ZScore for value below mean, got %f", normBelow)
	}

	normAbove := cn.Normalize("agent1", 65, ZScore)
	if normAbove <= 0 {
		t.Errorf("expected positive ZScore for value above mean, got %f", normAbove)
	}
}

func TestConvictionNormalizer_Percentile(t *testing.T) {
	cn := NewConvictionNormalizer()

	for _, v := range []int{40, 50, 60, 70, 80} {
		cn.RecordConviction("agent1", v)
	}

	norm := cn.Normalize("agent1", 60, Percentile)

	if norm < 40 || norm > 70 {
		t.Errorf("expected Percentile around 50-60 for mean value, got %f", norm)
	}

	normMin := cn.Normalize("agent1", 40, Percentile)
	if normMin >= 10 {
		t.Errorf("expected low Percentile for min value, got %f", normMin)
	}

	normMax := cn.Normalize("agent1", 80, Percentile)
	if normMax <= 90 {
		t.Errorf("expected high Percentile for max value, got %f", normMax)
	}
}

func TestConvictionNormalizer_MinMax(t *testing.T) {
	cn := NewConvictionNormalizer()

	for _, v := range []int{30, 50, 70} {
		cn.RecordConviction("agent1", v)
	}

	normMin := cn.Normalize("agent1", 30, MinMax)
	if math.Abs(normMin) > 0.01 {
		t.Errorf("expected MinMax 0 for min value, got %f", normMin)
	}

	normMax := cn.Normalize("agent1", 70, MinMax)
	if math.Abs(normMax-1) > 0.01 {
		t.Errorf("expected MinMax 1 for max value, got %f", normMax)
	}

	normMid := cn.Normalize("agent1", 50, MinMax)
	if math.Abs(normMid-0.5) > 0.01 {
		t.Errorf("expected MinMax 0.5 for midpoint, got %f", normMid)
	}
}

func TestConvictionNormalizer_InsufficientData(t *testing.T) {
	cn := NewConvictionNormalizer()

	cn.RecordConviction("agent1", 50)

	norm := cn.Normalize("agent1", 50, ZScore)
	if norm != 50 {
		t.Errorf("expected fallback to raw value 50, got %f", norm)
	}

	norm = cn.Normalize("agent1", 50, Percentile)
	if norm != 50 {
		t.Errorf("expected fallback to raw value 50, got %f", norm)
	}

	norm = cn.Normalize("agent1", 50, MinMax)
	if norm != 50 {
		t.Errorf("expected fallback to raw value 50, got %f", norm)
	}

	norm = cn.Normalize("nonexistent", 50, ZScore)
	if norm != 50 {
		t.Errorf("expected fallback to raw value 50 for unknown agent, got %f", norm)
	}
}

func TestConvictionNormalizer_StandardNormalCDF(t *testing.T) {
	cn := NewConvictionNormalizer()

	tests := []struct {
		z      float64
		min    float64
		max    float64
		approx string
	}{
		{0, 0.49, 0.51, "z=0 should be ~0.5"},
		{1.96, 0.97, 0.98, "z=1.96 should be ~0.975"},
		{-1.96, 0.02, 0.03, "z=-1.96 should be ~0.025"},
		{3, 0.998, 0.999, "z=3 should be ~0.9987"},
	}

	for _, tt := range tests {
		result := cn.standardNormalCDF(tt.z)
		if result < tt.min || result > tt.max {
			t.Errorf("%s: expected ~%f, got %f", tt.approx, (tt.min+tt.max)/2, result)
		}
	}
}

func TestConvictionNormalizer_MultipleAgents(t *testing.T) {
	cn := NewConvictionNormalizer()

	for _, v := range []int{10, 20, 30} {
		cn.RecordConviction("agent1", v)
	}
	for _, v := range []int{100, 200, 300} {
		cn.RecordConviction("agent2", v)
	}

	norm1 := cn.Normalize("agent1", 20, ZScore)
	norm2 := cn.Normalize("agent2", 200, ZScore)

	if math.Abs(norm1) > 0.1 || math.Abs(norm2) > 0.1 {
		t.Errorf("expected ZScore ~0 for mean values: agent1=%f, agent2=%f", norm1, norm2)
	}

	_, mean1, _, _, _ := cn.GetStats("agent1")
	_, mean2, _, _, _ := cn.GetStats("agent2")

	if math.Abs(mean1-20) > 0.01 {
		t.Errorf("expected agent1 mean 20, got %f", mean1)
	}
	if math.Abs(mean2-200) > 0.01 {
		t.Errorf("expected agent2 mean 200, got %f", mean2)
	}
}
