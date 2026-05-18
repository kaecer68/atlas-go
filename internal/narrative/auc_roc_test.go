package narrative

import (
	"math"
	"testing"
)

func TestComputeAUCROC_PerfectClassifier(t *testing.T) {
	labels := []bool{true, true, false, false}
	scores := []float64{0.9, 0.8, 0.2, 0.1}
	result := ComputeAUCROC(labels, scores)
	if math.Abs(result.AUC-1.0) > 1e-9 {
		t.Errorf("expected AUC=1.0, got %.4f", result.AUC)
	}
}

func TestComputeAUCROC_RandomClassifier(t *testing.T) {
	labels := []bool{true, false, true, false}
	scores := []float64{0.5, 0.5, 0.5, 0.5}
	result := ComputeAUCROC(labels, scores)
	if result.AUC != 0.5 {
		t.Errorf("expected AUC=0.5 for random, got %.4f", result.AUC)
	}
}

func TestComputeAUCROC_EmptyInput(t *testing.T) {
	result := ComputeAUCROC(nil, nil)
	if result.AUC != 0 {
		t.Errorf("expected AUC=0 for empty input, got %.4f", result.AUC)
	}
}
