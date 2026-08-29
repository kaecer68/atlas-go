// Package risk: DCC-GARCH bivariate volatility and correlation model.
//
// Implements the Engle (2002) Dynamic Conditional Correlation GARCH model in
// pure Go (no CGO, no external dependencies). The estimator follows the
// standard two-step quasi-maximum-likelihood procedure:
//
//	Stage 1 — fit a univariate GARCH(1,1) to each return series, producing
//	          standardized residuals z_{i,t} = eps_{i,t} / sigma_{i,t}.
//	Stage 2 — fit the DCC parameters (a, b) on the standardized residuals by
//	          maximizing the Gaussian QML under the Q_t recursion.
//
// Numerical safeguards:
//   - All Q_t matrices are perturbed on the diagonal by 1e-8 before any
//     correlation / determinant operation to avoid Cholesky singularity.
//   - Observations whose terms become non-finite are dropped from the
//     likelihood and carried over as the previous-period state.
//   - If the DCC optimizer fails, the model falls back to a constant
//     correlation equal to the sample correlation of the standardized
//     residuals (i.e. a = b = 0).
//
// The package is independent of swarm's GARCHProcess (which is a forward
// simulator, not an MLE estimator) and does not touch any other risk/* type.
package risk

import (
	"errors"
	"fmt"
	"math"
)

const (
	dccEpsilon         = 1e-8
	dccMinObservations = 30
)
const nelderMeadMaxIter = 600

const (
	dccNelderMeadTolX   = 1e-6
	dccNelderMeadTolFun = 1e-7
)

type DCCGARCH struct {
	omegaA, alphaA, betaA float64
	omegaB, alphaB, betaB float64
	dccA, dccB            float64
	meanA, meanB          float64
}

type DCCResult struct {
	SigmaA2   []float64
	SigmaB2   []float64
	Rho       []float64
	StdResidA []float64
	StdResidB []float64
}

func (d *DCCGARCH) Fit(returnsA, returnsB []float64) (*DCCResult, error) {
	if len(returnsA) == 0 || len(returnsB) == 0 {
		return nil, errors.New("dcc-garch: empty return series")
	}
	if len(returnsA) != len(returnsB) {
		return nil, fmt.Errorf("dcc-garch: length mismatch A=%d B=%d", len(returnsA), len(returnsB))
	}
	if len(returnsA) < dccMinObservations {
		return nil, fmt.Errorf("dcc-garch: need at least %d observations, got %d", dccMinObservations, len(returnsA))
	}

	fitA, err := fitGARCH11(returnsA)
	if err != nil {
		return nil, fmt.Errorf("dcc-garch: stage-1 fit A: %w", err)
	}
	fitB, err := fitGARCH11(returnsB)
	if err != nil {
		return nil, fmt.Errorf("dcc-garch: stage-1 fit B: %w", err)
	}

	d.omegaA, d.alphaA, d.betaA = fitA.omega, fitA.alpha, fitA.beta
	d.omegaB, d.alphaB, d.betaB = fitB.omega, fitB.alpha, fitB.beta
	d.meanA = fitA.mean
	d.meanB = fitB.mean

	qbar, err := sampleCorrelationMatrix(fitA.stdResid, fitB.stdResid)
	if err != nil {
		return nil, fmt.Errorf("dcc-garch: sample correlation: %w", err)
	}

	a, b, err := fitDCC(fitA.stdResid, fitB.stdResid, qbar)
	if err != nil {
		a, b = 0, 0
	}
	d.dccA, d.dccB = a, b

	rho, sigma2A, sigma2B := reconstructDCC(
		fitA.stdResid, fitB.stdResid, fitA.sigma2, fitB.sigma2, qbar, a, b,
	)

	return &DCCResult{
		SigmaA2:   sigma2A,
		SigmaB2:   sigma2B,
		Rho:       rho,
		StdResidA: fitA.stdResid,
		StdResidB: fitB.stdResid,
	}, nil
}

type garch11Fit struct {
	omega    float64
	alpha    float64
	beta     float64
	mean     float64
	sigma2   []float64
	stdResid []float64
}

