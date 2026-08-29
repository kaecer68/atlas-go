package ml

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestPCR_DimensionReduction(t *testing.T) {
	// Generate collinear data: n=60, p=6, true rank ≈ 2
	// Derived from 2 latent variables: f1, f2
	n, p := 60, 6
	rng := rand.New(rand.NewPCG(42, 0))

	f1 := make([]float64, n)
	f2 := make([]float64, n)
	for i := range n {
		f1[i] = rng.NormFloat64()
		f2[i] = rng.NormFloat64()
	}

	X := make([][]float64, n)
	for i := range n {
		X[i] = make([]float64, p)
		// First 3 features from f1
		for j := range 3 {
			X[i][j] = f1[i] + 0.05*rng.NormFloat64()
		}
		// Last 3 features from f2
		for j := 3; j < 6; j++ {
			X[i][j] = f2[i] + 0.05*rng.NormFloat64()
		}
	}

	y := make([]float64, n)
	for i := range n {
		y[i] = 3*f1[i] - 2*f2[i] + 0.1*rng.NormFloat64()
	}

	// Fit PCR with NComponents=4
	pcr := NewPCR()
	pcr.NComponents = 4
	if err := pcr.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	pred, err := pcr.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	if len(pred) != n {
		t.Fatalf("expected %d predictions, got %d", n, len(pred))
	}

	// Since true rank=2, PCR should pick up the signal.
	// Check that predictions are "in the ballpark" (R² > 0.5).
	var ssRes, ssTot, meanY float64
	for _, v := range y {
		meanY += v
	}
	meanY /= float64(n)
	for i := range n {
		ssRes += (y[i] - pred[i]) * (y[i] - pred[i])
		ssTot += (y[i] - meanY) * (y[i] - meanY)
	}
	r2 := 1.0 - ssRes/ssTot
	if r2 < 0.5 {
		t.Errorf("R² too low: %.4f (expected > 0.5 for rank-2 data)", r2)
	}

	// Verify VarianceThreshold=0.95 auto-selects ≤ 4 components
	pcr2 := NewPCR()
	pcr2.VarianceThreshold = 0.95
	if err := pcr2.Fit(X, y); err != nil {
		t.Fatalf("Fit with VarianceThreshold=0.95 failed: %v", err)
	}
	if pcr2.nComponentsUsed > 4 {
		t.Errorf("VarianceThreshold=0.95 selected %d components, expected ≤ 4", pcr2.nComponentsUsed)
	}
	// With true rank=2, 0.95 threshold should need at most 3 components
	if pcr2.nComponentsUsed > 3 {
		t.Errorf("VarianceThreshold=0.95 used %d components, expected ≤ 3 for rank-2 data", pcr2.nComponentsUsed)
	}

	t.Logf("NComponents=4: R²=%.4f, VarianceThreshold=0.95 used %d components", r2, pcr2.nComponentsUsed)
}

func TestPCR_Predict(t *testing.T) {
	// Small synthetic data: y = 2*x1 + 3*x2
	X := [][]float64{
		{1, 2},
		{2, 1},
		{3, 4},
		{4, 3},
		{5, 5},
	}
	y := make([]float64, len(X))
	for i, row := range X {
		y[i] = 2*row[0] + 3*row[1]
	}

	pcr := NewPCR()
	if err := pcr.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	pred, err := pcr.Predict(X)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}
	if len(pred) != len(y) {
		t.Fatalf("expected %d predictions, got %d", len(y), len(pred))
	}

	// Perfect linear relationship, should fit very well
	for i := range y {
		if math.Abs(y[i]-pred[i]) > 1e-6 {
			t.Errorf("prediction[%d] = %f, expected %f (diff %e)", i, pred[i], y[i], math.Abs(y[i]-pred[i]))
		}
	}

	// Predict before Fit should error
	pcr2 := NewPCR()
	_, err = pcr2.Predict(X)
	if err == nil {
		t.Error("Predict before Fit should return error")
	}

	// Mismatched dimensions
	if err := pcr.Fit(X, y); err != nil {
		t.Fatalf("re-fit failed: %v", err)
	}
	_, err = pcr.Predict([][]float64{{1, 2, 3}})
	if err == nil {
		t.Error("Predict with wrong dimension should return error")
	}
}

func TestPCR_EmptyData(t *testing.T) {
	pcr := NewPCR()
	if err := pcr.Fit(nil, nil); err == nil {
		t.Error("Fit with nil X should return error")
	}
	if err := pcr.Fit([][]float64{}, []float64{}); err == nil {
		t.Error("Fit with empty X should return error")
	}
}

func TestPCR_VarianceThreshold(t *testing.T) {
	// Same collinear data as DimensionReduction test
	n, p := 60, 6
	rng := rand.New(rand.NewPCG(99, 0))

	f1 := make([]float64, n)
	f2 := make([]float64, n)
	for i := range n {
		f1[i] = rng.NormFloat64()
		f2[i] = rng.NormFloat64()
	}

	X := make([][]float64, n)
	for i := range n {
		X[i] = make([]float64, p)
		for j := range 3 {
			X[i][j] = f1[i] + 0.05*rng.NormFloat64()
		}
		for j := 3; j < 6; j++ {
			X[i][j] = f2[i] + 0.05*rng.NormFloat64()
		}
	}

	y := make([]float64, n)
	for i := range n {
		y[i] = 3*f1[i] - 2*f2[i] + 0.1*rng.NormFloat64()
	}

	// VarianceThreshold=0.999 should use all components for 6-feature rank-2 data
	pcr := NewPCR()
	pcr.VarianceThreshold = 0.999
	if err := pcr.Fit(X, y); err != nil {
		t.Fatalf("Fit with VarianceThreshold=0.999 failed: %v", err)
	}

	if pcr.nComponentsUsed == 1 {
		t.Errorf("VarianceThreshold=0.999 should use >1 component, got %d", pcr.nComponentsUsed)
	}
	t.Logf("VarianceThreshold=0.999 used %d components", pcr.nComponentsUsed)
}
