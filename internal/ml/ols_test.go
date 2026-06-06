package ml

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestOLS_BetaRecovery(t *testing.T) {
	nSamples, nFeatures := 100, 2
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)

	for i := range X {
		X[i] = make([]float64, nFeatures)
		X[i][0] = rand.Float64()*10 - 5
		X[i][1] = rand.Float64()*10 - 5
		y[i] = 2.0*X[i][0] + 3.0*X[i][1] + rand.Float64()*0.001
	}

	model := NewOLS()
	err := model.Fit(X, y)
	if err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	if model.coeffs == nil || len(model.coeffs) != nFeatures+1 {
		t.Fatalf("expected %d coefficients with intercept, got %d", nFeatures+1, len(model.coeffs))
	}

	if math.Abs(model.coeffs[0]) > 0.01 {
		t.Errorf("intercept expected ~0, got %.6f", model.coeffs[0])
	}
	if math.Abs(model.coeffs[1]-2.0) > 0.01 {
		t.Errorf("β1 expected ~2.0, got %.6f", model.coeffs[1])
	}
	if math.Abs(model.coeffs[2]-3.0) > 0.01 {
		t.Errorf("β2 expected ~3.0, got %.6f", model.coeffs[2])
	}
}

func TestOLS_Predict(t *testing.T) {
	X := [][]float64{
		{1.0, 3.0},
		{4.0, 2.0},
		{5.0, 8.0},
	}
	y := []float64{4.5, 7.0, 11.0}

	model := NewOLS()
	err := model.Fit(X, y)
	if err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	pred, err := model.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}

	if len(pred) != len(y) {
		t.Errorf("expected %d predictions, got %d", len(y), len(pred))
	}
	for i := range y {
		if math.Abs(pred[i]-y[i]) > 0.01 {
			t.Errorf("pred[%d] = %.6f, want %.6f", i, pred[i], y[i])
		}
	}

	// Predict before Fit
	unfitted := NewOLS()
	_, err = unfitted.Predict(X)
	if err == nil {
		t.Error("expected error from Predict before Fit, got nil")
	}
}

func TestOLS_EmptyData(t *testing.T) {
	model := NewOLS()

	// Empty X
	err := model.Fit([][]float64{}, []float64{})
	if err == nil {
		t.Error("expected error for empty X, got nil")
	}

	// Mismatched lengths
	X := [][]float64{{1.0, 2.0}}
	err = model.Fit(X, []float64{1.0, 2.0})
	if err == nil {
		t.Error("expected error for mismatched X/y lengths, got nil")
	}
}

func TestOLS_NoIntercept(t *testing.T) {
	nSamples, nFeatures := 100, 2
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)

	for i := range X {
		X[i] = make([]float64, nFeatures)
		X[i][0] = rand.Float64()*10 - 5
		X[i][1] = rand.Float64()*10 - 5
		y[i] = 2.0*X[i][0] + 3.0*X[i][1] + rand.Float64()*0.001
	}

	model := NewOLS()
	model.FitIntercept = false
	err := model.Fit(X, y)
	if err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	if len(model.coeffs) != nFeatures {
		t.Fatalf("expected %d coefficients without intercept, got %d", nFeatures, len(model.coeffs))
	}

	if math.Abs(model.coeffs[0]-2.0) > 0.01 {
		t.Errorf("β1 expected ~2.0, got %.6f", model.coeffs[0])
	}
	if math.Abs(model.coeffs[1]-3.0) > 0.01 {
		t.Errorf("β2 expected ~3.0, got %.6f", model.coeffs[1])
	}

	// Predict should also work without intercept
	pred, err := model.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	if len(pred) != len(y) {
		t.Errorf("expected %d predictions, got %d", len(y), len(pred))
	}
}

func TestOLS_SingularMatrix(t *testing.T) {
	nSamples := 10
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	for i := range X {
		x := rand.Float64()*10 - 5
		X[i] = []float64{x, 2.0 * x}
		y[i] = x + 3.0
	}

	model := NewOLS()
	err := model.Fit(X, y)
	if err == nil {
		t.Error("expected error for singular design matrix, got nil")
	}
}
