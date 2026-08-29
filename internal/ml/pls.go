package ml

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
)

// PLS implements Partial Least Squares regression via the NIPALS algorithm.
// It is designed for regression with a single response variable (PLS1),
// making it well-suited for factor-return prediction where features are
// collinear.
type PLS struct {
	// NComponents is the number of latent components to extract (default 4).
	NComponents int

	// MaxIter is the maximum inner-loop iterations for the NIPALS algorithm
	// (default 100). For the single-response case the weight vector is computed
	// directly, so this is reserved for future multi-response support.
	MaxIter int

	// Tol is the convergence tolerance for inner-loop iterations (default 1e-6).
	Tol float64

	// Preprocessing parameters stored during Fit.
	xMean     []float64 // per-feature mean
	xStd      []float64 // per-feature standard deviation
	yMean     float64   // global response mean
	nFeatures int       // number of features in training data

	// NIPALS result matrices.
	w    *mat.Dense    // p × k  X-weight matrix
	p    *mat.Dense    // p × k  X-loading matrix
	c    *mat.VecDense // k × 1  Y-loading vector
	bPLS *mat.VecDense // p × 1  regression coefficients (original scale)

	fitted bool
}

// NewPLS creates a PLS model with sensible defaults.
func NewPLS() *PLS {
	return &PLS{
		NComponents: 4,
		MaxIter:     100,
		Tol:         1e-6,
	}
}

// Fit trains the PLS model on feature matrix X (n × p) and target vector y (n).
// It implements the NIPALS algorithm for the single-response (PLS1) case.
func (m *PLS) Fit(X [][]float64, y []float64) error {
	n := len(X)
	if n == 0 {
		return fmt.Errorf("ml/pls: empty X")
	}
	if len(y) == 0 {
		return fmt.Errorf("ml/pls: empty y")
	}
	if n != len(y) {
		return fmt.Errorf("ml/pls: X and y length mismatch: %d vs %d", n, len(y))
	}

	p := len(X[0])
	m.nFeatures = p
	for i, row := range X {
		if len(row) != p {
			return fmt.Errorf("ml/pls: row %d has %d features, expected %d", i, len(row), p)
		}
	}

	k := min(min(max(m.NComponents, 1), p), n)

	// 1. Compute per-feature mean and standard deviation.
	m.xMean = make([]float64, p)
	m.xStd = make([]float64, p)
	for j := range p {
		var sum float64
		for i := range n {
			sum += X[i][j] //nolint:gosec // G602: bounds validated at lines 55-70
		}
		m.xMean[j] = sum / float64(n)

		var sqSum float64
		for i := range n {
			diff := X[i][j] - m.xMean[j] //nolint:gosec // G602: bounds validated at lines 55-70
			sqSum += diff * diff
		}
		std := math.Sqrt(sqSum / float64(n))
		if std < 1e-12 {
			std = 1.0 // avoid division by zero for constant features
		}
		m.xStd[j] = std
	}

	// 2. Standardize X into mat.Dense (n × p).
	X0 := mat.NewDense(n, p, nil)
	for i := range n {
		for j := range p {
			X0.Set(i, j, (X[i][j]-m.xMean[j])/m.xStd[j]) //nolint:gosec // G602: bounds validated at lines 55-70
		}
	}

	// 3. Center y.
	var ySum float64
	for _, v := range y {
		ySum += v
	}
	m.yMean = ySum / float64(n)

	y0 := mat.NewVecDense(n, nil)
	for i, v := range y {
		y0.SetVec(i, v-m.yMean)
	}

	// 4. NIPALS loop.
	W := mat.NewDense(p, k, nil)    // p × k weight matrix
	Pmat := mat.NewDense(p, k, nil) // p × k X-loading matrix
	Cvec := mat.NewVecDense(k, nil) // k × 1 Y-loading vector

	extracted := 0
	for range k {
		// (a) w = X0ᵀ * y0, normalized to unit length.
		var wVec mat.VecDense
		wVec.MulVec(X0.T(), y0)
		wNorm := mat.Norm(&wVec, 2)
		if wNorm < 1e-12 {
			// Deflated y0 is effectively zero; no more structure to extract.
			break
		}
		wVec.ScaleVec(1.0/wNorm, &wVec)

		// (b) t = X0 * w   (score vector, n × 1)
		var tVec mat.VecDense
		tVec.MulVec(X0, &wVec)

		// (c) c = tᵀ * y0 / (tᵀ * t)   (Y-loading, scalar)
		tt := mat.Dot(&tVec, &tVec)
		if tt < 1e-12 {
			break
		}
		cScalar := mat.Dot(&tVec, y0) / tt

		// (d) p = X0ᵀ * t / (tᵀ * t)   (X-loading, p × 1)
		var pVec mat.VecDense
		pVec.MulVec(X0.T(), &tVec)
		pVec.ScaleVec(1.0/tt, &pVec)

		// (e) Deflate X0: X0 = X0 - t * pᵀ
		var tpOuter mat.Dense
		tpOuter.Mul(&tVec, pVec.T())
		X0.Sub(X0, &tpOuter)

		// (f) Deflate y0: y0 = y0 - t * c
		y0.AddScaledVec(y0, -cScalar, &tVec)

		// (g) Store component.
		W.SetCol(extracted, wVec.RawVector().Data)
		Pmat.SetCol(extracted, pVec.RawVector().Data)
		Cvec.SetVec(extracted, cScalar)

		extracted++
	}

	if extracted == 0 {
		return fmt.Errorf("ml/pls: failed to extract any components")
	}

	// Build final matrices at the actual extracted dimension.
	m.w = mat.NewDense(p, extracted, nil)
	m.p = mat.NewDense(p, extracted, nil)
	m.c = mat.NewVecDense(extracted, nil)
	for j := 0; j < extracted; j++ {
		m.w.SetCol(j, colDataAt(W, j))
		m.p.SetCol(j, colDataAt(Pmat, j))
		m.c.SetVec(j, Cvec.AtVec(j))
	}

	// 5. Compute regression coefficients B_pls = W * (PᵀW)⁻¹ * C.
	if err := m.computeCoefficients(p); err != nil {
		return fmt.Errorf("ml/pls: compute coefficients: %w", err)
	}

	m.fitted = true
	return nil
}

