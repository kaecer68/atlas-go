package ml

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestGLMSpline_Gaussian(t *testing.T) {
	// Generate synthetic linear data.
	nSamples := 100
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	for i := range X {
		X[i] = []float64{rand.Float64()*10 - 5, rand.Float64()*10 - 5}
		y[i] = 2.0*X[i][0] + 3.0*X[i][1] + rand.Float64()*0.1
	}

	// Fit GLMSpline with gaussian family (should behave like OLS + spline).
	g := NewGLMSpline()
	g.Degree = 3
	g.Family = "gaussian"
	g.FitIntercept = true
	if err := g.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	pred, err := g.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	if len(pred) != nSamples {
		t.Errorf("expected %d predictions, got %d", nSamples, len(pred))
	}

	// Compute R².
	meanY := 0.0
	for _, v := range y {
		meanY += v
	}
	meanY /= float64(nSamples)
	ssRes, ssTot := 0.0, 0.0
	for i := range y {
		ssRes += (y[i] - pred[i]) * (y[i] - pred[i])
		ssTot += (y[i] - meanY) * (y[i] - meanY)
	}
	r2 := 1.0 - ssRes/ssTot
	if r2 < 0.8 {
		t.Errorf("expected R² > 0.8, got %.4f", r2)
	}
}

func TestGLMSpline_EmptyData(t *testing.T) {
	g := NewGLMSpline()

	// Empty X.
	if err := g.Fit([][]float64{}, []float64{}); err == nil {
		t.Error("expected error for empty X, got nil")
	}

	// Mismatched lengths.
	X := [][]float64{{1.0, 2.0}}
	if err := g.Fit(X, []float64{1.0, 2.0}); err == nil {
		t.Error("expected error for mismatched X/y lengths, got nil")
	}

	// Predict before Fit.
	_, err := g.Predict(X)
	if err == nil {
		t.Error("expected error from Predict before Fit, got nil")
	}
}

func TestGLMSpline_Poisson(t *testing.T) {
	nSamples := 100
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	rng := rand.New(rand.NewPCG(42, 0))
	for i := range X {
		x := rng.Float64()*2 - 1
		X[i] = []float64{x}
		// Poisson mean = exp(0.5 + 0.8*x).
		lambda := math.Exp(0.5 + 0.8*x)
		y[i] = float64(poissonRand(rng, lambda))
	}

	g := NewGLMSpline()
	g.Degree = 2 // Use low degree for Poisson to avoid overfitting.
	g.Family = "poisson"
	g.FitIntercept = true
	if err := g.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	pred, err := g.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}

	// Poisson predictions should be non-negative.
	for i, p := range pred {
		if p < -1e-6 {
			t.Errorf("pred[%d] = %f, expected non-negative for Poisson", i, p)
		}
	}
}

func TestGLMSpline_FitCV(t *testing.T) {
	nSamples := 80
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	for i := range X {
		x := rand.Float64()*10 - 5
		X[i] = []float64{x}
		y[i] = 2.0*x + rand.Float64()*0.5
	}

	g := NewGLMSpline()
	g.Family = "gaussian"
	// FitCV should try degrees and pick the best.
	if err := g.FitCV(X, y, []int{2, 3, 4, 5}, 3); err != nil {
		t.Fatalf("FitCV failed: %v", err)
	}

	if !g.fitted {
		t.Error("expected model to be fitted after FitCV")
	}

	// Should be able to predict after CV fit.
	pred, err := g.Predict(X)
	if err != nil {
		t.Fatalf("Predict after CV failed: %v", err)
	}
	if len(pred) != nSamples {
		t.Errorf("expected %d predictions, got %d", nSamples, len(pred))
	}
}

func TestGLMSpline_SingleFeature(t *testing.T) {
	// Non-linear data: y = sin(x) + noise.
	nSamples := 100
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	for i := range X {
		x := rand.Float64()*6 - 3 // Range [-3, 3].
		X[i] = []float64{x}
		y[i] = math.Sin(x) + rand.Float64()*0.2
	}

	// Fit spline model.
	g := NewGLMSpline()
	g.Degree = 4
	g.Family = "gaussian"
	if err := g.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	splinePred, err := g.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}

	// Fit OLS for comparison.
	ols := NewOLS()
	if err := ols.Fit(X, y); err != nil {
		t.Fatalf("OLS Fit failed: %v", err)
	}
	olsPred, err := ols.Predict(X)
	if err != nil {
		t.Fatalf("OLS Predict failed: %v", err)
	}

	// Compute R² for both.
	splineR2 := computeR2(y, splinePred)
	olsR2 := computeR2(y, olsPred)

	// Spline should fit non-linear sin(x) at least as well as OLS.
	if splineR2 < olsR2-0.1 {
		t.Errorf("spline R² (%.4f) unexpectedly worse than OLS R² (%.4f) on non-linear data", splineR2, olsR2)
	}

	// Spline should have positive R² (it's modeling sin(x) with spline basis).
	if splineR2 < 0.0 {
		t.Errorf("spline R² (%.4f) should be positive on non-linear data", splineR2)
	}
}

// poissonRand generates a Poisson-distributed random integer with mean lambda.
// Uses the Knuth algorithm (simple but adequate for small lambda).
func poissonRand(rng *rand.Rand, lambda float64) int {
	if lambda <= 0 {
		return 0
	}
	L := math.Exp(-lambda)
	k := 0
	p := 1.0
	for p > L {
		k++
		p *= rng.Float64()
	}
	return k - 1
}
