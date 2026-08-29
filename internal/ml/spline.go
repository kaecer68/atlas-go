package ml

import (
	"fmt"
	"math"
	"sort"

	"gonum.org/v1/gonum/mat"
)

// GLMSpline fits a Generalized Linear Model with natural cubic spline basis
// expansion on selected features. It satisfies the Model interface.
//
// For each selected feature, the spline basis expands 1 column into Degree+1
// columns using truncated power basis functions with knots at data quantiles.
// The model then fits via normal equations (Gaussian) or IRLS (Poisson/Gamma).
type GLMSpline struct {
	// Degree is the number of basis functions per splined feature (default 3).
	Degree int
	// Features lists which feature indices to apply spline expansion to.
	// If nil, all features are expanded.
	Features []int
	// Family specifies the GLM family: "gaussian", "poisson", or "gamma".
	Family string
	// FitIntercept prepends an intercept column (default true).
	FitIntercept bool

	// internal state
	nFeatures     int
	knotPositions [][]float64 // per-feature knot positions (only for splined features)
	coeffs        []float64
	fitted        bool
}

// NewGLMSpline returns a new GLMSpline with sensible defaults.
func NewGLMSpline() *GLMSpline {
	return &GLMSpline{
		Degree:       3,
		Family:       "gaussian",
		FitIntercept: true,
	}
}

// splineBasis expands a single value x into Degree+1 basis values using
// truncated power basis with the given interior knots.
//
// Basis functions: B1(x)=1, B2(x)=x, B_{j+2}(x)=max(0, x-k_j)^3.
func splineBasis(x float64, knots []float64) []float64 {
	basis := make([]float64, len(knots)+2)
	basis[0] = 1.0
	basis[1] = x
	for j, k := range knots {
		d := x - k
		if d > 0 {
			basis[j+2] = d * d * d
		}
	}
	return basis
}

// computeKnots returns interior knot positions as evenly spaced quantiles
// of the data. For degree d, we need d-1 interior knots.
func computeKnots(vals []float64, degree int) []float64 {
	if degree < 3 {
		return nil // degree 2 = linear + intercept, no knots needed.
	}
	nKnots := degree - 1
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)

	knots := make([]float64, nKnots)
	for i := range nKnots {
		// Quantile positions: equally spaced from 1/(nKnots+1) to nKnots/(nKnots+1).
		frac := float64(i+1) / float64(nKnots+1)
		pos := frac * float64(len(sorted)-1)
		lo := int(math.Floor(pos))
		hi := min(lo+1, len(sorted)-1)
		// Linear interpolation.
		if lo == hi {
			knots[i] = sorted[lo]
		} else {
			knots[i] = sorted[lo] + (pos-float64(lo))*(sorted[hi]-sorted[lo])
		}
	}
	return knots
}

// expandFeature extracts the values of a single feature across all samples.
func expandFeature(X [][]float64, featIdx int) []float64 {
	vals := make([]float64, len(X))
	for i, row := range X {
		vals[i] = row[featIdx]
	}
	return vals
}

// buildDesignMatrix constructs the expanded design matrix with spline basis
// functions. Returns the flat matrix data, number of rows, and number of cols.
func (g *GLMSpline) buildDesignMatrix(X [][]float64) ([]float64, int, int) {
	nSamples := len(X)
	nOrig := len(X[0])

	// Determine which features get spline expansion.
	expandSet := make(map[int]bool)
	if g.Features == nil {
		for j := range nOrig {
			expandSet[j] = true
		}
	} else {
		for _, j := range g.Features {
			if j >= 0 && j < nOrig {
				expandSet[j] = true
			}
		}
	}

	// Count total columns: for each expanded feature, Degree+1; for others, 1.
	nCols := 0
	if g.FitIntercept {
		nCols++
	}
	for j := range nOrig {
		if expandSet[j] {
			nCols += g.Degree + 1
		} else {
			nCols++
		}
	}

	flat := make([]float64, nSamples*nCols)
	for i := range nSamples {
		col := 0
		if g.FitIntercept {
			flat[i*nCols+col] = 1.0
			col++
		}
		for j := range nOrig {
			if expandSet[j] && len(g.knotPositions) > j && g.knotPositions[j] != nil {
				basis := splineBasis(X[i][j], g.knotPositions[j])
				for _, bv := range basis {
					flat[i*nCols+col] = bv
					col++
				}
			} else {
				flat[i*nCols+col] = X[i][j]
				col++
			}
		}
	}
	return flat, nSamples, nCols
}

