package risk

import (
	"fmt"
	"math"
)

// CausalityResult reports the outcome of a Granger causality test between two time series.
type CausalityResult struct {
	Cause         string  // "X→Y" or "Y→X" or "none" or "bidirectional"
	BestLag       int     // strongest causal direction's optimal lag
	FStatistic    float64 // F-statistic for the strongest direction
	PValue        float64 // p-value for the strongest direction
	Bidirectional bool    // true if both directions are significant
}

// TestGrangerCausality tests whether x Granger-causes y (X→Y) and vice-versa.
// It uses BIC to select the optimal lag length and an F-test for significance.
//
// H0 (restricted):  y[t] = α + Σ βᵢ·y[t-i] + ε[t]
// H1 (unrestricted): y[t] = α + Σ βᵢ·y[t-i] + Σ γⱼ·x[t-j] + ε[t]
func TestGrangerCausality(x, y []float64, maxLag int) (*CausalityResult, error) {
	if len(x) < 50 || len(y) < 50 {
		return nil, fmt.Errorf("insufficient data: need at least 50 observations, got x=%d, y=%d", len(x), len(y))
	}
	if len(x) != len(y) {
		return nil, fmt.Errorf("series length mismatch: x=%d, y=%d", len(x), len(y))
	}
	if maxLag < 1 {
		return nil, fmt.Errorf("maxLag must be >= 1, got %d", maxLag)
	}
	if variance(x) == 0 || variance(y) == 0 {
		return nil, fmt.Errorf("zero variance in input series")
	}

	// Test both directions
	xToY, err := testDirection(x, y, maxLag)
	if err != nil {
		return nil, fmt.Errorf("x→y test: %w", err)
	}

	yToX, err := testDirection(y, x, maxLag)
	if err != nil {
		return nil, fmt.Errorf("y→x test: %w", err)
	}

	const alpha = 0.05
	xSig := xToY.pValue < alpha
	ySig := yToX.pValue < alpha

	res := &CausalityResult{
		Bidirectional: xSig && ySig,
	}

	// Determine strongest direction
	if xSig && ySig {
		res.Cause = "bidirectional"
		// Pick direction with smaller p-value as "primary"
		if xToY.pValue <= yToX.pValue {
			res.BestLag = xToY.bestLag
			res.FStatistic = xToY.fStat
			res.PValue = xToY.pValue
		} else {
			res.BestLag = yToX.bestLag
			res.FStatistic = yToX.fStat
			res.PValue = yToX.pValue
		}
	} else if xSig {
		res.Cause = "X→Y"
		res.BestLag = xToY.bestLag
		res.FStatistic = xToY.fStat
		res.PValue = xToY.pValue
	} else if ySig {
		res.Cause = "Y→X"
		res.BestLag = yToX.bestLag
		res.FStatistic = yToX.fStat
		res.PValue = yToX.pValue
	} else {
		res.Cause = "none"
		res.BestLag = xToY.bestLag // arbitrary, use x→y selection
		res.FStatistic = xToY.fStat
		res.PValue = xToY.pValue
	}

	return res, nil
}

// directionResult holds the test outcome for a single causal direction.
type directionResult struct {
	bestLag int
	fStat   float64
	pValue  float64
}