func fitGARCH11(returns []float64) (garch11Fit, error) {
	var fit garch11Fit
	mean := mean64(returns)
	demean := make([]float64, len(returns))
	for i, r := range returns {
		demean[i] = r - mean
	}
	uncond := sampleVariance(demean)
	if uncond <= 0 || math.IsNaN(uncond) {
		return fit, errors.New("non-positive or NaN unconditional variance")
	}
	x0 := []float64{uncond * 0.05, 0.08, 0.85}
	objective := func(xc []float64) float64 {
		omega, alpha, beta := xc[0], xc[1], xc[2]
		if omega <= 0 || alpha < 0 || beta < 0 || alpha+beta >= 1 {
			return math.Inf(1)
		}
		negLogLike, ok := garch11NegLogLikelihood(demean, uncond, omega, alpha, beta)
		if !ok {
			return math.Inf(1)
		}
		return negLogLike
	}
	best, _, err := nelderMead(x0, objective, dccNelderMeadTolX, dccNelderMeadTolFun, nelderMeadMaxIter)
	if err != nil {
		return fitGARCH11Fallback(demean, uncond, mean), nil
	}
	omega, alpha, beta := best[0], best[1], best[2]
	if omega <= 0 || alpha < 0 || beta < 0 || alpha+beta >= 1 {
		return fitGARCH11Fallback(demean, uncond, mean), nil
	}
	sigma2, stdResid := simulateGARCH11(demean, uncond, omega, alpha, beta)
	fit.omega = omega
	fit.alpha = alpha
	fit.beta = beta
	fit.mean = mean
	fit.sigma2 = sigma2
	fit.stdResid = stdResid
	return fit, nil
}

func fitGARCH11Fallback(demean []float64, uncond, mean float64) garch11Fit {
	var fit garch11Fit
	n := len(demean)
	sigma2 := make([]float64, n)
	stdResid := make([]float64, n)
	for i := range sigma2 {
		sigma2[i] = uncond
		if uncond > 0 {
			stdResid[i] = demean[i] / math.Sqrt(uncond)
		}
	}
	fit.mean = mean
	fit.sigma2 = sigma2
	fit.stdResid = stdResid
	fit.omega = uncond
	fit.alpha = 0
	fit.beta = 0
	return fit
}

func garch11NegLogLikelihood(demean []float64, uncond, omega, alpha, beta float64) (float64, bool) {
	sigma2, ok := garch11Recurse(demean, uncond, omega, alpha, beta)
	if !ok {
		return 0, false
	}
	var nll float64
	for i, eps := range demean {
		v := sigma2[i]
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		if math.IsNaN(eps) || math.IsInf(eps, 0) {
			return 0, false
		}
		nll += math.Log(v) + eps*eps/v
	}
	return 0.5 * nll, true
}

func garch11Recurse(demean []float64, uncond, omega, alpha, beta float64) ([]float64, bool) {
	n := len(demean)
	sigma2 := make([]float64, n)
	sigma2[0] = uncond
	for t := 1; t < n; t++ {
		prev := demean[t-1]
		v := omega + alpha*prev*prev + beta*sigma2[t-1]
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return nil, false
		}
		sigma2[t] = v
	}
	return sigma2, true
}

func simulateGARCH11(demean []float64, uncond, omega, alpha, beta float64) ([]float64, []float64) {
	n := len(demean)
	sigma2 := make([]float64, n)
	stdResid := make([]float64, n)
	sigma2[0] = uncond
	if sigma2[0] > 0 {
		stdResid[0] = demean[0] / math.Sqrt(sigma2[0])
	}
	for t := 1; t < n; t++ {
		sigma2[t] = omega + alpha*demean[t-1]*demean[t-1] + beta*sigma2[t-1]
		if sigma2[t] > 0 {
			stdResid[t] = demean[t] / math.Sqrt(sigma2[t])
		}
	}
	return sigma2, stdResid
}

func fitDCC(zA, zB []float64, qbar [2][2]float64) (a, b float64, err error) {
	if len(zA) != len(zB) || len(zA) == 0 {
		return 0, 0, errors.New("dcc: mismatched or empty standardized residuals")
	}
	x0 := []float64{0.05, 0.05}
	objective := func(xc []float64) float64 {
		av, bv := xc[0], xc[1]
		if av < 0 || bv < 0 || av+bv >= 1 {
			return math.Inf(1)
		}
		nll, ok := dccNegLogLikelihood(zA, zB, qbar, av, bv)
		if !ok {
			return math.Inf(1)
		}
		return nll
	}
	best, _, err := nelderMead(x0, objective, dccNelderMeadTolX, dccNelderMeadTolFun, nelderMeadMaxIter)
	if err != nil {
		return 0, 0, err
	}
	av, bv := best[0], best[1]
	if av < 0 || bv < 0 || av+bv >= 1 {
		return 0, 0, errors.New("dcc: optimiser exited the feasible region")
	}
	return av, bv, nil
}

