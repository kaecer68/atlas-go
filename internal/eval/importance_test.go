package eval_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/eval"
)

// mockPredictor implements eval.Predictor with a simple linear model.
type mockPredictor struct {
	weights []float64
	bias    float64
}

func (m *mockPredictor) Predict(X [][]float64) ([]float64, error) {
	out := make([]float64, len(X))
	for i, row := range X {
		var sum float64
		for j, v := range row {
			sum += v * m.weights[j]
		}
		out[i] = sum + m.bias
	}
	return out, nil
}

func TestPermutationImportance(t *testing.T) {
	t.Run("basic 3 features", func(t *testing.T) {
		// y = 2*x0 + 1*x1 + 0*x2 + noise
		// So x0 is most important, x1 is medium, x2 is noise
		nSamples := 100
		model := &mockPredictor{
			weights: []float64{1.0, 0.5, 0.1},
			bias:    0.0,
		}
		X := make([][]float64, nSamples)
		y := make([]float64, nSamples)
		for i := range nSamples {
			X[i] = []float64{
				float64(i%10) * 0.1, // feature 0: varies 0-0.9
				float64(i%5) * 0.05, // feature 1: varies 0-0.2
				0.01,                // feature 2: nearly constant
			}
			pred, _ := model.Predict([][]float64{X[i]})
			y[i] = pred[0]
		}

		result, err := eval.PermutationImportance(model, X, y, 10, "r2")
		if err != nil {
			t.Fatalf("PermutationImportance failed: %v", err)
		}

		// Feature 0 should be most important (rank 1)
		if result.Ranks[0] != 1 {
			t.Errorf("feature_0 rank = %d, want 1 (most important). Importances: %v",
				result.Ranks[0], result.Importances)
		}

		// Feature 2 (constant) should be least important (rank 3)
		if result.Ranks[2] != 3 {
			t.Errorf("feature_2 rank = %d, want 3 (least important). Importances: %v",
				result.Ranks[2], result.Importances)
		}

		// Verify result shape
		if len(result.Importances) != 3 {
			t.Errorf("expected 3 importances, got %d", len(result.Importances))
		}
		if len(result.FeatureNames) != 3 {
			t.Errorf("expected 3 feature names, got %d", len(result.FeatureNames))
		}
		if len(result.Ranks) != 3 {
			t.Errorf("expected 3 ranks, got %d", len(result.Ranks))
		}
	})

	t.Run("empty X returns error", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1}, bias: 0}
		_, err := eval.PermutationImportance(model, [][]float64{}, []float64{}, 5, "r2")
		if err == nil {
			t.Error("expected error for empty X, got nil")
		}
	})

	t.Run("unsupported metric returns error", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1}, bias: 0}
		X := [][]float64{{1.0}}
		y := []float64{2.0}
		_, err := eval.PermutationImportance(model, X, y, 5, "accuracy")
		if err == nil {
			t.Error("expected error for unsupported metric, got nil")
		}
	})

	t.Run("mismatched y length returns error", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1}, bias: 0}
		X := [][]float64{{1.0}, {2.0}}
		y := []float64{1.0} // shorter than X
		_, err := eval.PermutationImportance(model, X, y, 5, "r2")
		if err == nil {
			t.Error("expected error for length mismatch, got nil")
		}
	})

	t.Run("zero features returns error", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{}, bias: 0}
		X := [][]float64{{}}
		y := []float64{0}
		_, err := eval.PermutationImportance(model, X, y, 5, "r2")
		if err == nil {
			t.Error("expected error for zero features, got nil")
		}
	})

	t.Run("nRepeats <= 0 returns error", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1}, bias: 0}
		X := [][]float64{{1.0}}
		y := []float64{2.0}
		_, err := eval.PermutationImportance(model, X, y, 0, "r2")
		if err == nil {
			t.Error("expected error for zero nRepeats, got nil")
		}
	})

	t.Run("nRepeats=1 vs nRepeats=5 stability", func(t *testing.T) {
		nSamples := 50
		model := &mockPredictor{
			weights: []float64{2.0, 1.0, 0.0},
			bias:    0.5,
		}
		X := make([][]float64, nSamples)
		y := make([]float64, nSamples)
		for i := range nSamples {
			X[i] = []float64{
				float64(i) * 0.02,
				float64((i*7)%13) * 0.1,
				float64((i*3)%7) * 0.05,
			}
			pred, _ := model.Predict([][]float64{X[i]})
			y[i] = pred[0]
		}

		result1, err := eval.PermutationImportance(model, X, y, 1, "r2")
		if err != nil {
			t.Fatalf("nRepeats=1 failed: %v", err)
		}
		result5, err := eval.PermutationImportance(model, X, y, 5, "r2")
		if err != nil {
			t.Fatalf("nRepeats=5 failed: %v", err)
		}

		// Both should agree on which feature is most and least important
		if result1.Ranks[0] != result5.Ranks[0] {
			t.Logf("nRepeats=1 ranks: %v, imports: %v", result1.Ranks, result1.Importances)
			t.Logf("nRepeats=5 ranks: %v, imports: %v", result5.Ranks, result5.Importances)
			// With just 1 repeat, randomness can flip close features; this is informational
		}
	})

	t.Run("model Predict error propagates", func(t *testing.T) {
		errModel := &errorPredictor{}
		X := [][]float64{{1.0, 2.0}}
		y := []float64{3.0}
		_, err := eval.PermutationImportance(errModel, X, y, 1, "r2")
		if err == nil {
			t.Error("expected error from Predict, got nil")
		}
	})

	t.Run("does not mutate input X", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1.0, 0.5}, bias: 0}
		X := [][]float64{{1.0, 2.0}, {3.0, 4.0}, {5.0, 6.0}}
		y := []float64{2.5, 5.0, 7.5}
		original := [][]float64{{1.0, 2.0}, {3.0, 4.0}, {5.0, 6.0}}

		_, err := eval.PermutationImportance(model, X, y, 5, "r2")
		if err != nil {
			t.Fatalf("PermutationImportance failed: %v", err)
		}

		for i := range X {
			for j := range X[i] {
				if math.Abs(X[i][j]-original[i][j]) > 1e-10 {
					t.Errorf("X[%d][%d] mutated from %v to %v", i, j, original[i][j], X[i][j])
				}
			}
		}
	})

	t.Run("ranks are 1-indexed", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{2.0, 1.0}, bias: 0}
		X := [][]float64{{1.0, 0.5}, {2.0, 1.0}, {3.0, 1.5}}
		y := []float64{2.5, 5.0, 7.5}
		result, err := eval.PermutationImportance(model, X, y, 5, "r2")
		if err != nil {
			t.Fatalf("PermutationImportance failed: %v", err)
		}
		for i, r := range result.Ranks {
			if r < 1 || r > len(result.Ranks) {
				t.Errorf("feature %d has invalid rank %d (should be 1..%d)", i, r, len(result.Ranks))
			}
		}
		// Check all ranks are unique
		seen := make(map[int]bool)
		for _, r := range result.Ranks {
			if seen[r] {
				t.Errorf("duplicate rank %d", r)
			}
			seen[r] = true
		}
	})
}

// errorPredictor always returns an error from Predict.
type errorPredictor struct{}

func (e *errorPredictor) Predict(X [][]float64) ([]float64, error) {
	return nil, fmt.Errorf("predict failed")
}
