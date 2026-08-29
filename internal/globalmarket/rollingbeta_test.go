package globalmarket

import (
	"math"
	"math/rand"
	"testing"
)

func TestRollingBeta_PerfectCorrelation(t *testing.T) {
	rb := NewRollingBeta(60)

	// y = 2x: perfect positive linear relationship
	for i := range 60 {
		x := float64(i) * 0.01
		y := 2.0 * x
		beta, _, r2 := rb.Update(x, y)
		if i >= 5 {
			if math.Abs(beta-2.0) > 0.001 {
				t.Errorf("iteration %d: expected beta≈2.0, got %f", i, beta)
			}
			if math.Abs(r2-1.0) > 0.001 {
				t.Errorf("iteration %d: expected r2≈1.0, got %f", i, r2)
			}
		}
	}

	beta, alpha, r2 := rb.GetCurrent()
	if math.Abs(beta-2.0) > 0.001 {
		t.Errorf("expected beta≈2.0, got %f", beta)
	}
	if math.Abs(alpha-0.0) > 0.001 {
		t.Errorf("expected alpha≈0.0, got %f", alpha)
	}
	if math.Abs(r2-1.0) > 0.001 {
		t.Errorf("expected r2≈1.0, got %f", r2)
	}
	if rb.Observations() != 60 {
		t.Errorf("expected 60 observations, got %d", rb.Observations())
	}
}

func TestRollingBeta_ZeroCorrelation(t *testing.T) {
	rb := NewRollingBeta(60)

	// x and y are independent random series
	src := rand.New(rand.NewSource(42))
	for range 60 {
		x := src.NormFloat64()
		y := src.NormFloat64()
		rb.Update(x, y)
	}

	beta, _, r2 := rb.GetCurrent()
	// With independent normals, beta should be near 0
	if math.Abs(beta) > 0.5 {
		t.Errorf("expected beta≈0 for independent series, got %f", beta)
	}
	if r2 > 0.5 {
		t.Errorf("expected low r2 for independent series, got %f", r2)
	}
}

func TestRollingBeta_NegativeCorrelation(t *testing.T) {
	rb := NewRollingBeta(60)

	// y = -x: perfect negative correlation
	for i := range 60 {
		x := float64(i) * 0.01
		y := -x
		rb.Update(x, y)
	}

	beta, _, r2 := rb.GetCurrent()
	if beta >= 0 {
		t.Errorf("expected negative beta, got %f", beta)
	}
	if math.Abs(beta+1.0) > 0.001 {
		t.Errorf("expected beta≈-1.0, got %f", beta)
	}
	if math.Abs(r2-1.0) > 0.001 {
		t.Errorf("expected r2≈1.0, got %f", r2)
	}
}

func TestRollingBeta_InsufficientData(t *testing.T) {
	rb := NewRollingBeta(60)

	// First 4 observations should return defaults
	for i := range 4 {
		beta, alpha, r2 := rb.Update(float64(i)*0.01, float64(i)*0.02)
		if beta != 1.0 {
			t.Errorf("iteration %d: expected default beta=1.0, got %f", i, beta)
		}
		if alpha != 0.0 {
			t.Errorf("iteration %d: expected default alpha=0.0, got %f", i, alpha)
		}
		if r2 != 0.0 {
			t.Errorf("iteration %d: expected default r2=0.0, got %f", i, r2)
		}
	}

	// 5th observation should compute actual values
	beta, _, _ := rb.Update(0.04, 0.08)
	if math.Abs(beta-2.0) > 0.001 {
		t.Errorf("at n=5: expected beta≈2.0, got %f", beta)
	}
}

func TestRollingBeta_StabilityAtLowVolatility(t *testing.T) {
	rb := NewRollingBeta(60)

	// x is effectively constant (very low variance)
	for i := range 60 {
		x := 1.0 + float64(i)*1e-15
		y := float64(i) * 0.01 // y varies normally
		rb.Update(x, y)
	}

	beta, alpha, r2 := rb.GetCurrent()

	// Should not produce NaN or Inf
	if math.IsNaN(beta) || math.IsInf(beta, 0) {
		t.Errorf("beta should not be NaN/Inf at low volatility, got %f", beta)
	}
	if math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		t.Errorf("alpha should not be NaN/Inf at low volatility, got %f", alpha)
	}
	if math.IsNaN(r2) || math.IsInf(r2, 0) {
		t.Errorf("r2 should not be NaN/Inf at low volatility, got %f", r2)
	}

	// When x variance is very low, we expect the den ≤ 0 guard to trigger,
	// returning defaults. Accept either the guard kick-in or numerically stable results.
	// The critical property: no NaN/Inf and no crash.
	t.Logf("Low-volatility result: beta=%f, alpha=%f, r2=%f", beta, alpha, r2)
}

