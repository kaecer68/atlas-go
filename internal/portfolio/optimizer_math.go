package portfolio

import (
	"gonum.org/v1/gonum/mat"
)

//
// Data Dependencies
// =========================================================
// | Data               | Source                         | Computation          |
// |--------------------|--------------------------------|----------------------|
// | Daily price series | HistoricalPrices.GetCloseSeries | (Pₜ-Pₜ₋₁)/Pₜ₋₁     |
// | Factor scores      | FactorEngine                   | Momentum/Value/Quality|
// | Constraints        | configs/parameters.json         | w_max, cash_reserve  |
//
// Parameter Provenance
// =========================================================
// | Parameter    | Value  | Source                                        |
// |-------------|--------|-----------------------------------------------|
// | lookbackDays | 60     | Academic consensus (60-120 days for covariance)|
// | riskFreeRate | 0.015  | Taiwan 1Y government bond (≈1.5% p.a.)        |
// | w_max        | 0.15   | Constraints.MaxPositionPct (config-driven)     |
// | QP maxIter   | 100    | Standard active-set convergence bound          |
// | QP tol       | 1e-10  | Numerical stability for KKT optimality check   |

type returnMatrix struct {
	assets  []string
	returns [][]float64 // returns[t][i], T rows × N cols
	means   []float64
}

func (o *Optimizer) extractReturnMatrix(symbols []string) *returnMatrix {
	const minDays = 20
	o.mu.RLock()
	hp := o.history
	lookback := o.lookbackDays
	o.mu.RUnlock()

	if hp == nil {
		return nil
	}

	series := make([][]float64, 0, len(symbols))
	valid := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		prices := hp.GetCloseSeries(sym)
		if len(prices) < minDays+1 {
			continue
		}
		n := len(prices)
		start := max(n-lookback-1, 0)
		window := prices[start:]
		if len(window) < minDays+1 {
			continue
		}
		series = append(series, window)
		valid = append(valid, sym)
	}

	N := len(valid)
	if N < 2 {
		return nil
	}

	T := len(series[0])
	for _, s := range series {
		if len(s) < T {
			T = len(s)
		}
	}
	if T < minDays+1 {
		return nil
	}

	Tret := T - 1
	ret := make([][]float64, Tret)
	for t := range Tret {
		ret[t] = make([]float64, N)
	}
	means := make([]float64, N)

	for i := range N {
		s := series[i]
		offset := len(s) - T
		sum := 0.0
		for t := range Tret {
			prev := s[offset+t]
			curr := s[offset+t+1]
			if prev == 0 {
				continue
			}
			r := curr/prev - 1
			ret[t][i] = r
			sum += r
		}
		means[i] = sum / float64(Tret)
	}

	return &returnMatrix{assets: valid, returns: ret, means: means}
}

func (o *Optimizer) sampleCov(rm *returnMatrix) *mat.SymDense {
	T := float64(len(rm.returns))
	N := len(rm.assets)
	if N == 0 || T == 0 {
		return nil
	}

	cov := mat.NewSymDense(N, nil)
	for i := range N {
		for j := i; j < N; j++ {
			var sum float64
			for t := 0; t < len(rm.returns); t++ {
				dx := rm.returns[t][i] - rm.means[i]
				dy := rm.returns[t][j] - rm.means[j]
				sum += dx * dy
			}
			cov.SetSym(i, j, sum/T)
		}
	}
	return cov
}

// ledoitWolfShrink: Σ_shrink = (1-δ)·S + δ·ν·I  (Ledoit & Wolf, 2004)
func (o *Optimizer) ledoitWolfShrink(rm *returnMatrix, sample *mat.SymDense) *mat.SymDense {
	N := len(rm.assets)
	T := float64(len(rm.returns))
	S := sample

	nu := 0.0
	for i := range N {
		nu += S.At(i, i)
	}
	nu /= float64(N)

	var pi, rho float64
	demeaned := make([][]float64, len(rm.returns))
	for t := 0; t < len(rm.returns); t++ {
		demeaned[t] = make([]float64, N)
		for i := range N {
			demeaned[t][i] = rm.returns[t][i] - rm.means[i]
		}
	}

	for i := range N {
		for j := range N {
			sij := S.At(i, j)
			var ssq float64
			for t := range demeaned {
				d := demeaned[t][i]*demeaned[t][j] - sij
				ssq += d * d
			}
			pij := ssq / T
			pi += pij
			if i == j {
				rho += pij
			}
		}
	}

	var gamma float64
	for i := range N {
		for j := range N {
			target := 0.0
			if i == j {
				target = nu
			}
			d := S.At(i, j) - target
			gamma += d * d
		}
	}

	var delta float64
	if gamma == 0 || T == 0 {
		delta = 0
	} else {
		delta = (pi - rho) / (T * gamma)
		if delta < 0 {
			delta = 0
		}
		if delta > 1 {
			delta = 1
		}
	}

	shrunk := mat.NewSymDense(N, nil)
	for i := range N {
		for j := i; j < N; j++ {
			val := (1 - delta) * S.At(i, j)
			if i == j {
				val += delta * nu
			}
			shrunk.SetSym(i, j, val)
		}
	}
	return shrunk
}

