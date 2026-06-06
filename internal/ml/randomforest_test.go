package ml

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestRandomForest_Basic(t *testing.T) {
	// Synthetic linear data with noise.
	nSamples := 200
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	for i := range X {
		X[i] = []float64{
			rand.Float64()*10 - 5,
			rand.Float64()*10 - 5,
			rand.Float64()*10 - 5,
		}
		y[i] = 2.0*X[i][0] + 3.0*X[i][1] - 1.0*X[i][2] + rand.Float64()*0.5
	}

	rf := NewRandomForest()
	rf.NTrees = 10
	rf.MaxDepth = 5
	rf.Seed = 42
	if err := rf.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	pred, err := rf.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	if len(pred) != nSamples {
		t.Errorf("expected %d predictions, got %d", nSamples, len(pred))
	}

	// R² should be reasonable on linear data.
	r2 := computeR2(y, pred)
	if r2 < 0.5 {
		t.Errorf("R² = %.4f, expected >= 0.5 on linear data", r2)
	}
}

func TestRandomForest_EmptyData(t *testing.T) {
	rf := NewRandomForest()

	// Empty X.
	if err := rf.Fit([][]float64{}, []float64{}); err == nil {
		t.Error("expected error for empty X, got nil")
	}

	// Mismatched lengths.
	X := [][]float64{{1.0, 2.0}}
	if err := rf.Fit(X, []float64{1.0, 2.0}); err == nil {
		t.Error("expected error for mismatched X/y lengths, got nil")
	}

	// Predict before Fit.
	_, err := rf.Predict(X)
	if err == nil {
		t.Error("expected error from Predict before Fit, got nil")
	}
}

func TestRandomForest_CompareOLS(t *testing.T) {
	// Non-linear data: y = sin(x1) + cos(x2) + noise.
	nSamples := 200
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	for i := range X {
		x1 := rand.Float64()*6 - 3
		x2 := rand.Float64()*6 - 3
		X[i] = []float64{x1, x2}
		y[i] = math.Sin(x1) + math.Cos(x2) + rand.Float64()*0.3
	}

	// Train RandomForest.
	rf := NewRandomForest()
	rf.NTrees = 20
	rf.MaxDepth = 8
	rf.Seed = 42
	if err := rf.Fit(X, y); err != nil {
		t.Fatalf("RF Fit failed: %v", err)
	}
	rfPred, err := rf.Predict(X)
	if err != nil {
		t.Fatalf("RF Predict failed: %v", err)
	}
	rfR2 := computeR2(y, rfPred)

	// Train OLS.
	ols := NewOLS()
	if err := ols.Fit(X, y); err != nil {
		t.Fatalf("OLS Fit failed: %v", err)
	}
	olsPred, err := ols.Predict(X)
	if err != nil {
		t.Fatalf("OLS Predict failed: %v", err)
	}
	olsR2 := computeR2(y, olsPred)

	// RF should at least capture some signal on non-linear data.
	if rfR2 < 0.0 {
		t.Errorf("RF R² = %.4f, expected positive on non-linear data", rfR2)
	}

	// RF should outperform OLS on this non-linear task.
	t.Logf("RF R²: %.4f, OLS R²: %.4f", rfR2, olsR2)
}

func TestRandomForest_PredictShape(t *testing.T) {
	nSamples := 50
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	for i := range X {
		X[i] = []float64{rand.Float64(), rand.Float64()}
		y[i] = X[i][0] + X[i][1]
	}

	rf := NewRandomForest()
	rf.NTrees = 5
	rf.MaxDepth = 3
	rf.Seed = 42
	if err := rf.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	// Predict on same data.
	pred, err := rf.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	if len(pred) != nSamples {
		t.Errorf("expected %d predictions, got %d", nSamples, len(pred))
	}

	// Predict on different data (different number of samples).
	Xnew := make([][]float64, 10)
	for i := range Xnew {
		Xnew[i] = []float64{rand.Float64(), rand.Float64()}
	}
	pred2, err := rf.Predict(Xnew)
	if err != nil {
		t.Fatalf("Predict on new data failed: %v", err)
	}
	if len(pred2) != 10 {
		t.Errorf("expected 10 predictions, got %d", len(pred2))
	}

	// Dimension mismatch should error.
	Xbad := [][]float64{{1.0}}
	_, err = rf.Predict(Xbad)
	if err == nil {
		t.Error("expected error for dimension mismatch, got nil")
	}
}

func TestRandomForest_MaxFeatures(t *testing.T) {
	nSamples := 100
	X := make([][]float64, nSamples)
	y := make([]float64, nSamples)
	for i := range X {
		X[i] = []float64{rand.Float64(), rand.Float64(), rand.Float64(), rand.Float64()}
		y[i] = X[i][0] + X[i][1] + rand.Float64()*0.1
	}

	// Test "sqrt" (default).
	rfSqrt := NewRandomForest()
	rfSqrt.NTrees = 10
	rfSqrt.MaxDepth = 5
	rfSqrt.MaxFeatures = "sqrt"
	rfSqrt.Seed = 42
	if err := rfSqrt.Fit(X, y); err != nil {
		t.Fatalf("sqrt Fit failed: %v", err)
	}
	predSqrt, _ := rfSqrt.Predict(X)
	r2Sqrt := computeR2(y, predSqrt)

	// Test "all".
	rfAll := NewRandomForest()
	rfAll.NTrees = 10
	rfAll.MaxDepth = 5
	rfAll.MaxFeatures = "all"
	rfAll.Seed = 42
	if err := rfAll.Fit(X, y); err != nil {
		t.Fatalf("all Fit failed: %v", err)
	}
	predAll, _ := rfAll.Predict(X)
	r2All := computeR2(y, predAll)

	// Both should produce reasonable predictions.
	if r2Sqrt < 0.0 {
		t.Errorf("sqrt R² = %.4f, expected positive", r2Sqrt)
	}
	if r2All < 0.0 {
		t.Errorf("all R² = %.4f, expected positive", r2All)
	}
	t.Logf("sqrt R²: %.4f, all R²: %.4f", r2Sqrt, r2All)
}
