package risk

import (
	"fmt"
	"math"
)

// SpilloverIndex configures the Diebold-Yilmaz (2012) directional spillover
// computation using a VAR(p) model and Cholesky-based FEVD.
type SpilloverIndex struct {
	horizon   int
	varLags   int
	variables []string
}

// SpilloverResult holds the directional spillover decomposition from
// a Cholesky-identified forecast error variance decomposition.
//
// All FromTo / FromOthers / ToOthers values are percentages (0–100).
// NetSpillover = ToOthers − FromOthers (positive = net transmitter).
// Total is the overall spillover index (0–100).
type SpilloverResult struct {
	FromTo       map[string]map[string]float64 `json:"from_to"`
	FromOthers   map[string]float64            `json:"from_others"`
	ToOthers     map[string]float64            `json:"to_others"`
	NetSpillover map[string]float64            `json:"net_spillover"`
	Total        float64                       `json:"total"`
}

// NewSpilloverIndex creates a SpilloverIndex with default horizon=10 and
// VAR lags=2. Pass the variable names that correspond to each return series.
func NewSpilloverIndex(variables []string) *SpilloverIndex {
	return &SpilloverIndex{
		horizon:   10,
		varLags:   2,
		variables: variables,
	}
}

// ---------------------------------------------------------------------------
// Matrix utilities (manual — no external library dependency)
// ---------------------------------------------------------------------------

// matMul computes C = A × B where A is m×n and B is n×p.
func matMul(A, B [][]float64) [][]float64 {
	m, n := len(A), len(A[0])
	p := len(B[0])
	C := make([][]float64, m)
	for i := range C {
		C[i] = make([]float64, p)
	}
	for i := range m {
		rowA := A[i]
		rowC := C[i]
		for k := range n {
			aik := rowA[k]
			if aik == 0 {
				continue
			}
			rowB := B[k]
			for j := range p {
				rowC[j] += aik * rowB[j]
			}
		}
	}
	return C
}

// matTranspose returns the transpose of A.
func matTranspose(A [][]float64) [][]float64 {
	m, n := len(A), len(A[0])
	At := make([][]float64, n)
	for i := range At {
		At[i] = make([]float64, m)
	}
	for i := range m {
		rowA := A[i]
		for j := range n {
			At[j][i] = rowA[j]
		}
	}
	return At
}

// matIdentity returns an n×n identity matrix.
func matIdentity(n int) [][]float64 {
	I := make([][]float64, n)
	for i := range I {
		I[i] = make([]float64, n)
		I[i][i] = 1.0
	}
	return I
}

// luDecompose performs Doolittle LU decomposition (L unit lower triangular).
// Returns L, U. On near-singularity, perturbs the diagonal by 1e-8 and retries.
func luDecompose(A [][]float64) ([][]float64, [][]float64, error) {
	n := len(A)

	U := make([][]float64, n)
	for i := range U {
		U[i] = make([]float64, n)
		copy(U[i], A[i])
	}
	L := make([][]float64, n)
	for i := range L {
		L[i] = make([]float64, n)
		L[i][i] = 1.0
	}

	const eps = 1e-8
	perturbed := false

retry:
	for k := range n {
		if math.Abs(U[k][k]) < eps {
			if !perturbed {
				for i := range n {
					U[i][i] += 1e-8
				}
				perturbed = true
				goto retry
			}
			return nil, nil, fmt.Errorf("spillover: singular matrix after perturbation")
		}
		pivot := U[k][k]
		for i := k + 1; i < n; i++ {
			L[i][k] = U[i][k] / pivot
			lik := L[i][k]
			for j := k; j < n; j++ {
				U[i][j] -= lik * U[k][j]
			}
		}
	}
	return L, U, nil
}