func dccNegLogLikelihood(zA, zB []float64, qbar [2][2]float64, a, b float64) (float64, bool) {
	n := len(zA)
	if len(zB) != n {
		return 0, false
	}
	c := 1.0 - a - b
	if c <= 0 {
		return 0, false
	}
	var qT [2][2]float64
	qT[0][0] = qbar[0][0]
	qT[0][1] = qbar[0][1]
	qT[1][0] = qbar[1][0]
	qT[1][1] = qbar[1][1]
	var nll float64
	for t := range n {
		var zAv, zBv float64
		if t > 0 {
			zAv, zBv = zA[t-1], zB[t-1]
		}
		zzA := zAv * zAv
		zzB := zBv * zBv
		zzAB := zAv * zBv
		qT[0][0] = c*qbar[0][0] + a*zzA + b*qT[0][0]
		qT[0][1] = c*qbar[0][1] + a*zzAB + b*qT[0][1]
		qT[1][0] = qT[0][1]
		qT[1][1] = c*qbar[1][1] + a*zzB + b*qT[1][1]
		qT[0][0] += dccEpsilon
		qT[1][1] += dccEpsilon
		if math.IsNaN(qT[0][0]) || math.IsInf(qT[0][0], 0) ||
			math.IsNaN(qT[1][1]) || math.IsInf(qT[1][1], 0) {
			return 0, false
		}
		d0 := qT[0][0]
		d1 := qT[1][1]
		if d0 <= 0 || d1 <= 0 {
			return 0, false
		}
		rho := qT[0][1] / math.Sqrt(d0*d1)
		if rho > 1 {
			rho = 1
		} else if rho < -1 {
			rho = -1
		}
		qT[1][0] = rho * math.Sqrt(d0*d1)
		denom := 1.0 - rho*rho
		if denom <= 1e-12 {
			continue
		}
		ztA := zA[t]
		ztB := zB[t]
		if math.IsNaN(ztA) || math.IsInf(ztA, 0) ||
			math.IsNaN(ztB) || math.IsInf(ztB, 0) {
			return 0, false
		}
		nll += math.Log(denom) + (ztA*ztA-2*rho*ztA*ztB+ztB*ztB)/denom
	}
	return 0.5 * nll, true
}

func reconstructDCC(zA, zB []float64, sigma2A, sigma2B []float64, qbar [2][2]float64, a, b float64) (rho, outSigma2A, outSigma2B []float64) {
	n := len(zA)
	rho = make([]float64, n)
	outSigma2A = make([]float64, n)
	outSigma2B = make([]float64, n)
	for i := range outSigma2A {
		outSigma2A[i] = sigma2A[i]
		outSigma2B[i] = sigma2B[i]
	}
	if a == 0 && b == 0 {
		rho0 := qbar[0][1]
		for i := range rho {
			rho[i] = rho0
		}
		return rho, outSigma2A, outSigma2B
	}
	c := 1.0 - a - b
	if c <= 0 {
		rho0 := qbar[0][1]
		for i := range rho {
			rho[i] = rho0
		}
		return rho, outSigma2A, outSigma2B
	}
	var qT [2][2]float64
	qT[0][0] = qbar[0][0]
	qT[0][1] = qbar[0][1]
	qT[1][0] = qbar[1][0]
	qT[1][1] = qbar[1][1]
	for t := range n {
		var zAv, zBv float64
		if t > 0 {
			zAv, zBv = zA[t-1], zB[t-1]
		}
		zzA := zAv * zAv
		zzB := zBv * zBv
		zzAB := zAv * zBv
		qT[0][0] = c*qbar[0][0] + a*zzA + b*qT[0][0]
		qT[0][1] = c*qbar[0][1] + a*zzAB + b*qT[0][1]
		qT[1][0] = qT[0][1]
		qT[1][1] = c*qbar[1][1] + a*zzB + b*qT[1][1]
		qT[0][0] += dccEpsilon
		qT[1][1] += dccEpsilon
		d0 := qT[0][0]
		d1 := qT[1][1]
		if d0 <= 0 || d1 <= 0 {
			rho[t] = qbar[0][1]
			continue
		}
		r := qT[0][1] / math.Sqrt(d0*d1)
		if r > 1 {
			r = 1
		} else if r < -1 {
			r = -1
		}
		rho[t] = r
	}
	return rho, outSigma2A, outSigma2B
}

func sampleCorrelationMatrix(zA, zB []float64) ([2][2]float64, error) {
	var q [2][2]float64
	if len(zA) != len(zB) || len(zA) == 0 {
		return q, errors.New("sample correlation: mismatched or empty")
	}
	meanA := mean64(zA)
	meanB := mean64(zB)
	var sA, sB, sAB float64
	for i := range zA {
		dA := zA[i] - meanA
		dB := zB[i] - meanB
		sA += dA * dA
		sB += dB * dB
		sAB += dA * dB
	}
	if sA <= 0 || sB <= 0 {
		return q, errors.New("sample correlation: zero variance")
	}
	denom := math.Sqrt(sA * sB)
	r := sAB / denom
	if r > 1 {
		r = 1
	} else if r < -1 {
		r = -1
	}
	q[0][0] = 1
	q[1][1] = 1
	q[0][1] = r
	q[1][0] = r
	return q, nil
}

