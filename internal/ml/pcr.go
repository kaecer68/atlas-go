package ml

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
)

// PCR implements Principal Component Regression via SVD.
// It first performs PCA on the feature matrix to extract orthogonal components,
// then fits ordinary least squares on the principal component scores.
type PCR struct {
	// NComponents is the number of principal components to retain.
	// Default 4. Ignored if VarianceThreshold > 0.
	NComponents int

	// VarianceThreshold, if > 0, auto-selects enough components to explain
	// this fraction of total variance (e.g. 0.9 = 90%). Overrides NComponents.
	VarianceThreshold float64

	// --- internal state (set by Fit) ---

	// nComponentsUsed is the actual number of components retained after fitting.
	nComponentsUsed int

	// xMean and xStd hold per-column mean and standard deviation for standardization.
	xMean []float64
	xStd  []float64

	// yMean is the mean of the target vector (used for centering during Predict).
	yMean float64

	// vk is the projection matrix (n_features × k), first k columns of V from SVD.
	vk *mat.Dense

	// coeff is the OLS coefficients on principal component scores (length k).
	coeff []float64

	fitted bool
}

// NewPCR creates a new PCR model with default parameters (NComponents=4).
func NewPCR() *PCR {
	return &PCR{
		NComponents: 4,
	}
}

// Fit trains the PCR model on feature matrix X (n_samples × n_features) and
// target vector y (n_samples). Returns an error if training fails.
func (p *PCR) Fit(X [][]float64, y []float64) error {
	p.fitted = false
	p.nComponentsUsed = 0

	if len(X) == 0 || len(y) == 0 {
		return fmt.Errorf("ml/pcr: empty data")
	}
	if len(X) != len(y) {
		return fmt.Errorf("ml/pcr: X and y have different lengths: %d vs %d", len(X), len(y))
	}
	nSamples := len(X)
	if nSamples == 0 {
		return fmt.Errorf("ml/pcr: empty data")
	}
	nFeatures := len(X[0])
	if nFeatures == 0 {
		return fmt.Errorf("ml/pcr: zero features")
	}
	for i, row := range X {
		if len(row) != nFeatures {
			return fmt.Errorf("ml/pcr: row %d has %d features, expected %d", i, len(row), nFeatures)
		}
	}

	// 1. Standardize X: compute column mean and std, transform to z-scores.
	p.xMean = make([]float64, nFeatures)
	p.xStd = make([]float64, nFeatures)

	for j := range nFeatures {
		var sum float64
		for i := range nSamples {
			sum += X[i][j]
		}
		p.xMean[j] = sum / float64(nSamples)
	}

	for j := range nFeatures {
		var sumSq float64
		for i := range nSamples {
			diff := X[i][j] - p.xMean[j]
			sumSq += diff * diff
		}
		std := math.Sqrt(sumSq / float64(nSamples))
		if std < 1e-12 {
			std = 1.0 // handle constant column
		}
		p.xStd[j] = std
	}

	// Build standardized X as mat.Dense.
	Xstd := mat.NewDense(nSamples, nFeatures, nil)
	for i := range nSamples {
		for j := range nFeatures {
			Xstd.Set(i, j, (X[i][j]-p.xMean[j])/p.xStd[j])
		}
	}

	// 2. Center y.
	p.yMean = 0
	for i := range nSamples {
		p.yMean += y[i]
	}
	p.yMean /= float64(nSamples)

	yCentered := mat.NewVecDense(nSamples, nil)
	for i := range nSamples {
		yCentered.SetVec(i, y[i]-p.yMean)
	}

	// 3. SVD on standardized X.
	var svd mat.SVD
	if !svd.Factorize(Xstd, mat.SVDFull) {
		return fmt.Errorf("ml/pcr: SVD factorization failed")
	}

	// Get singular values.
	svals := svd.Values(nil)

	// Get V (right singular vectors / loadings).
	var V mat.Dense
	svd.VTo(&V)
	// V is nFeatures × nFeatures, columns are right singular vectors.

	// 4. Determine number of components.
	k := p.NComponents

	if p.VarianceThreshold > 0 {
		// Compute total variance (sum of squared singular values).
		var totalVar float64
		for _, s := range svals {
			totalVar += s * s
		}
		// Select components until cumulative variance exceeds threshold.
		var cumVar float64
		for i, s := range svals {
			cumVar += s * s
			if cumVar/totalVar >= p.VarianceThreshold {
				k = i + 1
				break
			}
		}
		if k > nFeatures {
			k = nFeatures
		}
	}

	// Cap at min(nFeatures, nSamples) and ensure at least 1.
	k = max(1, min(k, nFeatures, nSamples))
	p.nComponentsUsed = k

	// 5. Extract first k columns of V as projection matrix V_k (nFeatures × k).
	p.vk = mat.NewDense(nFeatures, k, nil)
	for i := range nFeatures {
		for j := range k {
			p.vk.Set(i, j, V.At(i, j))
		}
	}

	// 6. Project X onto principal components: Z = X_std * V_k (nSamples × k).
	var Z mat.Dense
	Z.Mul(Xstd, p.vk)

	// 7. Fit OLS on Z: β = (ZᵀZ)⁻¹Zᵀy.
	// Compute ZᵀZ (k × k).
	var ZtZ mat.Dense
	ZtZ.Mul(Z.T(), &Z)

	// Compute (ZᵀZ)⁻¹.
	var ZtZinv mat.Dense
	if err := ZtZinv.Inverse(&ZtZ); err != nil {
		return fmt.Errorf("ml/pcr: ZᵀZ singular, cannot invert: %w", err)
	}

	// Compute Zᵀy (k × 1).
	var Zty mat.VecDense
	Zty.MulVec(Z.T(), yCentered)

	// β = (ZᵀZ)⁻¹ * (Zᵀy).
	var coeffVec mat.VecDense
	coeffVec.MulVec(&ZtZinv, &Zty)

	p.coeff = make([]float64, k)
	for j := range k {
		p.coeff[j] = coeffVec.AtVec(j)
	}

	p.fitted = true
	return nil
}

