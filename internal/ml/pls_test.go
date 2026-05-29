package ml

import (
	"math"
	"math/rand/v2"
	"testing"
)

// generateCollinearData creates n×p feature matrix X and target y where
// X[:,0:3] ≈ f1 + noise, X[:,3:6] ≈ f2 + noise, and y ≈ 3*f1 - 2*f2 + noise.
func generateCollinearData(n, p int) ([][]float64, []float64) {
	rng := rand.New(rand.NewPCG(42, 0))
	X := make([][]float64, n)
	y := make([]float64, n)

	for i := range n {
		f1 := rng.NormFloat64() * 2.0
		f2 := rng.NormFloat64() * 1.5
		X[i] = make([]float64, p)
		for j := range 3 {
			X[i][j] = f1 + rng.NormFloat64()*0.1
		}
		for j := 3; j < p; j++ {
			X[i][j] = f2 + rng.NormFloat64()*0.1
		}
		y[i] = 3*f1 - 2*f2 + rng.NormFloat64()*0.5
	}
	return X, y
}

// meanPredictorMSE computes MSE of predicting mean(y) for every sample.
func meanPredictorMSE(y []float64) float64 {
	if len(y) == 0 {
		return 0
	}
	var sum float64
	for _, v := range y {
		sum += v
	}
	mean := sum / float64(len(y))
	var mse float64
	for _, v := range y {
		diff := v - mean
		mse += diff * diff
	}
	return mse / float64(len(y))
}

func mse(yTrue, yPred []float64) float64 {
	if len(yTrue) == 0 {
		return 0
	}
	var sum float64
	for i := range yTrue {
		diff := yTrue[i] - yPred[i]
		sum += diff * diff
	}
	return sum / float64(len(yTrue))
}

func TestPLS_DimensionReduction(t *testing.T) {
	X, y := generateCollinearData(60, 6)

	pls := NewPLS()
	pls.NComponents = 2

	err := pls.Fit(X, y)
	if err != nil {
		t.Fatalf("PLS Fit failed: %v", err)
	}

	pred, err := pls.Predict(X)
	if err != nil {
		t.Fatalf("PLS Predict failed: %v", err)
	}

	if len(pred) != len(y) {
		t.Fatalf("expected %d predictions, got %d", len(y), len(pred))
	}

	// PLS with 2 components should substantially beat the naive mean predictor.
	naiveMSE := meanPredictorMSE(y)
	modelMSE := mse(y, pred)

	if modelMSE >= naiveMSE {
		t.Errorf("PLS MSE (%f) should be lower than naive mean MSE (%f)", modelMSE, naiveMSE)
	}

	// Verify predictions are in a reasonable range (no NaN or extreme values).
	for i, v := range pred {
		if math.IsNaN(v) {
			t.Errorf("prediction[%d] is NaN", i)
		}
		if math.IsInf(v, 0) {
			t.Errorf("prediction[%d] is Inf", i)
		}
	}
}

func TestPLS_Predict(t *testing.T) {
	X, y := generateCollinearData(30, 4)

	// Predict before Fit should error.
	pls := NewPLS()
	_, err := pls.Predict(X)
	if err == nil {
		t.Error("expected error from Predict before Fit, got nil")
	}

	// Fit first.
	pls.NComponents = 2
	if err := pls.Fit(X, y); err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	// Valid prediction.
	pred, err := pls.Predict(X)
	if err != nil {
		t.Fatalf("Predict after Fit failed: %v", err)
	}
	if len(pred) != len(y) {
		t.Fatalf("expected %d predictions, got %d", len(y), len(pred))
	}

	// Predict with mismatched feature count should error.
	Xwrong := make([][]float64, 3)
	for i := range Xwrong {
		Xwrong[i] = make([]float64, 3) // only 3 features, not 4
	}
	_, err = pls.Predict(Xwrong)
	if err == nil {
		t.Error("expected error from Predict with wrong feature count, got nil")
	}
}

func TestPLS_EmptyData(t *testing.T) {
	pls := NewPLS()

	err := pls.Fit([][]float64{}, []float64{})
	if err == nil {
		t.Error("expected error from Fit with empty X, got nil")
	}

	err = pls.Fit([][]float64{{1.0, 2.0}}, []float64{})
	if err == nil {
		t.Error("expected error from Fit with empty y, got nil")
	}
}

func TestPLS_MoreComponents(t *testing.T) {
	X, y := generateCollinearData(60, 6)

	// 1-component PLS.
	pls1 := NewPLS()
	pls1.NComponents = 1
	if err := pls1.Fit(X, y); err != nil {
		t.Fatalf("PLS (1 comp) Fit failed: %v", err)
	}
	pred1, err := pls1.Predict(X)
	if err != nil {
		t.Fatalf("PLS (1 comp) Predict failed: %v", err)
	}
	mse1 := mse(y, pred1)

	// 4-component PLS (uses most available; p=6 so 4 < 6 is fine).
	pls4 := NewPLS()
	pls4.NComponents = 4
	if err := pls4.Fit(X, y); err != nil {
		t.Fatalf("PLS (4 comp) Fit failed: %v", err)
	}
	pred4, err := pls4.Predict(X)
	if err != nil {
		t.Fatalf("PLS (4 comp) Predict failed: %v", err)
	}
	mse4 := mse(y, pred4)

	// More components should give at least as good a fit (training MSE).
	if mse4 > mse1+1e-6 {
		t.Errorf("PLS with 4 components (MSE=%f) should have <= MSE than 1 component (MSE=%f)", mse4, mse1)
	}
}
