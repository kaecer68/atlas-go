package eval

import (
	"math"
	"testing"
)

// mockPredictor implements the Predictor interface for testing.
type mockPredictor struct {
	// fn maps each sample (row of X) to a prediction value.
	fn func(X [][]float64) ([]float64, error)
}

func (m *mockPredictor) Predict(X [][]float64) ([]float64, error) {
	return m.fn(X)
}

// generateData creates n samples with nFeatures features, where each feature
// is uniformly spaced in [0, 1].
func generateData(n, nFeatures int) [][]float64 {
	X := make([][]float64, n)
	for i := range n {
		row := make([]float64, nFeatures)
		for j := range nFeatures {
			row[j] = float64(i) / float64(n-1)
		}
		X[i] = row
	}
	return X
}

// TestFriedmanH_AdditiveModel verifies that a purely additive model (no interaction)
// produces negligible H-statistic values (all < 0.1).
func TestFriedmanH_AdditiveModel(t *testing.T) {
	X := generateData(20, 2)
	// y = x1 + x2 (purely additive, no interaction)
	predictor := &mockPredictor{fn: func(X [][]float64) ([]float64, error) {
		out := make([]float64, len(X))
		for i, row := range X {
			out[i] = row[0] + row[1]
		}
		return out, nil
	}}

	// Compute y from original X for yMean
	y := make([]float64, len(X))
	for i, row := range X {
		y[i] = row[0] + row[1]
	}

	result, err := FriedmanH(predictor, X, y, []string{"x1", "x2"}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.FeatureNames) != 2 {
		t.Fatalf("expected 2 feature names, got %d", len(result.FeatureNames))
	}

	for _, pair := range result.SignificantPairs {
		if pair.HStatistic >= 0.1 {
			t.Errorf("additive model should have negligible H, got H(%s,%s)=%.4f, interpretation=%s",
				pair.FeatureA, pair.FeatureB, pair.HStatistic, pair.Interpretation)
		}
		if pair.Interpretation != "negligible" {
			t.Errorf("expected interpretation 'negligible', got %q for H=%.4f", pair.Interpretation, pair.HStatistic)
		}
	}
}

// TestFriedmanH_MultiplicativeInteraction verifies that y = x1 * x2 produces
// a strong interaction with H₁₂ > 0.3.
func TestFriedmanH_MultiplicativeInteraction(t *testing.T) {
	X := generateData(20, 2)
	// y = x1 * x2 (multiplicative interaction)
	predictor := &mockPredictor{fn: func(X [][]float64) ([]float64, error) {
		out := make([]float64, len(X))
		for i, row := range X {
			out[i] = row[0] * row[1]
		}
		return out, nil
	}}

	y := make([]float64, len(X))
	for i, row := range X {
		y[i] = row[0] * row[1]
	}

	result, err := FriedmanH(predictor, X, y, []string{"x1", "x2"}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.SignificantPairs) == 0 {
		t.Fatal("expected at least one pair")
	}

	pair := result.SignificantPairs[0]
	if pair.HStatistic < 0.3 {
		t.Errorf("multiplicative interaction should have H > 0.3, got H(%s,%s)=%.4f",
			pair.FeatureA, pair.FeatureB, pair.HStatistic)
	}

	if pair.Interpretation == "negligible" || pair.Interpretation == "weak" {
		t.Errorf("expected at least 'moderate' interaction, got %q for H=%.4f", pair.Interpretation, pair.HStatistic)
	}
}

// TestFriedmanH_ThresholdInteraction verifies that a threshold-based interaction
// (AND gate) produces H₁₂ > 0.3.
func TestFriedmanH_ThresholdInteraction(t *testing.T) {
	X := generateData(20, 2)
	// y = 1.0 if (x1 > 0.5 && x2 > 0.5) else 0
	predictor := &mockPredictor{fn: func(X [][]float64) ([]float64, error) {
		out := make([]float64, len(X))
		for i, row := range X {
			if row[0] > 0.5 && row[1] > 0.5 {
				out[i] = 1.0
			} else {
				out[i] = 0.0
			}
		}
		return out, nil
	}}

	y := make([]float64, len(X))
	for i, row := range X {
		if row[0] > 0.5 && row[1] > 0.5 {
			y[i] = 1.0
		} else {
			y[i] = 0.0
		}
	}

	result, err := FriedmanH(predictor, X, y, []string{"x1", "x2"}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.SignificantPairs) == 0 {
		t.Fatal("expected at least one pair")
	}

	pair := result.SignificantPairs[0]
	if pair.HStatistic < 0.3 {
		t.Errorf("threshold interaction should have H > 0.3, got H(%.4f)=%.4f",
			pair.HStatistic, pair.HStatistic)
	}
}

// TestFriedmanH_ThreeFeatures verifies that y = x1 + x2 + x3 + 5*x1*x2 produces
// a strong H₁₂ while other pairs remain weak.
func TestFriedmanH_ThreeFeatures(t *testing.T) {
	X := generateData(30, 3)
	// y = x1 + x2 + x3 + 5*x1*x2 (strong x1-x2 interaction)
	predictor := &mockPredictor{fn: func(X [][]float64) ([]float64, error) {
		out := make([]float64, len(X))
		for i, row := range X {
			out[i] = row[0] + row[1] + row[2] + 5*row[0]*row[1]
		}
		return out, nil
	}}

	y := make([]float64, len(X))
	for i, row := range X {
		y[i] = row[0] + row[1] + row[2] + 5*row[0]*row[1]
	}

	result, err := FriedmanH(predictor, X, y, []string{"x1", "x2", "x3"}, 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.SignificantPairs) != 3 {
		t.Fatalf("expected 3 pairs for 3 features, got %d", len(result.SignificantPairs))
	}

	// Pairs sorted by H descending; the (x1, x2) pair should be first
	top := result.SignificantPairs[0]
	if top.FeatureA != "x1" || top.FeatureB != "x2" {
		t.Errorf("expected top pair (x1, x2), got (%s, %s)", top.FeatureA, top.FeatureB)
	}
	if top.HStatistic < 0.3 {
		t.Errorf("x1*x2 interaction should be moderate or stronger (H > 0.3), got H=%.4f", top.HStatistic)
	}

	// Other pairs should have much weaker interactions
	for _, pair := range result.SignificantPairs[1:] {
		if pair.HStatistic >= top.HStatistic {
			t.Errorf("non-interacting pair (%s,%s) has H=%.4f >= top pair H=%.4f",
				pair.FeatureA, pair.FeatureB, pair.HStatistic, top.HStatistic)
		}
	}
}

