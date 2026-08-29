package eval_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/kaecer68/atlas-go/internal/eval"
)

func TestPartialDependence(t *testing.T) {
	t.Run("simple linear relationship", func(t *testing.T) {
		// Model: y = 2*x0 + 3*x1 + 1
		// PDP on feature 0 should show a linear increase
		model := &mockPredictor{weights: []float64{2.0, 3.0}, bias: 1.0}
		nSamples := 20
		X := make([][]float64, nSamples)
		for i := range nSamples {
			X[i] = []float64{
				float64(i) * 0.5,        // feature 0: 0, 0.5, 1.0, ..., 9.5
				float64((i*7)%10) * 0.2, // feature 1: varies
			}
		}

		result, err := eval.PartialDependence(model, X, 0, 10)
		if err != nil {
			t.Fatalf("PartialDependence failed: %v", err)
		}

		if len(result.X) != 10 {
			t.Errorf("expected 10 grid points, got %d", len(result.X))
		}
		if len(result.Y) != 10 {
			t.Errorf("expected 10 Y values, got %d", len(result.Y))
		}

		// PDP Y should approximately be: avg(2*g + 3*x1_avg + 1) = 2*g + const
		// So Y should increase linearly with X
		if result.Y[0] >= result.Y[len(result.Y)-1] {
			t.Errorf("PDP Y should increase with X for positive weight. Y[0]=%v, Y[last]=%v",
				result.Y[0], result.Y[len(result.Y)-1])
		}

		// Check that grid is evenly spaced
		step := result.X[1] - result.X[0]
		for i := 1; i < len(result.X); i++ {
			diff := result.X[i] - result.X[i-1]
			if math.Abs(diff-step) > 1e-10 {
				t.Errorf("grid not evenly spaced at index %d: diff=%v, expected step=%v", i, diff, step)
			}
		}
	})

	t.Run("feature index out of range", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1.0}, bias: 0}
		X := [][]float64{{1.0}}
		_, err := eval.PartialDependence(model, X, 1, 10)
		if err == nil {
			t.Error("expected error for out-of-range feature index")
		}

		_, err = eval.PartialDependence(model, X, -1, 10)
		if err == nil {
			t.Error("expected error for negative feature index")
		}
	})

	t.Run("empty X returns error", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1.0}, bias: 0}
		_, err := eval.PartialDependence(model, [][]float64{}, 0, 10)
		if err == nil {
			t.Error("expected error for empty X")
		}
	})

	t.Run("gridResolution=0 returns error", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1.0}, bias: 0}
		X := [][]float64{{1.0}}
		_, err := eval.PartialDependence(model, X, 0, 0)
		if err == nil {
			t.Error("expected error for zero grid resolution")
		}
	})

	t.Run("gridResolution=1 works", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1.0}, bias: 0}
		X := [][]float64{{1.0}, {5.0}}
		result, err := eval.PartialDependence(model, X, 0, 1)
		if err != nil {
			t.Fatalf("PartialDependence with gridRes=1 failed: %v", err)
		}
		if len(result.X) != 1 {
			t.Errorf("expected 1 grid point, got %d", len(result.X))
		}
		if len(result.Y) != 1 {
			t.Errorf("expected 1 Y value, got %d", len(result.Y))
		}
	})

	t.Run("verify grid resolution matches requested", func(t *testing.T) {
		model := &mockPredictor{weights: []float64{1.0, 2.0, 3.0}, bias: 0.5}
		nSamples := 10
		X := make([][]float64, nSamples)
		for i := range nSamples {
			X[i] = []float64{float64(i), float64(i) * 2, float64(i) * 0.5}
		}

		for _, res := range []int{3, 5, 7, 11, 20} {
			t.Run("resolution="+strconv.Itoa(res), func(t *testing.T) {
				result, err := eval.PartialDependence(model, X, 1, res)
				if err != nil {
					t.Fatalf("PartialDependence failed: %v", err)
				}
				if len(result.X) != res {
					t.Errorf("expected %d grid points, got %d", res, len(result.X))
				}
				if len(result.Y) != res {
					t.Errorf("expected %d Y values, got %d", res, len(result.Y))
				}
			})
		}
	})

	t.Run("predict error propagates", func(t *testing.T) {
		errModel := &errorPredictor{}
		X := [][]float64{{1.0, 2.0}}
		_, err := eval.PartialDependence(errModel, X, 0, 5)
		if err == nil {
			t.Error("expected error from Predict, got nil")
		}
	})

	t.Run("constant feature produces centered PDP", func(t *testing.T) {
		// If all values of a feature are the same, grid is a single point
		model := &mockPredictor{weights: []float64{0.0, 1.0}, bias: 2.0}
		nSamples := 10
		X := make([][]float64, nSamples)
		for i := range nSamples {
			X[i] = []float64{5.0, float64(i)} // feature 0 is constant at 5.0
		}

		result, err := eval.PartialDependence(model, X, 0, 3)
		if err != nil {
			t.Fatalf("PartialDependence failed: %v", err)
		}
		// All grid points should be 5.0 since min==max
		for i, x := range result.X {
			if math.Abs(x-5.0) > 1e-10 {
				t.Errorf("grid[%d]=%v, want 5.0 (constant feature)", i, x)
			}
		}
	})
}