// activeSetQP solves: minimize ½ w' Σ w  s.t. A_eq' w = b_eq, lb ≤ w ≤ ub.
func (o *Optimizer) activeSetQP(sigma *mat.SymDense, Aeq *mat.Dense, beq []float64,
	lb, ub []float64, wInit []float64,
) []float64 {
	N := sigma.SymmetricDim()
	if N == 0 {
		return nil
	}
	mEq := len(beq)

	w := make([]float64, N)
	copy(w, wInit)

	const maxIter = 100
	const tol = 1e-10

	active := make([]bool, N)

	for range maxIter {
		isActive := make([]bool, N)
		for i := range N {
			isActive[i] = active[i] || w[i] <= lb[i]+tol || w[i] >= ub[i]-tol
		}

		freeIdx := make([]int, 0, N)
		for i := range N {
			if !isActive[i] {
				freeIdx = append(freeIdx, i)
			}
		}
		nFree := len(freeIdx)

		if nFree == 0 {
			if o.isOptimal(sigma, w, isActive, lb, ub) {
				return w
			}
			o.releaseConstraint(sigma, w, &active, isActive, lb, ub)
			continue
		}

		// KKT system: [2·Σ_FF  A_eq_F'] [w_F] = [-2·Σ_FA·w_A]
		//             [A_eq_F    0   ] [ λ ]   [b_eq - A_eq_A·w_A]
		kktDim := nFree + mEq
		kkt := mat.NewDense(kktDim, kktDim, nil)

		for p := range nFree {
			for q := p; q < nFree; q++ {
				val := 2 * sigma.At(freeIdx[p], freeIdx[q])
				kkt.Set(p, q, val)
				if p != q {
					kkt.Set(q, p, val)
				}
			}
		}

		for p := range nFree {
			for k := range mEq {
				kkt.Set(p, nFree+k, Aeq.At(k, freeIdx[p]))
				kkt.Set(nFree+k, p, Aeq.At(k, freeIdx[p]))
			}
		}

		rhs := make([]float64, kktDim)
		for p := range nFree {
			var sum float64
			for j := range N {
				if isActive[j] {
					sum += sigma.At(freeIdx[p], j) * w[j]
				}
			}
			rhs[p] = -2 * sum
		}
		for k := range mEq {
			Aw := 0.0
			for j := range N {
				if isActive[j] {
					Aw += Aeq.At(k, j) * w[j]
				}
			}
			rhs[nFree+k] = beq[k] - Aw
		}

		rhsVec := mat.NewVecDense(kktDim, rhs)
		var soln mat.VecDense
		if err := soln.SolveVec(kkt, rhsVec); err != nil {
			return o.gradientProjection(sigma, w, lb, ub)
		}

		wFree := make([]float64, nFree)
		for p := range nFree {
			wFree[p] = soln.AtVec(p)
		}

		d := make([]float64, N)
		for p, fi := range freeIdx {
			d[fi] = wFree[p] - w[fi]
		}

		normD := 0.0
		for _, dv := range d {
			normD += dv * dv
		}
		if normD < tol*tol {
			if o.isOptimal(sigma, w, isActive, lb, ub) {
				return w
			}
			o.releaseConstraint(sigma, w, &active, isActive, lb, ub)
			continue
		}

		alpha := 1.0
		blockingIdx := -1
		for i := range N {
			if d[i] > tol {
				a := (ub[i] - w[i]) / d[i]
				if a < alpha {
					alpha = a
					blockingIdx = i
				}
			} else if d[i] < -tol {
				a := (lb[i] - w[i]) / d[i]
				if a < alpha {
					alpha = a
					blockingIdx = i
				}
			}
		}

		for i := range N {
			w[i] += alpha * d[i]
		}

		if blockingIdx >= 0 {
			active[blockingIdx] = true
		} else {
			if o.isOptimal(sigma, w, isActive, lb, ub) {
				return w
			}
			o.releaseConstraint(sigma, w, &active, isActive, lb, ub)
		}
	}

	return w
}

func (o *Optimizer) isOptimal(sigma *mat.SymDense, w []float64, active []bool, lb, ub []float64) bool {
	const tol = 1e-10
	N := sigma.SymmetricDim()

	for i := range N {
		g := 0.0
		for j := range N {
			g += sigma.At(i, j) * w[j]
		}
		if active[i] && w[i] <= lb[i]+tol && g < -tol {
			return false
		}
		if active[i] && w[i] >= ub[i]-tol && g > tol {
			return false
		}
	}
	return true
}

func (o *Optimizer) releaseConstraint(sigma *mat.SymDense, w []float64,
	active *[]bool, isActive []bool, lb, ub []float64,
) {
	const tol = 1e-10
	N := sigma.SymmetricDim()

	worstIdx := -1
	worstViolation := 0.0

	for i := range N {
		if !isActive[i] {
			continue
		}
		g := 0.0
		for j := range N {
			g += sigma.At(i, j) * w[j]
		}
		if w[i] <= lb[i]+tol && g < -worstViolation {
			worstViolation = -g
			worstIdx = i
		}
		if w[i] >= ub[i]-tol && g > worstViolation {
			worstViolation = g
			worstIdx = i
		}
	}

	if worstIdx >= 0 {
		(*active)[worstIdx] = false
	}
}

func (o *Optimizer) gradientProjection(sigma *mat.SymDense, w, lb, ub []float64) []float64 {
	N := len(w)
	grad := make([]float64, N)
	for i := range N {
		for j := range N {
			grad[i] += sigma.At(i, j) * w[j]
		}
	}

	wNew := make([]float64, N)
	for i := range N {
		wNew[i] = w[i] - 0.5*grad[i]
		if wNew[i] < lb[i] {
			wNew[i] = lb[i]
		}
		if wNew[i] > ub[i] {
			wNew[i] = ub[i]
		}
	}

	sum := 0.0
	for _, wi := range wNew {
		sum += wi
	}
	if sum > 1e-15 {
		for i := range wNew {
			wNew[i] /= sum
		}
	} else {
		for i := range wNew {
			wNew[i] = 1.0 / float64(N)
		}
	}
	return wNew
}