// Fit trains the GLM spline model.
func (g *GLMSpline) Fit(X [][]float64, y []float64) error {
	nSamples := len(X)
	if nSamples == 0 {
		return fmt.Errorf("spline: Fit called with empty X")
	}
	if len(y) != nSamples {
		return fmt.Errorf("spline: X and y have mismatched lengths: %d vs %d", nSamples, len(y))
	}
	if len(X) == 0 {
		return fmt.Errorf("spline: Fit called with zero features")
	}
	nOrig := len(X[0])
	if nOrig == 0 {
		return fmt.Errorf("spline: Fit called with zero features")
	}
	for _, row := range X {
		if len(row) != nOrig {
			return fmt.Errorf("spline: inconsistent feature counts in X")
		}
	}

	// Determine which features get spline expansion.
	expandSet := make(map[int]bool)
	if g.Features == nil {
		for j := range nOrig {
			expandSet[j] = true
		}
	} else {
		for _, j := range g.Features {
			if j >= 0 && j < nOrig {
				expandSet[j] = true
			}
		}
	}

	// Compute knot positions for each expanded feature.
	g.knotPositions = make([][]float64, nOrig)
	for j := range nOrig {
		if expandSet[j] {
			vals := expandFeature(X, j)
			g.knotPositions[j] = computeKnots(vals, g.Degree)
		}
	}

	// Build design matrix.
	flat, nRows, nCols := g.buildDesignMatrix(X)
	design := mat.NewDense(nRows, nCols, flat)
	yVec := mat.NewVecDense(nSamples, y)

	switch g.Family {
	case "gaussian":
		return g.fitGaussian(design, yVec, nCols)
	case "poisson":
		return g.fitIRLS(design, yVec, nCols, "poisson")
	case "gamma":
		return g.fitIRLS(design, yVec, nCols, "gamma")
	default:
		return fmt.Errorf("spline: unknown family %q", g.Family)
	}
}

// fitGaussian solves via normal equations: β = (XᵀX + λI)⁻¹Xᵀy.
func (g *GLMSpline) fitGaussian(design *mat.Dense, yVec *mat.VecDense, nCols int) error {
	nSamples, _ := design.Dims()
	ridge := 1e-8

	var xtx mat.Dense
	xtx.Mul(design.T(), design)

	// Add ridge penalty to diagonal.
	for i := range nCols {
		xtx.Set(i, i, xtx.At(i, i)+ridge)
	}

	var xtxInv mat.Dense
	if err := xtxInv.Inverse(&xtx); err != nil {
		return fmt.Errorf("spline: singular design matrix: %w", err)
	}

	var xty mat.VecDense
	xty.MulVec(design.T(), yVec)

	var beta mat.VecDense
	beta.MulVec(&xtxInv, &xty)

	g.coeffs = make([]float64, nCols)
	copy(g.coeffs, beta.RawVector().Data)
	g.nFeatures = nCols
	if g.FitIntercept {
		g.nFeatures--
	}
	g.fitted = true

	_ = nSamples
	return nil
}

// fitIRLS solves via Iteratively Reweighted Least Squares.
func (g *GLMSpline) fitIRLS(design *mat.Dense, yVec *mat.VecDense, nCols int, family string) error {
	nSamples, _ := design.Dims()
	maxIter := 25
	tol := 1e-6
	ridge := 1e-8

	// Initialize coefficients to small values.
	beta := make([]float64, nCols)
	for i := range beta {
		beta[i] = 0.01
	}

	for iter := range maxIter {
		// Compute η = Xβ.
		betaVec := mat.NewVecDense(nCols, beta)
		var eta mat.VecDense
		eta.MulVec(design, betaVec)

		// Compute weights and working response.
		weights := make([]float64, nSamples)
		z := make([]float64, nSamples)
		for i := range nSamples {
			etaI := eta.AtVec(i)

			switch family {
			case "poisson":
				mu := math.Exp(etaI)
				if mu < 1e-10 {
					mu = 1e-10
				}
				weights[i] = mu
				// Working response: η + (y - mu)/mu.
				z[i] = etaI + (yVec.AtVec(i)-mu)/mu
			case "gamma":
				if math.Abs(etaI) < 1e-10 {
					etaI = 1e-10
				}
				mu := math.Exp(etaI)
				if mu < 1e-10 {
					mu = 1e-10
				}
				// Gamma: weight = 1/(η^2) with log link.
				weights[i] = 1.0 / (etaI * etaI)
				if weights[i] > 1e6 {
					weights[i] = 1e6
				}
				z[i] = etaI + (yVec.AtVec(i)-mu)/mu
			}
		}

		// Weighted least squares: β_new = (XᵀWX + λI)⁻¹XᵀWz.
		var xtwx mat.Dense
		// Build sqrt(W) * X for stability.
		sqrtW := mat.NewDense(nSamples, nSamples, nil)
		for i := range nSamples {
			sqrtW.Set(i, i, math.Sqrt(weights[i]))
		}

		var wx mat.Dense
		wx.Mul(sqrtW, design)

		xtwx.Mul(wx.T(), &wx)

		// Ridge penalty.
		for i := range nCols {
			xtwx.Set(i, i, xtwx.At(i, i)+ridge)
		}

		var xtwxInv mat.Dense
		if err := xtwxInv.Inverse(&xtwx); err != nil {
			return fmt.Errorf("spline: singular weighted design matrix at iter %d: %w", iter, err)
		}

		// XᵀWz.
		wz := mat.NewVecDense(nSamples, nil)
		for i := range nSamples {
			wz.SetVec(i, weights[i]*z[i])
		}
		var xtwz mat.VecDense
		xtwz.MulVec(design.T(), wz)

		// β_new.
		var betaNew mat.VecDense
		betaNew.MulVec(&xtwxInv, &xtwz)

		// Check convergence.
		maxDiff := 0.0
		for i := range nCols {
			diff := math.Abs(betaNew.AtVec(i) - beta[i])
			if diff > maxDiff {
				maxDiff = diff
			}
			beta[i] = betaNew.AtVec(i)
		}

		if maxDiff < tol {
			break
		}
	}

	g.coeffs = make([]float64, nCols)
	copy(g.coeffs, beta)
	g.nFeatures = nCols
	if g.FitIntercept {
		g.nFeatures--
	}
	g.fitted = true

	return nil
}