func TestRollingBeta_WindowDefaults(t *testing.T) {
	t.Run("explicit window", func(t *testing.T) {
		rb := NewRollingBeta(30)
		if rb.window != 30 {
			t.Errorf("expected window 30, got %d", rb.window)
		}
	})

	t.Run("zero window defaults to 60", func(t *testing.T) {
		rb := NewRollingBeta(0)
		if rb.window != 60 {
			t.Errorf("expected window 60, got %d", rb.window)
		}
	})

	t.Run("negative window defaults to 60", func(t *testing.T) {
		rb := NewRollingBeta(-10)
		if rb.window != 60 {
			t.Errorf("expected window 60, got %d", rb.window)
		}
	})
}

func TestRollingBeta_RingBufferWrap(t *testing.T) {
	rb := NewRollingBeta(10)

	// Fill 10 observations with perfect correlation (y=2x)
	for i := range 10 {
		x := float64(i + 1)
		y := 2.0 * x
		rb.Update(x, y)
	}

	beta, _, r2 := rb.GetCurrent()
	if math.Abs(beta-2.0) > 0.001 {
		t.Fatalf("pre-wrap: expected beta≈2.0, got %f", beta)
	}
	if math.Abs(r2-1.0) > 0.001 {
		t.Fatalf("pre-wrap: expected r2≈1.0, got %f", r2)
	}

	// Wrap: add new observations that maintain the same relationship
	for i := 10; i < 20; i++ {
		x := float64(i + 1)
		y := 2.0 * x
		rb.Update(x, y)
	}

	beta, _, r2 = rb.GetCurrent()
	if math.Abs(beta-2.0) > 0.001 {
		t.Errorf("post-wrap: expected beta≈2.0, got %f", beta)
	}
	if math.Abs(r2-1.0) > 0.001 {
		t.Errorf("post-wrap: expected r2≈1.0, got %f", r2)
	}
	if rb.Observations() != 10 {
		t.Errorf("expected 10 observations post-wrap, got %d", rb.Observations())
	}
}

func TestRollingBeta_NaNHandling(t *testing.T) {
	rb := NewRollingBeta(60)

	// Feed valid data first
	for i := range 5 {
		rb.Update(float64(i)*0.01, float64(i)*0.02)
	}

	// Feed NaN values
	rb.Update(math.NaN(), 0.01)
	rb.Update(0.02, math.NaN())

	beta, alpha, r2 := rb.GetCurrent()
	if math.IsNaN(beta) || math.IsInf(beta, 0) {
		t.Errorf("beta should not be NaN/Inf after NaN input, got %f", beta)
	}
	if math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		t.Errorf("alpha should not be NaN/Inf after NaN input, got %f", alpha)
	}
	if math.IsNaN(r2) || math.IsInf(r2, 0) {
		t.Errorf("r2 should not be NaN/Inf after NaN input, got %f", r2)
	}
	t.Logf("NaN-handling result: beta=%f, alpha=%f, r2=%f", beta, alpha, r2)
}

func TestRollingBeta_InfHandling(t *testing.T) {
	rb := NewRollingBeta(60)

	// Feed valid data first
	for i := range 5 {
		rb.Update(float64(i)*0.01, float64(i)*0.02)
	}

	rb.Update(math.Inf(1), 0.01)
	rb.Update(0.02, math.Inf(-1))

	beta, alpha, r2 := rb.GetCurrent()
	if math.IsNaN(beta) || math.IsInf(beta, 0) {
		t.Errorf("beta should not be NaN/Inf after Inf input, got %f", beta)
	}
	if math.IsNaN(alpha) || math.IsInf(alpha, 0) {
		t.Errorf("alpha should not be NaN/Inf after Inf input, got %f", alpha)
	}
	if math.IsNaN(r2) || math.IsInf(r2, 0) {
		t.Errorf("r2 should not be NaN/Inf after Inf input, got %f", r2)
	}
	t.Logf("Inf-handling result: beta=%f, alpha=%f, r2=%f", beta, alpha, r2)
}

func TestRollingBeta_Observations(t *testing.T) {
	rb := NewRollingBeta(60)

	if obs := rb.Observations(); obs != 0 {
		t.Errorf("expected 0 observations initially, got %d", obs)
	}

	rb.Update(0.01, 0.02)
	if obs := rb.Observations(); obs != 1 {
		t.Errorf("expected 1 observation, got %d", obs)
	}

	for i := 1; i < 10; i++ {
		rb.Update(float64(i)*0.01, float64(i)*0.02)
	}
	if obs := rb.Observations(); obs != 10 {
		t.Errorf("expected 10 observations, got %d", obs)
	}
}

func TestRollingBeta_GetCurrentEmpty(t *testing.T) {
	rb := NewRollingBeta(60)
	beta, alpha, r2 := rb.GetCurrent()
	// Before any Update, values should be zero-valued
	if beta != 0.0 {
		t.Errorf("expected beta=0 for empty tracker, got %f", beta)
	}
	if alpha != 0.0 {
		t.Errorf("expected alpha=0 for empty tracker, got %f", alpha)
	}
	if r2 != 0.0 {
		t.Errorf("expected r2=0 for empty tracker, got %f", r2)
	}
}
