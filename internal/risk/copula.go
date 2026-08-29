// Package risk: copula.go — Archimedean copula parameter estimation and
// tail dependence for stress-period risk modeling.
//
// Two copula families are exposed:
//
//   - Gumbel: captures upper-tail dependence (λ_U > 0 under stress).
//   - Clayton: captures lower-tail dependence (λ_L > 0 in drawdowns).
//
// Both estimators use the Kendall rank-correlation method-of-moments, which
// is consistent, requires no iterative optimization, and degrades gracefully
// when the optimizer is replaced (no MLE fallback is wired in — the Kendall
// estimator IS the moment estimator; if a future MLE path is added, fall
// back to this on non-convergence).
package risk

import (
	"errors"
	"fmt"
	"math"
)

// maxThetaClamp caps θ when Kendall's τ saturates at 1.0 (perfect rank
// concordance). The cap keeps tail-dependence numerics finite while still
// yielding λ → 1 within float64 precision.
const maxThetaClamp = 1e6

// tauZeroEpsilon is the Kendall-τ magnitude below which we refuse to fit
// the Clayton copula, because θ = 2τ/(1-τ) → 0 (independence) — a regime
// Clayton is structurally not defined for.
const tauZeroEpsilon = 1e-9

var (
	errCopulaEmpty          = errors.New("copula: input series is empty")
	errCopulaLengthMismatch = errors.New("copula: u and v must have the same length")
	errCopulaOutOfRange     = errors.New("copula: pseudo-observations must lie in [0,1]")
	errCopulaZeroVariance   = errors.New("copula: zero variance in input series")
	errCopulaTauDegenerate  = errors.New("copula: kendall tau is degenerate (all pairs tied)")
)

// EstimateGumbelTheta estimates the Gumbel copula parameter θ from rank-based
// pseudo-observations via the Kendall-τ method-of-moments.
//
//	u, v: empirical-CDF pseudo-observations, same length, both in [0,1],
//	      with non-zero variance.
//
// The Gumbel–Kendall relation is τ = 1 - 1/θ, so θ = 1/(1-τ) for τ < 1.
// Returns θ >= 1. When τ is within float-precision of 1.0 (perfect
// concordance), θ is clamped to maxThetaClamp; tail-dependence evaluations
// remain numerically valid (λ_U → 1).
func EstimateGumbelTheta(u, v []float64) (float64, error) {
	if err := validatePseudoObservations(u, v); err != nil {
		return 0, fmt.Errorf("gumbel: %w", err)
	}
	tau, err := kendallTau(u, v)
	if err != nil {
		return 0, fmt.Errorf("gumbel: %w", err)
	}
	if 1.0-tau < tauZeroEpsilon {
		return maxThetaClamp, nil
	}
	theta := 1.0 / (1.0 - tau)
	if theta < 1.0 {
		return 1.0, nil
	}
	return theta, nil
}

// GumbelUpperTailDep returns the upper tail dependence coefficient of the
// Gumbel copula for parameter θ >= 1: λ_U = 2 - 2^(1/θ).
//
// For θ ≤ 1 (outside the Gumbel domain or perfect independence), returns 0.
func GumbelUpperTailDep(theta float64) float64 {
	if theta <= 1.0 {
		return 0.0
	}
	return 2.0 - math.Pow(2.0, 1.0/theta)
}

// EstimateClaytonTheta estimates the Clayton copula parameter θ from
// pseudo-observations via the Kendall-τ method-of-moments.
//
// The Clayton–Kendall relation is τ = θ/(θ+2), so θ = 2τ/(1-τ) for τ < 1.
// Returns θ ∈ (-1, 0) ∪ (0, +∞). The θ = 0 (independence) limit is rejected:
// at τ = 0 the inversion is singular and Clayton is structurally undefined.
//
//	τ → -1  → θ → -1   (countermonotone floor)
//	τ →  1  → θ → +∞   (perfect lower-tail dependence, clamped)
//	τ →  0⁺ → θ → 0⁺   (rejected with error to avoid degenerate fit)
func EstimateClaytonTheta(u, v []float64) (float64, error) {
	if err := validatePseudoObservations(u, v); err != nil {
		return 0, fmt.Errorf("clayton: %w", err)
	}
	tau, err := kendallTau(u, v)
	if err != nil {
		return 0, fmt.Errorf("clayton: %w", err)
	}
	if math.Abs(tau) < tauZeroEpsilon {
		return 0, fmt.Errorf("clayton: τ ≈ 0 implies θ = 0 (independence), which is undefined: %w", errCopulaTauDegenerate)
	}
	if tau >= 1.0-tauZeroEpsilon {
		return maxThetaClamp, nil
	}
	theta := 2.0 * tau / (1.0 - tau)
	if theta < -1.0 {
		return -1.0, nil
	}
	return theta, nil
}

// ClaytonLowerTailDep returns the lower tail dependence coefficient of the
// Clayton copula for parameter θ > 0: λ_L = 2^(-1/θ).
//
// For θ ≤ 0, returns 0 (the formula is not defined in that region and the
// copula has no lower-tail dependence there). This convention keeps the
// caller from accidentally reading a stale 1.0 from the negative side.
func ClaytonLowerTailDep(theta float64) float64 {
	if theta <= 0.0 {
		return 0.0
	}
	return math.Pow(2.0, -1.0/theta)
}

func validatePseudoObservations(u, v []float64) error {
	if len(u) == 0 {
		return errCopulaEmpty
	}
	if len(u) != len(v) {
		return errCopulaLengthMismatch
	}
	uMin, uMax := u[0], u[0]
	vMin, vMax := v[0], v[0]
	for i := range u {
		ui, vi := u[i], v[i]
		if ui < 0 || ui > 1 || vi < 0 || vi > 1 {
			return errCopulaOutOfRange
		}
		if math.IsNaN(ui) || math.IsNaN(vi) || math.IsInf(ui, 0) || math.IsInf(vi, 0) {
			return errCopulaOutOfRange
		}
		if ui < uMin {
			uMin = ui
		}
		if ui > uMax {
			uMax = ui
		}
		if vi < vMin {
			vMin = vi
		}
		if vi > vMax {
			vMax = vi
		}
	}
	if uMin == uMax || vMin == vMax {
		return errCopulaZeroVariance
	}
	return nil
}

// kendallTau computes the O(n²) Kendall rank correlation coefficient on
// pseudo-observations. Ties are not expected (u and v are continuous
// empirical-CDF values), so we use the simple τ = (C - D)/(C + D) form
// without tie correction.
func kendallTau(u, v []float64) (float64, error) {
	n := len(u)
	if n < 2 {
		return 0, errCopulaEmpty
	}
	var concordant, discordant int
	for i := 0; i < n-1; i++ {
		ui, vi := u[i], v[i]
		for j := i + 1; j < n; j++ {
			du := ui - u[j]
			dv := vi - v[j]
			if du == 0 || dv == 0 {
				continue
			}
			if (du > 0 && dv > 0) || (du < 0 && dv < 0) {
				concordant++
			} else {
				discordant++
			}
		}
	}
	total := concordant + discordant
	if total == 0 {
		return 0, errCopulaTauDegenerate
	}
	return float64(concordant-discordant) / float64(total), nil
}