// computeCoefficients calculates B_pls mapping standardized X to centered y:
//
//	B_pls = W * (PᵀW)⁻¹ * C   (p × 1)
func (m *PLS) computeCoefficients(p int) error {
	// PtW = Pᵀ * W  → k × k
	var PtW mat.Dense
	PtW.Mul(m.p.T(), m.w)

	// invert PtW (k × k is small in practice).
	var PtWInv mat.Dense
	if err := PtWInv.Inverse(&PtW); err != nil {
		return fmt.Errorf("PᵀW singular, cannot compute PLS coefficients: %w", err)
	}

	// W * PtWInv → p × k
	var WPtWInv mat.Dense
	WPtWInv.Mul(m.w, &PtWInv)

	// WPtWInv * C → p × 1
	m.bPLS = mat.NewVecDense(p, nil)
	m.bPLS.MulVec(&WPtWInv, m.c)
	return nil
}

// colDataAt extracts column j from m as a contiguous []float64 suitable for SetCol.
func colDataAt(m *mat.Dense, j int) []float64 {
	raw := m.RawMatrix()
	rows := raw.Rows
	col := make([]float64, rows)
	for i := range rows {
		col[i] = raw.Data[i*raw.Stride+j]
	}
	return col
}

// Predict returns predictions for the given feature matrix X.
// X must have the same number of features as the training data.
func (m *PLS) Predict(X [][]float64) ([]float64, error) {
	if !m.fitted {
		return nil, fmt.Errorf("ml/pls: model not fitted")
	}
	n := len(X)
	if n == 0 {
		return nil, fmt.Errorf("ml/pls: empty X")
	}
	p := m.nFeatures
	for i, row := range X {
		if len(row) != p {
			return nil, fmt.Errorf("ml/pls: row %d has %d features, expected %d", i, len(row), p)
		}
	}

	// Standardize X using stored mean/std.
	Xstd := mat.NewDense(n, p, nil)
	for i := range n {
		for j := range p {
			Xstd.Set(i, j, (X[i][j]-m.xMean[j])/m.xStd[j])
		}
	}

	// ŷ = X_std * B_pls + y_mean
	var yPred mat.VecDense
	yPred.MulVec(Xstd, m.bPLS)

	pred := make([]float64, n)
	for i := range n {
		pred[i] = yPred.AtVec(i) + m.yMean
	}
	return pred, nil
}
