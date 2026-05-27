package swarm

import (
	"math"
	"math/rand"
)

// CorrelatedShocks generates correlated random shocks for a set of symbols
// using Cholesky decomposition of a correlation matrix.
//
// The correlation strength depends on the regime:
//   - crisis/high_vol: higher correlations (assets crash together)
//   - bull/low_vol: moderate correlations
//   - transition: variable
func CorrelatedShocks(symbols []string, regime string) map[string]float64 {
	n := len(symbols)
	if n == 0 {
		return nil
	}

	rho := correlationForRegime(regime)
	corr := buildCorrelationMatrix(n, rho)
	L := choleskyDecomp(corr)

	normals := make([]float64, n)
	for i := range n {
		normals[i] = rand.NormFloat64()
	}

	shocks := make(map[string]float64, n)
	for i := range n {
		sum := 0.0
		for j := 0; j <= i; j++ {
			sum += L[i][j] * normals[j]
		}
		shocks[symbols[i]] = sum
	}
	return shocks
}

// correlationForRegime returns the base correlation for a given regime.
func correlationForRegime(regime string) float64 {
	switch regime {
	case "crisis":
		return 0.70 // high: systemic sell-off
	case "risk_off":
		return 0.55 // elevated during bear markets
	case "risk_on":
		return 0.35 // moderate during bull markets
	case "complacent":
		return 0.25 // lower in calm periods
	default:
		return 0.40 // transition
	}
}

func buildCorrelationMatrix(n int, rho float64) [][]float64 {
	M := make([][]float64, n)
	for i := range n {
		M[i] = make([]float64, n)
		M[i][i] = 1.0
		for j := 0; j < i; j++ {
			M[i][j] = rho
			M[j][i] = rho
		}
	}
	return M
}

func choleskyDecomp(A [][]float64) [][]float64 {
	n := len(A)
	L := make([][]float64, n)
	for i := range n {
		L[i] = make([]float64, n)
		for j := 0; j <= i; j++ {
			sum := 0.0
			for k := 0; k < j; k++ {
				sum += L[i][k] * L[j][k]
			}
			if i == j {
				val := A[i][i] - sum
				if val < 1e-12 {
					val = 1e-12
				}
				L[i][j] = math.Sqrt(val)
			} else {
				L[i][j] = (A[i][j] - sum) / L[j][j]
			}
		}
	}
	return L
}
