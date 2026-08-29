package ml

import (
	"math"
	"math/rand/v2"
	"testing"
)

// generateL1Data creates n=50 samples, p=20 features.
// Only the first 3 features are signal; the remaining 17 are pure noise.
// True coefficients: [2.0, -1.5, 1.0], all others are 0.
func generateL1Data(rng *rand.Rand) ([][]float64, []float64, []float64) {
	n, p := 50, 20
	X := make([][]float64, n)
	y := make([]float64, n)
	trueCoef := make([]float64, p)
	trueCoef[0] = 2.0
	trueCoef[1] = -1.5
	trueCoef[2] = 1.0

	for i := range n {
		X[i] = make([]float64, p)
		for j := range p {
			X[i][j] = rng.NormFloat64()
		}
		y[i] = 0.0
		for j := range p {
			y[i] += X[i][j] * trueCoef[j]
		}
		// add minor noise
		y[i] += 0.1 * rng.NormFloat64()
	}
	return X, y, trueCoef
}

func TestElasticNet_L1Sparsity(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 0))
	X, y, trueCoef := generateL1Data(rng)

	en := NewElasticNet()
	en.L1Ratio = 0.9 // near lasso
	en.Alpha = 0.1
	en.AlphaAuto = false
	en.MaxIter = 5000
	en.Tol = 1e-5

	if err := en.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	// Check signal features (first 3) have non-zero coefficients.
	for j := range 3 {
		if en.coef[j] == 0 {
			t.Logf("WARNING: signal feature %d has zero coefficient (true=%.1f, fitted=%.6f)", j, trueCoef[j], en.coef[j])
		}
		if math.Abs(en.coef[j]) < 1e-3 {
			t.Errorf("signal feature %d should have non-zero coefficient, got %.6f (true=%.1f)", j, en.coef[j], trueCoef[j])
		}
	}

	// Check that some noise features have coefficients very close to 0.
	zeroCount := 0
	for j := 3; j < 20; j++ {
		if math.Abs(en.coef[j]) < 0.01 {
			zeroCount++
		}
	}
	if zeroCount < 8 {
		t.Errorf("L1Ratio=0.9 should produce sparsity: expected at least 8 noise features near zero, got %d", zeroCount)
	}
	t.Logf("Noise features with |coef|<0.01: %d/17", zeroCount)
}

func TestElasticNet_Predict(t *testing.T) {
	rng := rand.New(rand.NewPCG(24, 0))

	// Simple data: X is 1D, y = 3*X + 5 + noise
	n := 30
	X := make([][]float64, n)
	y := make([]float64, n)
	for i := range n {
		X[i] = []float64{float64(i) * 0.5}
		y[i] = 3*float64(i)*0.5 + 5 + 0.05*rng.NormFloat64()
	}

	en := NewElasticNet()
	en.L1Ratio = 0.5
	en.AlphaAuto = false
	en.Alpha = 0.01

	// Predict before Fit should error.
	_, err := en.Predict(X)
	if err == nil {
		t.Error("Predict before Fit should return error")
	}

	if err := en.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	pred, err := en.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	if len(pred) != n {
		t.Errorf("expected %d predictions, got %d", n, len(pred))
	}

	// Check predictions are numerically reasonable: MSE should be low.
	var mse float64
	for i := range n {
		diff := pred[i] - y[i]
		mse += diff * diff
	}
	mse /= float64(n)
	if mse > 2.0 {
		t.Errorf("MSE too large: %.4f (expected < 2.0)", mse)
	}
	t.Logf("MSE: %.6f", mse)
}

func TestElasticNet_EmptyData(t *testing.T) {
	en := NewElasticNet()
	err := en.Fit([][]float64{}, []float64{})
	if err == nil {
		t.Error("Fit with empty data should return error")
	}
	err = en.Fit([][]float64{{1.0}}, []float64{})
	if err == nil {
		t.Error("Fit with mismatched lengths should return error")
	}
}

func TestElasticNet_RidgeBehavior(t *testing.T) {
	rng := rand.New(rand.NewPCG(99, 0))
	X, y, _ := generateL1Data(rng)

	// Ridge (L1Ratio=0): all coefficients should be non-zero (no sparsity).
	ridge := NewElasticNet()
	ridge.L1Ratio = 0.0
	ridge.Alpha = 0.1
	ridge.AlphaAuto = false
	ridge.MaxIter = 5000

	if err := ridge.Fit(X, y); err != nil {
		t.Fatalf("Ridge Fit failed: %v", err)
	}

	ridgeZeroCount := 0
	for j := range 20 {
		if math.Abs(ridge.coef[j]) < 1e-6 {
			ridgeZeroCount++
		}
	}
	if ridgeZeroCount > 2 {
		t.Errorf("Ridge (L1Ratio=0) should not produce sparsity: got %d zero coefficients", ridgeZeroCount)
	}
	t.Logf("Ridge zero coefficients: %d/20", ridgeZeroCount)

	// Near-lasso (L1Ratio=0.9): some coefficients should be zero.
	lasso := NewElasticNet()
	lasso.L1Ratio = 0.9
	lasso.Alpha = 0.1
	lasso.AlphaAuto = false
	lasso.MaxIter = 5000

	if err := lasso.Fit(X, y); err != nil {
		t.Fatalf("Lasso Fit failed: %v", err)
	}

	lassoZeroCount := 0
	for j := range 20 {
		if math.Abs(lasso.coef[j]) < 0.01 {
			lassoZeroCount++
		}
	}
	if lassoZeroCount < 3 {
		t.Errorf("Lasso (L1Ratio=0.9) should produce sparsity: expected at least 3 near-zero coefficients, got %d", lassoZeroCount)
	}
	t.Logf("Lasso near-zero coefficients: %d/20", lassoZeroCount)
}