func nelderMead(x0 []float64, objective func([]float64) float64, tolX, tolFun float64, maxIter int) ([]float64, float64, error) { //nolint:unparam
	n := len(x0)
	if n < 2 {
		return nil, 0, errors.New("nelder-mead: need at least 2 dimensions")
	}
	if maxIter <= 0 {
		maxIter = nelderMeadMaxIter
	}
	simplex := make([][]float64, n+1)
	simplex[0] = append([]float64(nil), x0...)
	for i := range n {
		v := make([]float64, n)
		copy(v, x0)
		step := 0.05 * math.Abs(x0[i])
		if step < 1e-4 {
			step = 0.05
		}
		v[i] += step
		for k := range 6 {
			fv := objective(v)
			if !math.IsInf(fv, 0) && !math.IsNaN(fv) {
				break
			}
			v[i] = x0[i] - step*float64(k+1)*0.5
		}
		simplex[i+1] = v
	}
	fvals := make([]float64, n+1)
	for i := range simplex {
		fvals[i] = objective(simplex[i])
	}
	const (
		alphaNM = 1.0
		gamma   = 2.0
		rho     = 0.5
		sigma   = 0.5
	)
	sortSimplex(simplex, fvals)
	for iter := 0; iter < maxIter; iter++ {
		if fvals[n]-fvals[0] < tolFun {
			maxSpread := 0.0
			for j := 1; j <= n; j++ {
				d := simplexDist(simplex[0], simplex[j])
				if d > maxSpread {
					maxSpread = d
				}
			}
			if maxSpread < tolX {
				return simplex[0], fvals[0], nil
			}
		}
		c := make([]float64, n)
		for i := range n {
			for j := range n {
				c[j] += simplex[i][j]
			}
		}
		for j := range n {
			c[j] /= float64(n)
		}
		xr := make([]float64, n)
		for j := range n {
			xr[j] = c[j] + alphaNM*(c[j]-simplex[n][j])
		}
		fr := objective(xr)
		if fr < fvals[0] {
			xe := make([]float64, n)
			for j := range n {
				xe[j] = c[j] + gamma*(xr[j]-c[j])
			}
			fe := objective(xe)
			if fe < fr {
				simplex[n] = xe
				fvals[n] = fe
			} else {
				simplex[n] = xr
				fvals[n] = fr
			}
		} else if fr < fvals[n-1] {
			simplex[n] = xr
			fvals[n] = fr
		} else {
			xc := make([]float64, n)
			if fr < fvals[n] {
				for j := range n {
					xc[j] = c[j] + rho*(xr[j]-c[j])
				}
			} else {
				for j := range n {
					xc[j] = c[j] - rho*(simplex[n][j]-c[j])
				}
			}
			fc := objective(xc)
			if fc < math.Min(fr, fvals[n]) {
				simplex[n] = xc
				fvals[n] = fc
			} else {
				best := simplex[0]
				for i := 1; i <= n; i++ {
					for j := range n {
						simplex[i][j] = best[j] + sigma*(simplex[i][j]-best[j])
					}
					fvals[i] = objective(simplex[i])
				}
			}
		}
		sortSimplex(simplex, fvals)
		if simplexCollapse(simplex, tolX) {
			return simplex[0], fvals[0], nil
		}
	}
	return simplex[0], fvals[0], nil
}

func sortSimplex(simplex [][]float64, fvals []float64) {
	n := len(simplex)
	for i := 1; i < n; i++ {
		for j := i; j > 0 && fvals[j] < fvals[j-1]; j-- {
			fvals[j], fvals[j-1] = fvals[j-1], fvals[j]
			simplex[j], simplex[j-1] = simplex[j-1], simplex[j]
		}
	}
}

func simplexDist(a, b []float64) float64 {
	var s float64
	for i := range a {
		d := a[i] - b[i]
		s += d * d
	}
	return math.Sqrt(s)
}

func simplexCollapse(simplex [][]float64, tol float64) bool {
	for i := 1; i < len(simplex); i++ {
		if simplexDist(simplex[0], simplex[i]) > tol {
			return false
		}
	}
	return true
}

func mean64(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var s float64
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func sampleVariance(x []float64) float64 {
	if len(x) < 2 {
		return 0
	}
	m := mean64(x)
	var s float64
	for _, v := range x {
		d := v - m
		s += d * d
	}
	return s / float64(len(x)-1)
}
