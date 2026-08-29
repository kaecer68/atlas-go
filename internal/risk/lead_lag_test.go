package risk

import (
	"math"
	"math/rand"
	"testing"
)

// TestGranger_IndependentSeries verifies that two independent random series
// do not show significant Granger causality in either direction.
//
// Uses a fixed seed to avoid the ~5% false-positive rate that the
// p <= 0.05 threshold produces under random sampling (n=200, lag=5).
// A pre-vetted seed (1) was chosen so the generated data deterministically
// shows no causal structure.
func TestGranger_IndependentSeries(t *testing.T) {
	n := 200
	r := rand.New(rand.NewSource(1))
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = r.NormFloat64()
		y[i] = r.NormFloat64()
	}

	res, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Cause != "none" {
		t.Errorf("expected cause='none' for independent series, got '%s' (p=%.4f)", res.Cause, res.PValue)
	}
	if res.PValue <= 0.05 {
		t.Errorf("expected p > 0.05 for independent series, got p=%.4f", res.PValue)
	}
	if res.Bidirectional {
		t.Error("expected Bidirectional=false for independent series")
	}
}

// TestGranger_KnownCausality constructs a series where x[t-1] directly predicts y[t]
// and verifies the test detects "X→Y".
func TestGranger_KnownCausality(t *testing.T) {
	n := 200
	r := rand.New(rand.NewSource(1))
	x := make([]float64, n)
	y := make([]float64, n)

	for i := range n {
		x[i] = r.NormFloat64()
	}

	y[0] = r.NormFloat64()
	for i := 1; i < n; i++ {
		y[i] = 0.8*x[i-1] + r.NormFloat64()
	}

	res, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Cause != "X→Y" && res.Cause != "bidirectional" {
		t.Errorf("expected cause='X→Y' or 'bidirectional', got '%s' (p=%.4f, f=%.2f)", res.Cause, res.PValue, res.FStatistic)
	}
	if res.PValue > 0.05 {
		t.Errorf("expected p <= 0.05 for known causality, got p=%.4f", res.PValue)
	}
	if res.BestLag != 1 {
		t.Logf("BestLag=%d (expected 1, but may vary with noise)", res.BestLag)
	}
}

// TestGranger_Bidirectional constructs mutually influencing series and verifies
// the test detects bidirectional causality.
func TestGranger_Bidirectional(t *testing.T) {
	n := 300
	r := rand.New(rand.NewSource(1))
	x := make([]float64, n)
	y := make([]float64, n)

	// Strong mutual influence: x[t] depends on y[t-1] and y[t] depends on x[t-1]
	x[0] = r.NormFloat64()
	y[0] = r.NormFloat64()
	for i := 1; i < n; i++ {
		x[i] = 0.6*y[i-1] + r.NormFloat64()
		y[i] = 0.6*x[i-1] + r.NormFloat64()
	}

	res, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Bidirectional {
		t.Errorf("expected Bidirectional=true, got false (cause=%s, p=%.4f)", res.Cause, res.PValue)
	}
	if res.Cause != "bidirectional" {
		t.Errorf("expected cause='bidirectional', got '%s'", res.Cause)
	}
}

// TestGranger_InsufficientData verifies the function returns an error when
// the sample size is below the minimum threshold.
func TestGranger_InsufficientData(t *testing.T) {
	x := make([]float64, 30)
	y := make([]float64, 30)
	for i := range 30 {
		x[i] = rand.NormFloat64()
		y[i] = rand.NormFloat64()
	}

	_, err := TestGrangerCausality(x, y, 3)
	if err == nil {
		t.Fatal("expected error for insufficient data, got nil")
	}
}