// testDirection tests whether cause Granger-causes effect.
func testDirection(cause, effect []float64, maxLag int) (directionResult, error) { //nolint:unparam
	bestLag := 1
	bestBIC := math.Inf(1)
	var bestFStat, bestPVal float64

	// Search over lag lengths using BIC
	for lag := 1; lag <= maxLag; lag++ {
		n := len(effect) - lag
		if n <= 0 {
			continue
		}

		// Build unrestricted design matrix: [const, lagged effect, lagged cause]
		kUnrestricted := 1 + 2*lag // intercept + effect lags + cause lags
		X := make([][]float64, n)
		Y := make([]float64, n)
		for t := range n {
			row := make([]float64, kUnrestricted)
			row[0] = 1.0 // intercept
			for i := 1; i <= lag; i++ {
				row[i] = effect[t+lag-i]
			}
			for j := 1; j <= lag; j++ {
				row[lag+j] = cause[t+lag-j]
			}
			X[t] = row
			Y[t] = effect[t+lag]
		}

		// Restricted design: [const, lagged effect]
		kRestricted := 1 + lag
		Xr := make([][]float64, n)
		for t := range n {
			row := make([]float64, kRestricted)
			row[0] = 1.0
			for i := 1; i <= lag; i++ {
				row[i] = effect[t+lag-i]
			}
			Xr[t] = row
		}

		// Fit both models
		_, rssU, err := olsFit(X, Y)
		if err != nil {
			continue // skip problematic lag
		}
		_, rssR, err := olsFit(Xr, Y)
		if err != nil {
			continue
		}

		// BIC = n·ln(RSS/n) + k·ln(n)
		bic := float64(n)*math.Log(rssU/float64(n)) + float64(kUnrestricted)*math.Log(float64(n))
		if bic < bestBIC {
			bestBIC = bic
			bestLag = lag

			// F-statistic for H0: all cause coefficients = 0
			numConstraints := float64(lag)
			df1 := numConstraints
			df2 := float64(n - kUnrestricted)
			if df2 <= 0 || rssU <= 0 {
				bestFStat = 0
				bestPVal = 1
				continue
			}
			bestFStat = ((rssR - rssU) / numConstraints) / (rssU / df2)
			bestPVal = fCDFUpper(bestFStat, df1, df2)
		}
	}

	return directionResult{
		bestLag: bestLag,
		fStat:   bestFStat,
		pValue:  bestPVal,
	}, nil
}

// olsFit performs ordinary least squares regression using the normal equations.
// Returns coefficients, residual sum of squares, and error.
func olsFit(X [][]float64, y []float64) ([]float64, float64, error) { //nolint:unparam
	n := len(X)
	if n == 0 || len(y) != n {
		return nil, 0, fmt.Errorf("invalid dimensions")
	}
	k := len(X[0])

	// X'X
	xtx := make([][]float64, k)
	for i := range xtx {
		xtx[i] = make([]float64, k)
	}
	for i := range k {
		for j := 0; j <= i; j++ {
			sum := 0.0
			for t := range n {
				sum += X[t][i] * X[t][j]
			}
			xtx[i][j] = sum
			if i != j {
				xtx[j][i] = sum
			}
		}
	}

	// X'y
	xty := make([]float64, k)
	for i := range k {
		sum := 0.0
		for t := range n {
			sum += X[t][i] * y[t]
		}
		xty[i] = sum
	}

	// Add ridge penalty for numerical stability if near-singular
	for i := range k {
		xtx[i][i] += 1e-8
	}

	// Solve (X'X)β = X'y using Gaussian elimination
	beta, err := solveLinearSystem(xtx, xty)
	if err != nil {
		return nil, 0, err
	}

	// Compute RSS
	rss := 0.0
	for t := range n {
		pred := 0.0
		for i := range k {
			pred += X[t][i] * beta[i]
		}
		resid := y[t] - pred
		rss += resid * resid
	}

	return beta, rss, nil
}

