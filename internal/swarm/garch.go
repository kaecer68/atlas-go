package swarm

import "math"

// GARCHProcess implements a GARCH(1,1) volatility model.
// sigma_t^2 = omega + alpha * epsilon_{t-1}^2 + beta * sigma_{t-1}^2
//
// Parameters vary by regime to reflect different volatility dynamics:
//   - bull: low persistence (volatility decays quickly)
//   - bear: high sensitivity (volatility spikes on shocks)
//   - high_vol/crisis: extreme behavior
//   - low_vol/complacent: high persistence (volatility stays low)
//   - transition: balanced
type GARCHProcess struct {
	Omega   float64 // baseline (constant term)
	Alpha   float64 // shock sensitivity (ARCH term)
	Beta    float64 // variance persistence (GARCH term)
	SigmaSq float64 // current conditional variance (initialized from scenario.Volatility^2)
	LastEps float64 // last period's shock (for ARCH term)
}

// Advance computes the next period's volatility given the current return shock epsilon.
// Returns the updated standard deviation (sqrt of variance).
// epsilon is the realized shock from the previous period: drift + random * sigma_{t-1}
func (g *GARCHProcess) Advance(epsilon float64) float64 {
	g.SigmaSq = g.Omega + g.Alpha*epsilon*epsilon + g.Beta*g.SigmaSq
	g.LastEps = epsilon
	return math.Sqrt(g.SigmaSq)
}

// CurrentSigma returns the current standard deviation.
func (g *GARCHProcess) CurrentSigma() float64 {
	return math.Sqrt(g.SigmaSq)
}

// GARCHParamsForRegime returns GARCH parameters appropriate for a given market regime.
func GARCHParamsForRegime(regime string) (omega, alpha, beta float64) {
	switch regime {
	case "risk_on":
		// Low persistence — volatility shocks decay within ~10 periods
		// Annualized vol: ~5.0% baseline (0.00001 * 252 ≈ 0.0025 → daily 5%)
		return 0.00001, 0.05, 0.90
	case "risk_off":
		// High sensitivity — volatility spikes sharply, decays faster
		return 0.00005, 0.10, 0.85
	case "crisis":
		// Extreme — high baseline, high sensitivity, low persistence
		return 0.00010, 0.15, 0.80
	case "complacent":
		// High persistence — low baseline, volatility stays clamped
		return 0.000005, 0.03, 0.95
	default:
		// transition / balanced
		return 0.00002, 0.08, 0.88
	}
}

// NewGARCHProcess creates a GARCH process initialized from annualized volatility.
// annualVol is the initial annualized volatility (e.g., 0.15 for 15%).
func NewGARCHProcess(omega, alpha, beta, annualVol float64) *GARCHProcess {
	dailyVol := annualVol / math.Sqrt(252.0)
	return &GARCHProcess{
		Omega:   omega,
		Alpha:   alpha,
		Beta:    beta,
		SigmaSq: dailyVol * dailyVol,
	}
}