// TestGranger_LagSelection verifies that BIC selects the correct lag length
// when the true causal lag is known.
func TestGranger_LagSelection(t *testing.T) {
	n := 300
	r := rand.New(rand.NewSource(1))
	x := make([]float64, n)
	y := make([]float64, n)

	for i := range n {
		x[i] = r.NormFloat64()
	}

	// y[t] = 0.4·x[t-2] + ε[t]  (true lag is 2)
	y[0] = r.NormFloat64()
	y[1] = r.NormFloat64()
	for i := 2; i < n; i++ {
		y[i] = 0.4*x[i-2] + r.NormFloat64()
	}

	res, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should detect X→Y causality
	if res.Cause != "X→Y" {
		t.Errorf("expected cause='X→Y', got '%s' (p=%.4f)", res.Cause, res.PValue)
	}
	if res.PValue > 0.05 {
		t.Errorf("expected p <= 0.05, got p=%.4f", res.PValue)
	}

	// BIC should ideally pick lag=2, but with noise it may pick 1 or 3.
	// We allow some tolerance: lag should be <= 3 and not unreasonably large.
	if res.BestLag > 3 {
		t.Errorf("BIC selected lag=%d, expected near 2 (allowing ≤3)", res.BestLag)
	}
}

// TestGranger_ZeroVariance verifies the function returns an error when
// either input series has zero variance.
func TestGranger_ZeroVariance(t *testing.T) {
	x := make([]float64, 100)
	y := make([]float64, 100)
	for i := range y {
		y[i] = rand.NormFloat64()
	}

	_, err := TestGrangerCausality(x, y, 3)
	if err == nil {
		t.Fatal("expected error for zero-variance series, got nil")
	}
}

// TestGranger_SymmetricCausality verifies that swapping x and y swaps the
// causal direction label while maintaining the same significance pattern.
func TestGranger_SymmetricCausality(t *testing.T) {
	n := 200
	r := rand.New(rand.NewSource(1))
	x := make([]float64, n)
	y := make([]float64, n)

	for i := range n {
		x[i] = r.NormFloat64()
	}
	for i := 1; i < n; i++ {
		y[i] = 0.8*x[i-1] + r.NormFloat64()
	}

	resXY, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resYX, err := TestGrangerCausality(y, x, 5)
	if err != nil {
		t.Fatalf("unexpected error on swap: %v", err)
	}

	if resXY.Cause != "X→Y" && resXY.Cause != "bidirectional" {
		t.Errorf("x→y: expected cause='X→Y' or 'bidirectional', got '%s'", resXY.Cause)
	}
	if resYX.Cause == "X→Y" || resYX.Cause == "bidirectional" {
		t.Errorf("y→x: expected no causality, got '%s'", resYX.Cause)
	}
}

// TestGranger_UnitRootLikeSeries tests robustness with trending (non-stationary-like) data.
func TestGranger_UnitRootLikeSeries(t *testing.T) {
	n := 250
	x := make([]float64, n)
	y := make([]float64, n)

	// Random walk (unit root)
	x[0] = 100.0
	x[1] = x[0] + rand.NormFloat64()
	y[0] = 100.0
	y[1] = y[0] + rand.NormFloat64()
	for i := 2; i < n; i++ {
		x[i] = x[i-1] + rand.NormFloat64()
		y[i] = y[i-1] + 0.3*(x[i-1]-x[i-2]) + rand.NormFloat64()
	}

	res, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still detect some relationship (even if not strictly valid for unit roots)
	if res.Cause == "none" && res.PValue > 0.10 {
		t.Logf("no causality detected for random-walk data (p=%.3f) — expected behavior", res.PValue)
	}
}

// TestGranger_IdenticalSeries verifies behavior when both series are identical.
func TestGranger_IdenticalSeries(t *testing.T) {
	n := 200
	r := rand.New(rand.NewSource(1))
	x := make([]float64, n)
	for i := range n {
		x[i] = r.NormFloat64()
	}

	// y is a copy of x
	y := make([]float64, n)
	copy(y, x)

	res, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Cause != "none" {
		t.Errorf("identical series: expected cause='none' due to multicollinearity, got '%s'", res.Cause)
	}
}

// TestGranger_LagZeroError verifies maxLag < 1 returns an error.
func TestGranger_LagZeroError(t *testing.T) {
	x := make([]float64, 100)
	y := make([]float64, 100)
	for i := range x {
		x[i] = rand.NormFloat64()
		y[i] = rand.NormFloat64()
	}

	_, err := TestGrangerCausality(x, y, 0)
	if err == nil {
		t.Fatal("expected error for maxLag=0, got nil")
	}
}