// TestFriedmanH_SingleFeature verifies that less than 2 features returns an empty result.
func TestFriedmanH_SingleFeature(t *testing.T) {
	X := generateData(10, 1)
	y := make([]float64, len(X))
	for i, row := range X {
		y[i] = row[0]
	}

	predictor := &mockPredictor{fn: func(X [][]float64) ([]float64, error) {
		out := make([]float64, len(X))
		for i, row := range X {
			out[i] = row[0]
		}
		return out, nil
	}}

	result, err := FriedmanH(predictor, X, y, []string{"x1"}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.FeatureNames) != 0 {
		t.Errorf("expected empty FeatureNames for single feature, got %v", result.FeatureNames)
	}
	if len(result.SignificantPairs) != 0 {
		t.Errorf("expected empty SignificantPairs for single feature, got %d", len(result.SignificantPairs))
	}
	if result.HMatrix != nil {
		t.Errorf("expected nil HMatrix for single feature, got %v", result.HMatrix)
	}
}

// TestFriedmanH_EmptyInput verifies that empty inputs return empty results without error.
func TestFriedmanH_EmptyInput(t *testing.T) {
	predictor := &mockPredictor{fn: func(X [][]float64) ([]float64, error) {
		return nil, nil
	}}

	// Empty X
	result, err := FriedmanH(predictor, nil, nil, nil, 10)
	if err != nil {
		t.Fatalf("unexpected error for nil X: %v", err)
	}
	if len(result.FeatureNames) != 0 || len(result.SignificantPairs) != 0 {
		t.Error("expected empty result for nil X")
	}

	// Empty y
	result, err = FriedmanH(predictor, [][]float64{{1, 2}}, nil, nil, 10)
	if err != nil {
		t.Fatalf("unexpected error for nil y: %v", err)
	}
	if len(result.FeatureNames) != 0 || len(result.SignificantPairs) != 0 {
		t.Error("expected empty result for nil y")
	}

	// Empty X slice
	result, err = FriedmanH(predictor, [][]float64{}, []float64{}, nil, 10)
	if err != nil {
		t.Fatalf("unexpected error for empty slices: %v", err)
	}
	if len(result.FeatureNames) != 0 || len(result.SignificantPairs) != 0 {
		t.Error("expected empty result for empty slices")
	}
}

// TestSignificantPairs_Sorting verifies that SignificantPairs is sorted by H descending.
func TestSignificantPairs_Sorting(t *testing.T) {
	X := generateData(20, 3)
	// y = x1 + x2 + x3 + 5*x1*x2 + 2*x1*x3 (two interactions of different strength)
	predictor := &mockPredictor{fn: func(X [][]float64) ([]float64, error) {
		out := make([]float64, len(X))
		for i, row := range X {
			out[i] = row[0] + row[1] + row[2] + 5*row[0]*row[1] + 2*row[0]*row[2]
		}
		return out, nil
	}}

	y := make([]float64, len(X))
	for i, row := range X {
		y[i] = row[0] + row[1] + row[2] + 5*row[0]*row[1] + 2*row[0]*row[2]
	}

	result, err := FriedmanH(predictor, X, y, []string{"x1", "x2", "x3"}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify descending order
	for i := 1; i < len(result.SignificantPairs); i++ {
		prev := result.SignificantPairs[i-1].HStatistic
		curr := result.SignificantPairs[i].HStatistic
		if prev < curr {
			t.Errorf("SignificantPairs not sorted descending: pair[%d].H=%.4f < pair[%d].H=%.4f",
				i-1, prev, i, curr)
		}
	}

	// Verify H-matrix symmetry
	n := len(result.FeatureNames)
	for i := range n {
		for j := i + 1; j < n; j++ {
			if math.Abs(result.HMatrix[i][j]-result.HMatrix[j][i]) > 1e-10 {
				t.Errorf("H-matrix not symmetric: H[%d][%d]=%.6f, H[%d][%d]=%.6f",
					i, j, result.HMatrix[i][j], j, i, result.HMatrix[j][i])
			}
		}
	}
}

// TestInteractionPair_Interpretation verifies interpretation strings for various H values.
func TestInteractionPair_Interpretation(t *testing.T) {
	tests := []struct {
		h    float64
		want string
	}{
		{0.0, "negligible"},
		{0.05, "negligible"},
		{0.099, "negligible"},
		{0.1, "weak"},
		{0.2, "weak"},
		{0.299, "weak"},
		{0.3, "moderate"},
		{0.4, "moderate"},
		{0.499, "moderate"},
		{0.5, "strong"},
		{0.7, "strong"},
		{1.0, "strong"},
	}

	for _, tt := range tests {
		got := interpretH(tt.h)
		if got != tt.want {
			t.Errorf("interpretH(%.3f) = %q, want %q", tt.h, got, tt.want)
		}
	}
}
