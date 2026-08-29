package portfolio

import (
	"math"
	"math/rand/v2"
	"sort"

	"gonum.org/v1/gonum/mat"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ── Multi-Day Drawdown Simulation (P1) ──
// Uses the covariance matrix from P0-1 (Ledoit-Wolf shrinkage) to generate
// correlated multi-day return paths via Monte Carlo, then computes drawdown
// statistics for stress testing.

// DrawdownResult holds stress-test drawdown metrics for a portfolio.
type DrawdownResult struct {
	MaxDrawdown float64   // worst peak-to-trough across all paths
	VaR95       float64   // 95% Value-at-Risk (5th percentile of terminal return)
	WorstPath   []float64 // cumulative return of the worst-drawdown path
}

// SimulateDrawdown runs a Monte Carlo drawdown simulation.
//
// weights: portfolio weights (from Optimize output).
// volatilityScale: stress multiplier on daily volatility (e.g., 4.0 for VIX=80 vs normal VIX=20).
// numDays: trading days to simulate (e.g., 21 for ~1 month).
// numPaths: number of Monte Carlo paths (e.g., 1000).
//
// Uses the Cholesky decomposition of the shrunken covariance matrix to
// generate correlated normal returns: r_t = L · z_t, z_t ~ N(0, σ²·I).
func (o *Optimizer) SimulateDrawdown(
	weights []weightInfo,
	volatilityScale float64,
	numDays, numPaths int,
) DrawdownResult {
	symbols := make([]string, len(weights))
	for i, w := range weights {
		symbols[i] = w.Symbol
	}

	rm := o.extractReturnMatrix(symbols)
	if rm == nil || len(rm.assets) < 2 {
		return DrawdownResult{}
	}

	sample := o.sampleCov(rm)
	if sample == nil {
		return DrawdownResult{}
	}
	sigma := o.ledoitWolfShrink(rm, sample)

	N := len(rm.assets)

	// Build weight vector aligned with rm.assets order.
	w := make([]float64, N)
	weightBySym := make(map[string]float64, len(weights))
	for _, wi := range weights {
		weightBySym[wi.Symbol] = wi.Weight
	}
	for i, sym := range rm.assets {
		w[i] = weightBySym[sym]
	}

	// Cholesky decomposition of shrunken covariance.
	var chol mat.Cholesky
	if ok := chol.Factorize(sigma); !ok {
		return DrawdownResult{}
	}
	L := mat.NewTriDense(N, false, nil)
	chol.LTo(L)

	rng := rand.New(rand.NewPCG(42, 0))

	z := make([]float64, N)
	r := make([]float64, N)

	terminalReturns := make([]float64, numPaths)
	worstDD := 0.0
	var worstPath []float64

	for p := range numPaths {
		cumulative := 1.0
		peak := 1.0
		pathDD := 0.0
		pathVals := make([]float64, 0, numDays+1)
		pathVals = append(pathVals, 1.0)

		for range numDays {
			// Generate independent standard normals, then scale.
			for i := range N {
				z[i] = boxMuller(rng) * volatilityScale
			}

			// Correlated returns: r = L · z.
			for i := range N {
				var sum float64
				for j := 0; j <= i; j++ {
					// L is lower-triangular; L.At(i,j) accesses row i, col j.
					// Realistically the Cholesky factor from gonum is stored in
					// a dense TriDense; use L.At(i, j) for lower-tri access.
					sum += L.At(i, j) * z[j]
				}
				r[i] = sum
			}

			// Portfolio return = w' · r.
			portRet := 0.0
			for i := range N {
				portRet += w[i] * r[i]
			}

			cumulative *= (1 + portRet)
			pathVals = append(pathVals, cumulative)

			if cumulative > peak {
				peak = cumulative
			}
			dd := (peak - cumulative) / peak
			if dd > pathDD {
				pathDD = dd
			}
		}

		terminalReturns[p] = cumulative - 1.0

		if pathDD > worstDD {
			worstDD = pathDD
			worstPath = pathVals
		}
	}

	// Sort terminal returns for VaR computation.
	sort.Float64s(terminalReturns)
	idx95 := int(0.05 * float64(numPaths))
	vaR95 := -terminalReturns[idx95]

	return DrawdownResult{
		MaxDrawdown: worstDD,
		VaR95:       vaR95,
		WorstPath:   worstPath,
	}
}

// boxMuller generates a standard normal random variate.
func boxMuller(rng *rand.Rand) float64 {
	u1 := rng.Float64()
	u2 := rng.Float64()
	return math.Sqrt(-2*math.Log(max(u1, 1e-10))) * math.Cos(2*math.Pi*u2)
}

// GetCovarianceMatrix returns the Ledoit-Wolf shrunken covariance matrix for the
// given symbols as a plain [][]float64, plus the subset of symbols with sufficient
// history. Returns nil if fewer than 2 symbols have data. Designed for the stress
// test runner (P1) to enable correlated noise generation.
func (o *Optimizer) GetCovarianceMatrix(symbols []string) ([][]float64, []string) {
	rm := o.extractReturnMatrix(symbols)
	if rm == nil || len(rm.assets) < 2 {
		return nil, nil
	}
	sample := o.sampleCov(rm)
	if sample == nil {
		return nil, nil
	}
	sigma := o.ledoitWolfShrink(rm, sample)
	N := len(rm.assets)
	mat := make([][]float64, N)
	for i := range N {
		mat[i] = make([]float64, N)
		for j := range N {
			mat[i][j] = sigma.At(i, j)
		}
	}
	return mat, rm.assets
}

// SimulateDrawdownForMonitoring converts domain.Position to weightInfo and runs
// a standard 21-day, 1000-path Monte Carlo drawdown simulation. Used by the
// orchestrator's session-end hook for monitoring (P3-4).
func (o *Optimizer) SimulateDrawdownForMonitoring(positions []domain.Position, portfolioValue float64) DrawdownResult {
	if portfolioValue <= 0 {
		return DrawdownResult{}
	}
	weights := make([]weightInfo, 0, len(positions))
	for _, p := range positions {
		w := p.MarketValue / portfolioValue
		if w <= 0 {
			continue
		}
		weights = append(weights, weightInfo{
			Symbol: p.Symbol,
			Weight: w,
		})
	}
	if len(weights) == 0 {
		return DrawdownResult{}
	}
	return o.SimulateDrawdown(weights, 1.0, 21, 1000)
}