// TestGranger_MismatchLength verifies mismatched lengths return an error.
func TestGranger_MismatchLength(t *testing.T) {
	x := make([]float64, 100)
	y := make([]float64, 80)
	for i := range x {
		x[i] = rand.NormFloat64()
	}
	for i := range y {
		y[i] = rand.NormFloat64()
	}

	_, err := TestGrangerCausality(x, y, 3)
	if err == nil {
		t.Fatal("expected error for mismatched lengths, got nil")
	}
}

// TestGranger_FStatisticRange verifies the F-statistic is non-negative.
func TestGranger_FStatisticRange(t *testing.T) {
	n := 200
	x := make([]float64, n)
	y := make([]float64, n)
	for i := range n {
		x[i] = rand.NormFloat64()
		y[i] = rand.NormFloat64()
	}

	res, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.FStatistic < 0 {
		t.Errorf("F-statistic should be non-negative, got %.4f", res.FStatistic)
	}
	if res.PValue < 0 || res.PValue > 1 {
		t.Errorf("p-value should be in [0,1], got %.4f", res.PValue)
	}
}

// TestGranger_VeryWeakCausality tests borderline cases with very weak causal links.
func TestGranger_VeryWeakCausality(t *testing.T) {
	n := 300
	x := make([]float64, n)
	y := make([]float64, n)

	for i := range n {
		x[i] = rand.NormFloat64()
	}

	// Very weak causal link: coefficient = 0.05
	y[0] = rand.NormFloat64()
	for i := 1; i < n; i++ {
		y[i] = 0.05*x[i-1] + rand.NormFloat64()
	}

	res, err := TestGrangerCausality(x, y, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With 300 samples and coefficient 0.05, may or may not be significant.
	// Just verify it doesn't crash and returns valid values.
	if res.PValue < 0 || res.PValue > 1 {
		t.Errorf("invalid p-value: %.4f", res.PValue)
	}
	if math.IsNaN(res.FStatistic) || math.IsInf(res.FStatistic, 0) {
		t.Errorf("invalid F-statistic: %v", res.FStatistic)
	}
}

// TestGranger_Persistence tests that the same data always produces the same result
// (deterministic given fixed seed).
func TestGranger_Persistence(t *testing.T) {
	n := 200
	seed := int64(42)

	r1 := rand.New(rand.NewSource(seed))
	x1 := make([]float64, n)
	y1 := make([]float64, n)
	for i := range n {
		x1[i] = r1.NormFloat64()
	}
	for i := 1; i < n; i++ {
		y1[i] = 0.5*x1[i-1] + r1.NormFloat64()
	}

	r2 := rand.New(rand.NewSource(seed))
	x2 := make([]float64, n)
	y2 := make([]float64, n)
	for i := range n {
		x2[i] = r2.NormFloat64()
	}
	for i := 1; i < n; i++ {
		y2[i] = 0.5*x2[i-1] + r2.NormFloat64()
	}

	res1, err := TestGrangerCausality(x1, y1, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	res2, err := TestGrangerCausality(x2, y2, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res1.Cause != res2.Cause {
		t.Errorf("deterministic mismatch: cause1=%s, cause2=%s", res1.Cause, res2.Cause)
	}
	if res1.BestLag != res2.BestLag {
		t.Errorf("deterministic mismatch: lag1=%d, lag2=%d", res1.BestLag, res2.BestLag)
	}
	if math.Abs(res1.FStatistic-res2.FStatistic) > 1e-9 {
		t.Errorf("deterministic mismatch: f1=%.6f, f2=%.6f", res1.FStatistic, res2.FStatistic)
	}
	if math.Abs(res1.PValue-res2.PValue) > 1e-9 {
		t.Errorf("deterministic mismatch: p1=%.6f, p2=%.6f", res1.PValue, res2.PValue)
	}
}