// Predict computes predictions for the given feature matrix.
func (g *GLMSpline) Predict(X [][]float64) ([]float64, error) {
	if !g.fitted {
		return nil, fmt.Errorf("spline: Predict called before Fit")
	}
	nSamples := len(X)
	if nSamples == 0 {
		return nil, fmt.Errorf("spline: Predict called with empty X")
	}
	nOrig := len(X[0])
	for _, row := range X {
		if len(row) != nOrig {
			return nil, fmt.Errorf("spline: inconsistent feature counts in X")
		}
	}

	flat, nRows, nCols := g.buildDesignMatrix(X)
	if nCols != len(g.coeffs) {
		return nil, fmt.Errorf("spline: dimension mismatch: model expects %d expanded cols, got %d", len(g.coeffs), nCols)
	}

	design := mat.NewDense(nRows, nCols, flat)
	betaVec := mat.NewVecDense(nCols, g.coeffs)

	var predVec mat.VecDense
	predVec.MulVec(design, betaVec)

	if g.Family == "poisson" || g.Family == "gamma" {
		pred := predVec.RawVector().Data
		for i := range pred {
			pred[i] = math.Exp(pred[i])
		}
		return pred, nil
	}

	return predVec.RawVector().Data, nil
}

// FitCV performs k-fold cross-validation over multiple degree values and picks
// the best degree by mean R². After selection, refits on all data with the best degree.
func (g *GLMSpline) FitCV(X [][]float64, y []float64, degrees []int, kFold int) error {
	if len(degrees) == 0 {
		return fmt.Errorf("spline: FitCV called with empty degrees")
	}
	if kFold < 2 {
		kFold = 3
	}

	splitter := &KFoldSplitter{K: kFold, Seed: 42}
	folds := splitter.Split(len(X))

	bestDegree := degrees[0]
	bestR2 := math.Inf(-1)

	for _, degree := range degrees {
		g.Degree = degree
		var totalR2 float64
		validFolds := 0

		for _, fold := range folds {
			trainIdx, valIdx := fold[0], fold[1]
			Xtrain := make([][]float64, len(trainIdx))
			ytrain := make([]float64, len(trainIdx))
			for j, idx := range trainIdx {
				Xtrain[j] = X[idx]
				ytrain[j] = y[idx]
			}
			Xval := make([][]float64, len(valIdx))
			yval := make([]float64, len(valIdx))
			for j, idx := range valIdx {
				Xval[j] = X[idx]
				yval[j] = y[idx]
			}

			// Clone model (reset state for each fold).
			gFold := &GLMSpline{
				Degree:       degree,
				Features:     g.Features,
				Family:       g.Family,
				FitIntercept: g.FitIntercept,
			}
			if err := gFold.Fit(Xtrain, ytrain); err != nil {
				continue
			}
			pred, err := gFold.Predict(Xval)
			if err != nil {
				continue
			}
			r2 := computeR2(yval, pred)
			if !math.IsNaN(r2) {
				totalR2 += r2
				validFolds++
			}
		}

		if validFolds > 0 {
			meanR2 := totalR2 / float64(validFolds)
			if meanR2 > bestR2 {
				bestR2 = meanR2
				bestDegree = degree
			}
		}
	}

	// Refit with best degree on all data.
	g.Degree = bestDegree
	return g.Fit(X, y)
}