// solveLowerTriangular solves Lx = b where L is lower triangular.
func solveLowerTriangular(L [][]float64, b []float64) []float64 {
	n := len(L)
	x := make([]float64, n)
	for i := range n {
		sum := b[i]
		for j := 0; j < i; j++ {
			sum -= L[i][j] * x[j]
		}
		x[i] = sum / L[i][i]
	}
	return x
}

// solveUpperTriangular solves Ux = b where U is upper triangular.
func solveUpperTriangular(U [][]float64, b []float64) []float64 {
	n := len(U)
	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := b[i]
		for j := i + 1; j < n; j++ {
			sum -= U[i][j] * x[j]
		}
		if math.Abs(U[i][i]) < 1e-12 {
			x[i] = 0
		} else {
			x[i] = sum / U[i][i]
		}
	}
	return x
}

// invertMatrix computes A⁻¹ via LU decomposition (solves A × inv = I column-by-column).
func invertMatrix(A [][]float64) ([][]float64, error) {
	n := len(A)
	L, U, err := luDecompose(A)
	if err != nil {
		return nil, err
	}
	inv := make([][]float64, n)
	for col := range n {
		e := make([]float64, n)
		e[col] = 1.0
		y := solveLowerTriangular(L, e)
		x := solveUpperTriangular(U, y)
		invCol := make([]float64, n)
		for row := range n {
			invCol[row] = x[row]
		}
		inv[col] = invCol
	}
	// Transpose: currently inv[col][row] = x[row]; we want inv[row][col]
	result := matTranspose(inv)
	return result, nil
}

// cholesky performs Cholesky decomposition A = L Lᵀ (L lower triangular).
// Returns an error if A is not positive definite.
func cholesky(A [][]float64) ([][]float64, error) {
	n := len(A)
	L := make([][]float64, n)
	for i := range L {
		L[i] = make([]float64, n)
	}
	for i := range n {
		for j := 0; j <= i; j++ {
			sum := A[i][j]
			for k := 0; k < j; k++ {
				sum -= L[i][k] * L[j][k]
			}
			if i == j {
				if sum <= 0 {
					return nil, fmt.Errorf("spillover: covariance not positive definite (diag[%d]=%g)", i, sum)
				}
				L[i][j] = math.Sqrt(sum)
			} else {
				L[i][j] = sum / L[j][j]
			}
		}
	}
	return L, nil
}

// ---------------------------------------------------------------------------
// VAR estimation
// ---------------------------------------------------------------------------

// estimateVAR fits a VAR(p) model via OLS.
// Returns K×K coefficient matrices Φ₁ … Φ_p and residual covariance Σ (K×K).
func estimateVAR(returns [][]float64, p int) ([][][]float64, [][]float64, error) {
	K := len(returns)
	T := len(returns[0])
	N := T - p

	if N <= 0 {
		return nil, nil, fmt.Errorf("spillover: T=%d ≤ p=%d", T, p)
	}

	// Y: N × K  (response)
	Y := make([][]float64, N)
	for t := range N {
		Y[t] = make([]float64, K)
		for i := range K {
			Y[t][i] = returns[i][t+p]
		}
	}

	// X: N × (Kp + 1)  — [1 | y_{t-1}' | … | y_{t-p}']
	nCols := K*p + 1
	X := make([][]float64, N)
	for t := range N {
		X[t] = make([]float64, nCols)
		X[t][0] = 1.0
		for lag := range p {
			for i := range K {
				X[t][1+lag*K+i] = returns[i][t+p-1-lag]
			}
		}
	}

	// B̂ = (XᵀX)⁻¹ XᵀY
	Xt := matTranspose(X)
	XtX := matMul(Xt, X)
	XtXinv, err := invertMatrix(XtX)
	if err != nil {
		return nil, nil, fmt.Errorf("spillover: VAR (XᵀX)⁻¹ failed: %w", err)
	}
	XtY := matMul(Xt, Y)
	B := matMul(XtXinv, XtY) // (Kp+1) × K

	// Extract Φ₁ … Φ_p
	Phi := make([][][]float64, p)
	for lag := range p {
		Phi[lag] = make([][]float64, K)
		for i := range K {
			Phi[lag][i] = make([]float64, K)
			for j := range K {
				Phi[lag][i][j] = B[1+lag*K+i][j]
			}
		}
	}

	// Residual covariance Σ = EᵀE / (N − Kp − 1)
	Yhat := matMul(X, B)
	Sigma := make([][]float64, K)
	for i := range Sigma {
		Sigma[i] = make([]float64, K)
	}
	for t := range N {
		for i := range K {
			eI := Y[t][i] - Yhat[t][i]
			for j := range K {
				Sigma[i][j] += eI * (Y[t][j] - Yhat[t][j])
			}
		}
	}
	dof := float64(N - K*p - 1)
	if dof < 1 {
		dof = 1
	}
	for i := range K {
		for j := range K {
			Sigma[i][j] /= dof
		}
	}

	return Phi, Sigma, nil
}