// solveLinearSystem solves Ax = b using Gaussian elimination with partial pivoting.
func solveLinearSystem(A [][]float64, b []float64) ([]float64, error) {
	n := len(A)
	if n == 0 || len(b) != n {
		return nil, fmt.Errorf("invalid dimensions")
	}

	// Build augmented matrix
	aug := make([][]float64, n)
	for i := range aug {
		aug[i] = make([]float64, n+1)
		copy(aug[i], A[i])
		aug[i][n] = b[i]
	}

	// Gaussian elimination with partial pivoting
	for col := range n {
		// Find pivot
		maxRow := col
		maxVal := math.Abs(aug[col][col])
		for row := col + 1; row < n; row++ {
			if v := math.Abs(aug[row][col]); v > maxVal {
				maxVal = v
				maxRow = row
			}
		}
		if maxVal < 1e-12 {
			return nil, fmt.Errorf("singular or near-singular matrix")
		}
		aug[col], aug[maxRow] = aug[maxRow], aug[col]

		// Eliminate below
		for row := col + 1; row < n; row++ {
			factor := aug[row][col] / aug[col][col]
			for j := col; j <= n; j++ {
				aug[row][j] -= factor * aug[col][j]
			}
		}
	}

	// Back substitution
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		x[i] = aug[i][n]
		for j := i + 1; j < n; j++ {
			x[i] -= aug[i][j] * x[j]
		}
		x[i] /= aug[i][i]
	}

	return x, nil
}

// variance computes the sample variance of a slice.
func variance(data []float64) float64 {
	if len(data) < 2 {
		return 0
	}
	mean := 0.0
	for _, v := range data {
		mean += v
	}
	mean /= float64(len(data))

	sumSq := 0.0
	for _, v := range data {
		d := v - mean
		sumSq += d * d
	}
	return sumSq / float64(len(data)-1)
}

// fCDFUpper computes the upper tail probability P(F > f) for an F-distribution
// with df1, df2 degrees of freedom using the regularized incomplete beta function.
func fCDFUpper(f, df1, df2 float64) float64 {
	if f <= 0 {
		return 1.0
	}
	if math.IsInf(f, 1) {
		return 0.0
	}
	// P(F > f) = I_{df2/(df2+df1·f)}(df2/2, df1/2)
	x := df2 / (df2 + df1*f)
	a := df2 / 2.0
	b := df1 / 2.0
	return regularizedIncompleteBeta(x, a, b)
}

// regularizedIncompleteBeta computes I_x(a,b) using continued fraction representation.
func regularizedIncompleteBeta(x, a, b float64) float64 {
	if x < 0 || x > 1 {
		return 0
	}
	if x == 0 || x == 1 {
		return x
	}
	if a <= 0 || b <= 0 {
		return 0
	}

	// Use symmetry: I_x(a,b) = 1 - I_{1-x}(b,a)
	if x > (a+1.0)/(a+b+2.0) {
		return 1.0 - regularizedIncompleteBeta(1.0-x, b, a)
	}

	// I_x(a,b) = (x^a · (1-x)^b / (a·B(a,b))) · (1 / continued_fraction)
	lnBeta := lnbeta(a, b)
	front := math.Exp(a*math.Log(x)+b*math.Log(1.0-x)-lnBeta) / a

	cf := betaContinuedFraction(x, a, b)
	return front / cf
}

// lnbeta computes ln(B(a,b)) = ln(Γ(a)) + ln(Γ(b)) - ln(Γ(a+b)).
func lnbeta(a, b float64) float64 {
	la, _ := math.Lgamma(a)
	lb, _ := math.Lgamma(b)
	lab, _ := math.Lgamma(a + b)
	return la + lb - lab
}

// betaContinuedFraction evaluates the continued fraction for the incomplete beta function.
func betaContinuedFraction(x, a, b float64) float64 {
	const maxIter = 200
	const epsilon = 1e-14

	am := 1.0
	bm := 1.0
	az := 1.0
	qab := a + b
	qap := a + 1.0
	qam := a - 1.0
	bz := 1.0 - qab*x/qap

	for m := 1; m <= maxIter; m++ {
		fm := float64(m)
		m2 := float64(2 * m)

		// Even step
		d := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		ap := az + d*am
		bp := bz + d*bm
		d = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		app := ap + d*az
		bpp := bp + d*bz
		aold := az
		am = ap / bpp
		bm = bp / bpp
		az = app / bpp
		bz = 1.0

		if math.Abs(az-aold) < epsilon*math.Abs(az) {
			return az
		}
	}
	return az
}
