package risk

import (
	"math"
	"testing"
)

// pseudoUniformGrid returns n pseudo-observations on the empirical CDF grid
// i/(n+1) for i = 1..n. These are valid rank-based pseudo-observations.
func pseudoUniformGrid(n int) []float64 {
	u := make([]float64, n)
	for i := range n {
		u[i] = float64(i+1) / float64(n+1)
	}
	return u
}

// lowerTailGriddedV constructs a synthetic v series with strong lower-tail
// dependence on the u grid: v_i = u_i^2 keeps the lower-left corner tight.
func lowerTailGriddedV(u []float64) []float64 {
	v := make([]float64, len(u))
	for i, ui := range u {
		v[i] = ui * ui
	}
	return v
}

// lcgUint64 is a small SplitMix64 generator used only by the test suite to
// produce deterministic pseudo-uniforms without importing math/rand.
type lcgUint64 struct {
	state uint64
}

func newLCG(seed uint64) *lcgUint64 {
	return &lcgUint64{state: seed}
}

func (l *lcgUint64) Next() float64 {
	// splitmix64: right-shift by 11 to retain 53 mantissa bits, then
	// divide by 2^53 to map into [0, 1) exactly.
	l.state += 0x9E3779B97F4A7C15
	z := l.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z = z ^ (z >> 31)
	return float64(z>>11) / float64(1<<53)
}

// TestGumbel_IndependentUniform verifies that two independent uniform series
// produce θ ≈ 1 and λ_U → 0. Uses a deterministic PRNG for reproducibility.
func TestGumbel_IndependentUniform(t *testing.T) {
	const n = 1000
	rng := newLCG(0x1234567890ABCDEF)
	u := make([]float64, n)
	v := make([]float64, n)
	for i := range n {
		u[i] = rng.Next()
		v[i] = rng.Next()
	}

	theta, err := EstimateGumbelTheta(u, v)
	if err != nil {
		t.Fatalf("EstimateGumbelTheta returned error: %v", err)
	}
	// For independent uniforms with n=1000, σ_τ ≈ 0.021, so θ ∈ [0.95, 1.15]
	// with very high probability. Allow generous slack.
	if theta < 0.95 || theta > 1.15 {
		t.Errorf("expected θ ≈ 1.0 for independent uniforms, got θ=%f", theta)
	}
	upper := GumbelUpperTailDep(theta)
	if upper > 0.20 {
		t.Errorf("expected λ_U ≈ 0 for independent uniforms, got λ_U=%f", upper)
	}
}

// TestGumbel_PerfectDependence verifies that u=v implies τ = 1, θ → ∞
// and λ_U → 1.
func TestGumbel_PerfectDependence(t *testing.T) {
	n := 200
	u := pseudoUniformGrid(n)
	v := append([]float64(nil), u...)

	theta, err := EstimateGumbelTheta(u, v)
	if err != nil {
		t.Fatalf("EstimateGumbelTheta returned error: %v", err)
	}
	if theta < 100 {
		t.Errorf("expected θ very large for perfect dependence, got θ=%f", theta)
	}
	upper := GumbelUpperTailDep(theta)
	if upper < 0.99 {
		t.Errorf("expected λ_U ≈ 1 for perfect dependence, got λ_U=%f", upper)
	}
}

// TestClayton_LowerTailOnly verifies that data with strong lower-tail
// concordance (v = u² concentrates mass in (0,0)) yields a positive
// Clayton θ and λ_L > 0.
func TestClayton_LowerTailOnly(t *testing.T) {
	n := 200
	u := pseudoUniformGrid(n)
	v := lowerTailGriddedV(u)

	theta, err := EstimateClaytonTheta(u, v)
	if err != nil {
		t.Fatalf("EstimateClaytonTheta returned error: %v", err)
	}
	if theta <= 0 {
		t.Errorf("expected θ > 0 for lower-tail-concordant data, got θ=%f", theta)
	}
	lower := ClaytonLowerTailDep(theta)
	if lower <= 0 {
		t.Errorf("expected λ_L > 0 for θ > 0, got λ_L=%f", lower)
	}
	if lower >= 1.0 {
		t.Errorf("λ_L must be < 1, got λ_L=%f", lower)
	}
}

// TestClayton_NoUpperTail verifies the documented property of the Clayton
// copula: λ_L = 0 at θ = 0 (independence limit) and λ_L grows monotonically
// for θ > 0. The upper tail of Clayton is 0 by construction (Clayton has no
// upper-tail dependence); this test pins the formula so a future regression
// to the symmetric Gumbel formula would be caught.
func TestClayton_NoUpperTail(t *testing.T) {
	if got := ClaytonLowerTailDep(0); got != 0 {
		t.Errorf("ClaytonLowerTailDep(0) should be 0, got %f", got)
	}
	if got := ClaytonLowerTailDep(-0.5); got != 0 {
		t.Errorf("ClaytonLowerTailDep(negative) should be 0, got %f", got)
	}
	prev := 0.0
	for _, theta := range []float64{0.5, 1, 2, 4, 8, 16} {
		got := ClaytonLowerTailDep(theta)
		if got <= prev {
			t.Errorf("ClaytonLowerTailDep(%g)=%f should exceed previous %f", theta, got, prev)
		}
		prev = got
	}
	// Closed-form sanity: θ=2 → 2^(-1/2) = √2/2.
	if math.Abs(ClaytonLowerTailDep(2)-math.Sqrt2/2.0) > 1e-12 {
		t.Errorf("ClaytonLowerTailDep(2) closed form failed, got %f", ClaytonLowerTailDep(2))
	}
}

// TestKendallTau_Convergence verifies that τ estimates are stable across
// repeated independent samples. For n=2000, asymptotic σ_τ ≈ 0.015; we
// require |τ| < 0.10 (≈ 6σ) across all 30 runs, which is virtually
// guaranteed and exercises both the estimator and the LCG stream.
func TestKendallTau_Convergence(t *testing.T) {
	const (
		nRuns  = 30
		n      = 2000
		maxAbs = 0.10
	)
	rng := newLCG(0xCAFEBABEDEADBEEF)
	for run := range nRuns {
		u := make([]float64, n)
		v := make([]float64, n)
		for i := range n {
			u[i] = rng.Next()
			v[i] = rng.Next()
		}
		theta, err := EstimateGumbelTheta(u, v)
		if err != nil {
			t.Fatalf("run %d: EstimateGumbelTheta error: %v", run, err)
		}
		// θ = 1/(1-τ) → τ = 1 - 1/θ. For independent data we expect τ ≈ 0.
		tau := 1.0 - 1.0/theta
		if math.Abs(tau) > maxAbs {
			t.Errorf("run %d: |τ|=%f exceeds %f", run, math.Abs(tau), maxAbs)
		}
	}
}