// Predict returns predictions for the given feature matrix X.
// Returns an error if the model has not been fitted or if X dimensions mismatch.
func (p *PCR) Predict(X [][]float64) ([]float64, error) {
	if !p.fitted {
		return nil, fmt.Errorf("ml/pcr: model not fitted")
	}
	if len(X) == 0 {
		return nil, fmt.Errorf("ml/pcr: empty data")
	}
	nSamples := len(X)
	nFeatures := len(p.xMean)
	if len(X[0]) != nFeatures {
		return nil, fmt.Errorf("ml/pcr: expected %d features, got %d", nFeatures, len(X[0]))
	}
	for i, row := range X {
		if len(row) != nFeatures {
			return nil, fmt.Errorf("ml/pcr: row %d has %d features, expected %d", i, len(row), nFeatures)
		}
	}

	// 1. Standardize X.
	Xstd := mat.NewDense(nSamples, nFeatures, nil)
	for i := range nSamples {
		for j := range nFeatures {
			Xstd.Set(i, j, (X[i][j]-p.xMean[j])/p.xStd[j])
		}
	}

	// 2. Project onto principal components: Z = X_std * V_k.
	var Z mat.Dense
	Z.Mul(Xstd, p.vk)

	// 3. Compute y_pred = Z * β + y_mean.
	k := p.nComponentsUsed
	coeffVec := mat.NewVecDense(k, p.coeff)

	var pred mat.VecDense
	pred.MulVec(&Z, coeffVec)

	// Add back the y mean.
	result := make([]float64, nSamples)
	for i := range nSamples {
		result[i] = pred.AtVec(i) + p.yMean
	}

	return result, nil
}
