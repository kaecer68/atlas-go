package ml

import (
	"fmt"

	"gonum.org/v1/gonum/mat"
)

// OLS implements ordinary least squares linear regression via the
// gonum/mat matrix library. It satisfies the Model interface.
//
// Formula: β = (XᵀX)⁻¹Xᵀy
type OLS struct {
	// FitIntercept controls whether an intercept column (all 1s) is
	// prepended to the design matrix. Default: true.
	FitIntercept bool

	// nFeatures is the number of predictor columns (excluding intercept)
	// seen during Fit. Used to validate Predict input dimensions.
	nFeatures int

	// coeffs stores the fitted coefficients. If FitIntercept is true
	// the first element is the intercept term; the remaining elements
	// correspond to the predictor columns.
	coeffs []float64

	// fitted tracks whether Fit has been called successfully.
	fitted bool
}

// NewOLS returns a new OLS model with FitIntercept set to true.
func NewOLS() *OLS {
	return &OLS{FitIntercept: true}
}

// Fit trains the OLS model on feature matrix X (n_samples × n_features)
// and target vector y (n_samples). Returns an error if X has 0 rows, if
// X and y have mismatched lengths, or if the design matrix is singular.
func (o *OLS) Fit(X [][]float64, y []float64) error {
	nSamples := len(X)
	if nSamples == 0 {
		return fmt.Errorf("ml: Fit called with empty X")
	}
	if len(y) != nSamples {
		return fmt.Errorf("ml: X and y have mismatched lengths: %d vs %d", nSamples, len(y))
	}

	nCols := len(X[0])
	for _, row := range X {
		if len(row) != nCols {
			return fmt.Errorf("ml: inconsistent feature counts in X")
		}
	}
	if nCols == 0 {
		return fmt.Errorf("ml: Fit called with zero features")
	}

	if o.FitIntercept {
		nCols++
	}

	// Build the design matrix in row-major flat storage.
	flat := make([]float64, nSamples*nCols)
	if o.FitIntercept {
		for i := range nSamples {
			flat[i*nCols] = 1.0
			copy(flat[i*nCols+1:], X[i]) //nolint:gosec // G602: bounds validated
		}
	} else {
		for i := range nSamples {
			copy(flat[i*nCols:], X[i]) //nolint:gosec // G602: bounds validated
		}
	}

	design := mat.NewDense(nSamples, nCols, flat)
	yVec := mat.NewVecDense(nSamples, y)

	// XᵀX (nCols × nCols)
	var xtx mat.Dense
	xtx.Mul(design.T(), design)

	// (XᵀX)⁻¹
	var xtxInv mat.Dense
	if err := xtxInv.Inverse(&xtx); err != nil {
		return fmt.Errorf("ml: singular design matrix: %w", err)
	}

	// Xᵀy (nCols × 1)
	var xty mat.VecDense
	xty.MulVec(design.T(), yVec)

	// β = (XᵀX)⁻¹Xᵀy
	var beta mat.VecDense
	beta.MulVec(&xtxInv, &xty)

	o.coeffs = make([]float64, nCols)
	copy(o.coeffs, beta.RawVector().Data)
	o.nFeatures = len(X[0]) //nolint:gosec // G602: nSamples>0 checked at line 41
	o.fitted = true

	return nil
}

// Predict computes predictions y_pred = Xβ for the given feature matrix.
// If FitIntercept is true, an intercept column is prepended automatically.
// Returns an error if the model has not been fitted or if the input
// dimensions do not match the fitted model.
func (o *OLS) Predict(X [][]float64) ([]float64, error) {
	if !o.fitted {
		return nil, fmt.Errorf("ml: Predict called before Fit")
	}

	nSamples := len(X)
	if nSamples == 0 {
		return nil, fmt.Errorf("ml: Predict called with empty X")
	}

	nCols := len(X[0])
	for _, row := range X {
		if len(row) != nCols {
			return nil, fmt.Errorf("ml: inconsistent feature counts in X")
		}
	}

	if nCols != o.nFeatures {
		return nil, fmt.Errorf("ml: dimension mismatch: expected %d features, got %d", o.nFeatures, nCols)
	}

	if o.FitIntercept {
		nCols++
	}

	// Build design matrix.
	flat := make([]float64, nSamples*nCols)
	if o.FitIntercept {
		for i := range nSamples {
			flat[i*nCols] = 1.0
			copy(flat[i*nCols+1:], X[i]) //nolint:gosec // G602: bounds validated
		}
	} else {
		for i := range nSamples {
			copy(flat[i*nCols:], X[i]) //nolint:gosec // G602: bounds validated
		}
	}

	design := mat.NewDense(nSamples, nCols, flat)
	betaVec := mat.NewVecDense(nCols, o.coeffs)

	var predVec mat.VecDense
	predVec.MulVec(design, betaVec)

	return predVec.RawVector().Data, nil
}