// ---------------------------------------------------------------------------
// VMA impulse responses via companion form
// ---------------------------------------------------------------------------

// computeVMA returns Θ_h = J·Fʰ·Jᵀ for h = 0, …, H−1.
// F is the Kp×Kp companion matrix; J = [I_K | 0_{K×(K(p−1))}].
func computeVMA(Phi [][][]float64, horizon int) [][][]float64 {
	K := len(Phi[0])
	p := len(Phi)
	Kp := K * p

	// Build companion matrix F
	F := make([][]float64, Kp)
	for i := range F {
		F[i] = make([]float64, Kp)
	}
	for lag := range p {
		for i := range K {
			for j := range K {
				F[i][lag*K+j] = Phi[lag][i][j]
			}
		}
	}
	for lag := 0; lag < p-1; lag++ {
		for i := range K {
			F[(lag+1)*K+i][lag*K+i] = 1.0
		}
	}

	// J = [I_K  | 0]
	J := make([][]float64, K)
	for i := range J {
		J[i] = make([]float64, Kp)
		J[i][i] = 1.0
	}
	Jt := matTranspose(J)

	Theta := make([][][]float64, horizon)
	Fpow := matIdentity(Kp)
	for h := range horizon {
		temp := matMul(J, Fpow)
		Theta[h] = matMul(temp, Jt)
		Fpow = matMul(F, Fpow)
	}
	return Theta
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// ComputeSpillover calculates the Diebold-Yilmaz directional spillover index
// from a panel of return series using a VAR(2) model and Cholesky-identified FEVD.
//
// Parameters:
//   - returns: K parallel slices of equal-length daily log returns.
//   - vars:    K variable names (e.g., ["SPX","NDX","TAIEX"]).
//   - horizon: H-step forecast horizon (typically 10). If ≤ 0, defaults to 10.
//
// Returns an error if any series has zero variance, the sample is too small
// (T < 30), or the covariance matrix is not positive definite.
func ComputeSpillover(returns [][]float64, vars []string, horizon int) (*SpilloverResult, error) {
	if horizon <= 0 {
		horizon = 10
	}
	if len(returns) == 0 || len(vars) == 0 {
		return nil, fmt.Errorf("spillover: empty input: %d series, %d variables", len(returns), len(vars))
	}
	K := len(vars)
	if len(returns) != K {
		return nil, fmt.Errorf("spillover: returns count (%d) ≠ variables count (%d)", len(returns), K)
	}

	// Validate equal length and compute T
	T := len(returns[0])
	for i, series := range returns[1:] {
		if len(series) != T {
			return nil, fmt.Errorf("spillover: unequal series lengths: series[0]=%d, series[%d]=%d", T, i+1, len(series))
		}
	}
	if T < 30 {
		return nil, fmt.Errorf("spillover: sample size %d < 30, VAR estimation unstable", T)
	}

	// Zero-variance check
	for i := range K {
		mean := 0.0
		for t := range T {
			mean += returns[i][t]
		}
		mean /= float64(T)
		variance := 0.0
		for t := range T {
			d := returns[i][t] - mean
			variance += d * d
		}
		variance /= float64(T - 1)
		if variance < 1e-15 {
			return nil, fmt.Errorf("spillover: variable %q has zero variance", vars[i])
		}
	}

	p := 2 // VAR(2)

	// 1. Estimate VAR
	Phi, Sigma, err := estimateVAR(returns, p)
	if err != nil {
		return nil, err
	}

	// 2. Cholesky of residual covariance
	P, err := cholesky(Sigma)
	if err != nil {
		// Try perturbation on Sigma diagonal
		for i := range K {
			Sigma[i][i] += 1e-8
		}
		P, err = cholesky(Sigma)
		if err != nil {
			return nil, fmt.Errorf("spillover: Cholesky decomposition failed: %w", err)
		}
	}

	// 3. VMA impulse responses
	Theta := computeVMA(Phi, horizon)

	// 4. FEVD
	// C_h = Θ_h × P  (orthogonalised impulse response)
	// ω_{ij} = Σ_h (C_h[i][j])²
	// θ_{ij} = ω_{ij} / Σ_k ω_{ik}
	fevd := make([][]float64, K)
	for i := range fevd {
		fevd[i] = make([]float64, K)
	}

	for i := range K {
		denom := 0.0
		omega := make([]float64, K)
		for h := 0; h < horizon; h++ {
			rowH := Theta[h][i]
			for j := range K {
				// C_h[i][j] = Σ_k Θ_h[i][k] × P[k][j]
				c := 0.0
				for k := range K {
					c += rowH[k] * P[k][j]
				}
				omega[j] += c * c
			}
		}
		for j := range K {
			denom += omega[j]
		}
		if denom == 0 {
			fevd[i][i] = 1.0
		} else {
			for j := range K {
				fevd[i][j] = omega[j] / denom
			}
		}
	}

	// Normalise to percentages (each row sums to 100)
	normalised := make([][]float64, K)
	for i := range K {
		normalised[i] = make([]float64, K)
		rowSum := 0.0
		for j := range K {
			rowSum += fevd[i][j]
		}
		if rowSum > 0 {
			for j := range K {
				normalised[i][j] = (fevd[i][j] / rowSum) * 100.0
			}
		}
	}

	// Build result
	result := &SpilloverResult{
		FromTo:       make(map[string]map[string]float64, K),
		FromOthers:   make(map[string]float64, K),
		ToOthers:     make(map[string]float64, K),
		NetSpillover: make(map[string]float64, K),
	}

	for i := 0; i < K; i++ {
		result.FromTo[vars[i]] = make(map[string]float64, K)
		for j := 0; j < K; j++ {
			result.FromTo[vars[i]][vars[j]] = normalised[i][j]
		}
	}

	// FromOthers: row off-diagonal sum
	for i := 0; i < K; i++ {
		sum := 0.0
		for j := 0; j < K; j++ {
			if i != j {
				sum += normalised[i][j]
			}
		}
		result.FromOthers[vars[i]] = sum
	}

	// ToOthers: column off-diagonal sum
	for j := 0; j < K; j++ {
		sum := 0.0
		for i := 0; i < K; i++ {
			if i != j {
				sum += normalised[i][j]
			}
		}
		result.ToOthers[vars[j]] = sum
	}

	// Net
	for i := 0; i < K; i++ {
		result.NetSpillover[vars[i]] = result.ToOthers[vars[i]] - result.FromOthers[vars[i]]
	}

	// Total spillover index = average off-diagonal share
	totalSum := 0.0
	for i := 0; i < K; i++ {
		totalSum += result.FromOthers[vars[i]]
	}
	result.Total = totalSum / float64(K)

	return result, nil
}
