package ml

import (
	"fmt"
	"math"
)

// ElasticNet implements a regularized linear model combining L1 (Lasso) and
// L2 (Ridge) penalties, solved via coordinate descent.
//
// The objective is:
//
//	min_β (1/2n)‖y - Xβ‖²₂ + α·[l1_ratio·‖β‖₁ + 0.5(1-l1_ratio)·‖β‖²₂]
//
// When UseHuber is true, residuals are re-weighted using the Huber loss
// function with threshold Xi.
type ElasticNet struct {
	// L1Ratio controls the mix of L1 and L2 regularization.
	// 0 = pure ridge, 1 = pure lasso. Default 0.5.
	L1Ratio float64

	// Alpha is the regularization strength. Used when AlphaAuto is false.
	// Default 0.1.
	Alpha float64

	// AlphaAuto, when true, performs a simple alpha grid search via 3-fold
	// cross-validation to select the best Alpha. Default true.
	AlphaAuto bool

	// UseHuber, when true, uses Huber loss re-weighting with threshold Xi.
	// Default false.
	UseHuber bool

	// Xi is the Huber loss threshold. Residuals with |r| ≤ Xi use squared
	// loss; residuals with |r| > Xi use linear loss (robust to outliers).
	// Default 0.9.
	Xi float64

	// MaxIter is the maximum number of coordinate descent iterations.
	// Default 1000.
	MaxIter int

	// Tol is the convergence tolerance. The algorithm stops when the
	// maximum change in any coefficient is below Tol. Default 1e-4.
	Tol float64

	// Private fields
	fitIntercept bool
	coef         []float64 // coefficients on standardized scale
	xMean        []float64 // feature means
	xStd         []float64 // feature standard deviations
	yMean        float64   // target mean
	fitted       bool
}

// NewElasticNet creates a new ElasticNet model with sensible defaults.
func NewElasticNet() *ElasticNet {
	return &ElasticNet{
		L1Ratio:      0.5,
		Alpha:        0.1,
		AlphaAuto:    true,
		UseHuber:     false,
		Xi:           0.9,
		MaxIter:      1000,
		Tol:          1e-4,
		fitIntercept: true,
	}
}

// Fit trains the model on feature matrix X (n_samples × n_features) and
// target vector y (n_samples).
func (en *ElasticNet) Fit(X [][]float64, y []float64) error {
	if err := en.validateFitInputs(X, y); err != nil {
		return err
	}

	//nolint:gosec // validated by validateFitInputs
	//nolint:gosec // validated by validateFitInputs
	n, p := len(X), len(X[0])

	// Standardize X and center y.
	en.xMean = make([]float64, p)
	en.xStd = make([]float64, p)
	xs := en.standardizeX(X)

	en.yMean = mean(y)
	yc := make([]float64, n)
	for i := range y {
		yc[i] = y[i] - en.yMean
	}

	// Determine alpha.
	alpha := en.Alpha
	if en.AlphaAuto {
		alpha = en.searchAlpha(xs, yc)
	}
	en.Alpha = alpha

	// Coordinate descent.
	en.coef = make([]float64, p)
	if err := en.coordinateDescent(xs, yc, alpha); err != nil {
		return err
	}

	// Intercept is implicitly stored as yMean; Predict adds it back.
	en.fitted = true
	return nil
}

// Predict returns predictions for the given feature matrix X.
func (en *ElasticNet) Predict(X [][]float64) ([]float64, error) {
	if !en.fitted {
		return nil, fmt.Errorf("ml: model must be fitted before Predict")
	}
	if len(X) == 0 {
		return nil, fmt.Errorf("ml: Predict called with empty X")
	}

	p := len(en.coef)
	if len(X[0]) != p {
		return nil, fmt.Errorf("ml: expected %d features, got %d", p, len(X[0]))
	}

	n := len(X)
	pred := make([]float64, n)
	for i := range n {
		sum := en.yMean
		for j := range p {
			if en.xStd[j] > 0 {
				sum += (X[i][j] - en.xMean[j]) / en.xStd[j] * en.coef[j]
			}
		}
		pred[i] = sum
	}
	return pred, nil
}

// --- private helpers ---

func (en *ElasticNet) validateFitInputs(X [][]float64, y []float64) error {
	if len(X) == 0 || len(y) == 0 {
		return fmt.Errorf("ml: empty data")
	}
	if len(X) != len(y) {
		return fmt.Errorf("ml: X and y have different lengths: %d vs %d", len(X), len(y))
	}
	nCols := len(X[0])
	for i, row := range X {
		if len(row) != nCols {
			return fmt.Errorf("ml: inconsistent feature count at row %d: expected %d, got %d", i, nCols, len(row))
		}
	}
	return nil
}

