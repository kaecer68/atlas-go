package portfolio

import (
	"testing"
)

// Test 1: C1 — compile-time method signature assertion.
// If GetWeights signature changes, this line fails to compile.
func TestContract_GetWeights_Signature(t *testing.T) {
	var _ func(*FactorWeightEngine, string) map[FactorType]float64 = (*FactorWeightEngine).GetWeights
}

// Test 2: C2 — compile-time method signature assertion.
// If OnRegimeChange signature changes, this line fails to compile.
func TestContract_OnRegimeChange_Signature(t *testing.T) {
	var _ func(*FactorWeightEngine, string, string, float64) = (*FactorWeightEngine).OnRegimeChange
}

// Test 3: C1 — runtime behavior verification.
// Verify GetWeights("") returns a non-nil map with factor weights that sum to approximately 1.0.
func TestContract_GetWeights_ReturnsNonNil(t *testing.T) {
	engine := NewFactorWeightEngine()
	if engine == nil {
		t.Fatal("NewFactorWeightEngine() returned nil — contract violation")
	}
	weights := engine.GetWeights("")
	if weights == nil {
		t.Error("GetWeights(\"\") returned nil map")
	}
	var sum float64
	for _, w := range weights {
		sum += float64(w)
	}
	if len(weights) > 0 && (sum < 0.9 || sum > 1.1) {
		t.Errorf("GetWeights sum = %f, want ~1.0 (got %d factors)", sum, len(weights))
	}
}