// standardizeX computes feature means and standard deviations, and returns a
// standardized copy of X where each column has mean 0 and std 1.
// Features with zero variance get std = 1 (no scaling).
func (en *ElasticNet) standardizeX(X [][]float64) [][]float64 {
	//nolint:gosec // validated by validateFitInputs
	n, p := len(X), len(X[0])
	en.xMean = make([]float64, p)
	en.xStd = make([]float64, p)

	for j := range p {
		var sum float64
		for i := range n {
			sum += X[i][j]
		}
		en.xMean[j] = sum / float64(n)

		var ssq float64
		for i := range n {
			d := X[i][j] - en.xMean[j]
			ssq += d * d
		}
		std := math.Sqrt(ssq / float64(n))
		if std < 1e-10 {
			en.xStd[j] = 1.0 // avoid division by zero
		} else {
			en.xStd[j] = std
		}
	}

	xs := make([][]float64, n)
	for i := range n {
		xs[i] = make([]float64, p)
		for j := range p {
			xs[i][j] = (X[i][j] - en.xMean[j]) / en.xStd[j]
		}
	}
	return xs
}

// searchAlpha performs a simple 3-fold cross-validation grid search over
// alpha candidates [0.001, 0.01, 0.1, 1.0, 10.0] and returns the one with
// the lowest MSE.
func (en *ElasticNet) searchAlpha(Xs [][]float64, yc []float64) float64 {
	candidates := []float64{0.001, 0.01, 0.1, 1.0, 10.0}
	bestAlpha := candidates[0]
	bestMSE := math.MaxFloat64

	splitter := &KFoldSplitter{K: 3, Seed: 42}
	n := len(Xs)
	folds := splitter.Split(n)

	for _, alpha := range candidates {
		var totalMSE float64
		var foldCount int

		for _, fold := range folds {
			trainIdx := fold[0]
			valIdx := fold[1]
			if len(trainIdx) == 0 || len(valIdx) == 0 {
				continue
			}

			trainXs := make([][]float64, len(trainIdx))
			trainYc := make([]float64, len(trainIdx))
			for i, idx := range trainIdx {
				trainXs[i] = Xs[idx]
				trainYc[i] = yc[idx]
			}

			valXs := make([][]float64, len(valIdx))
			valYc := make([]float64, len(valIdx))
			for i, idx := range valIdx {
				valXs[i] = Xs[idx]
				valYc[i] = yc[idx]
			}

			p := len(Xs[0])
			beta := make([]float64, p)
			_ = en.fitCD(trainXs, trainYc, alpha, beta)

			var foldMSE float64
			for i := range valXs {
				var pred float64
				for j := range p {
					pred += valXs[i][j] * beta[j]
				}
				diff := pred - valYc[i]
				foldMSE += diff * diff
			}
			totalMSE += foldMSE / float64(len(valXs))
			foldCount++
		}

		avgMSE := totalMSE / float64(foldCount)
		if avgMSE < bestMSE {
			bestMSE = avgMSE
			bestAlpha = alpha
		}
	}
	return bestAlpha
}

// coordinateDescent runs the coordinate descent algorithm on standardized
// data (Xs, yc).
func (en *ElasticNet) coordinateDescent(Xs [][]float64, yc []float64, alpha float64) error {
	return en.fitCD(Xs, yc, alpha, en.coef)
}

// fitCD fits coefficients beta using coordinate descent on standardized data.
func (en *ElasticNet) fitCD(Xs [][]float64, yc []float64, alpha float64, beta []float64) error {
	n, p := len(Xs), len(Xs[0])
	maxIter := en.MaxIter
	tol := en.Tol
	l1Ratio := en.L1Ratio
	useHuber := en.UseHuber
	xi := en.Xi

	for range maxIter {
		maxDelta := 0.0

		for j := range p {
			oldBeta := beta[j]

			// Compute the partial residual r_{-j} = yc - Xs·beta + Xs_j·beta_j
			// and correlation ρ.

			var rho float64
			if useHuber {
				// Re-weight with Huber weights.
				var wSum float64
				for i := range n {
					// Full residual.
					var ri float64
					for k := range p {
						ri += Xs[i][k] * beta[k]
					}
					ri = yc[i] - ri

					// Huber weight.
					absR := math.Abs(ri)
					w := 1.0
					if absR > xi {
						w = xi / absR
					}

					// Weighted partial residual: add back Xs_ij * beta_j.
					rMinusJ := ri + Xs[i][j]*beta[j]
					rho += w * Xs[i][j] * rMinusJ
					wSum += w
				}
				if wSum > 0 {
					rho /= wSum
				}
			} else {
				for i := range n {
					// Full residual.
					var ri float64
					for k := range p {
						ri += Xs[i][k] * beta[k]
					}
					ri = yc[i] - ri

					// Partial residual: add back Xs_ij * beta_j.
					rMinusJ := ri + Xs[i][j]*beta[j]
					rho += Xs[i][j] * rMinusJ
				}
				rho /= float64(n)
			}

			// Apply ElasticNet soft-thresholding.
			penaltyL1 := alpha * l1Ratio
			denom := 1.0 + alpha*(1.0-l1Ratio)
			beta[j] = softThreshold(rho, penaltyL1) / denom

			delta := math.Abs(beta[j] - oldBeta)
			if delta > maxDelta {
				maxDelta = delta
			}
		}

		if maxDelta < tol {
			break
		}
	}
	return nil
}

// softThreshold returns sign(z) * max(|z| - gamma, 0).
func softThreshold(z, gamma float64) float64 {
	if z > gamma {
		return z - gamma
	}
	if z < -gamma {
		return z + gamma
	}
	return 0
}

// mean computes the mean of a slice.
func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}
