package backtest

import (
	"fmt"
	"math"

	"gonum.org/v1/gonum/mat"
)

// FF5Result holds the results of a Fama-French 5-factor + Momentum performance
// attribution regression. The regression decomposes portfolio returns into
// systematic factor exposures plus a true alpha (intercept).
type FF5Result struct {
	// Alpha is the annualized alpha derived from the regression intercept
	// (intercept × 252 trading days).
	Alpha float64 `json:"alpha"`

	// AlphaTStat is the t-statistic testing whether the intercept differs
	// significantly from zero.
	AlphaTStat float64 `json:"alpha_t_stat"`

	// Exposures maps each factor name to its estimated beta coefficient.
	// Keys may include "Mkt", "SMB", "HML", "RMW", "CMA", "MOM" depending
	// on which factors were available.
	Exposures map[string]float64 `json:"exposures"`

	// TStats maps each factor name to its t-statistic.
	TStats map[string]float64 `json:"t_stats"`

	// R2 is the coefficient of determination (unadjusted).
	R2 float64 `json:"r2"`

	// AdjR2 is the adjusted R² that penalizes for the number of regressors.
	AdjR2 float64 `json:"adj_r2"`

	// N is the number of daily observations used in the regression.
	N int `json:"n"`

	// AnnualizedVol is the annualized residual (idiosyncratic) volatility,
	// computed as sqrt(MSE × 252).
	AnnualizedVol float64 `json:"annualized_vol"`
}

// canonicalFactors is the ordered list of expected factor keys.
var canonicalFactors = []string{"Mkt", "SMB", "HML", "RMW", "CMA", "MOM"}

// ComputeFF5Alpha performs Fama-French 5-factor + Momentum performance
// attribution via OLS regression. It decomposes daily portfolio returns into
// systematic factor exposures and a residual alpha term.
//
// The factorReturns map keys should be a subset of {"Mkt", "SMB", "HML",
// "RMW", "CMA", "MOM"}. Missing factors are omitted from the regression
// rather than treated as an error.
func ComputeFF5Alpha(portfolioReturns []float64, factorReturns map[string][]float64) (FF5Result, error) {
	// --- input validation ---
	if len(portfolioReturns) == 0 {
		return FF5Result{}, fmt.Errorf("empty portfolio returns")
	}
	if len(factorReturns) == 0 {
		return FF5Result{}, fmt.Errorf("no factor data provided")
	}
	if len(portfolioReturns) < 10 {
		return FF5Result{}, fmt.Errorf("insufficient observations (< 10)")
	}

	// Determine which canonical factors are present and in canonical order.
	var available []string
	for _, k := range canonicalFactors {
		if data, ok := factorReturns[k]; ok && len(data) > 0 {
			available = append(available, k)
		}
	}
	if len(available) == 0 {
		return FF5Result{}, fmt.Errorf("no factor data provided")
	}

	// Validate lengths match.
	n := len(portfolioReturns)
	for _, k := range available {
		if len(factorReturns[k]) != n {
			return FF5Result{}, fmt.Errorf(
				"length mismatch: portfolio has %d observations but factor %q has %d",
				n, k, len(factorReturns[k]),
			)
		}
	}

	// --- build design matrix X (n × k) and target vector y ---
	// We include an intercept column, so the design matrix has k+1 columns.
	// Column layout: [intercept, factor_1, factor_2, ...]
	k := len(available)
	nCols := k + 1 // intercept + factors
	flat := make([]float64, n*nCols)
	for i := range n {
		offset := i * nCols
		flat[offset] = 1.0 // intercept
		for j, fk := range available {
			flat[offset+1+j] = factorReturns[fk][i]
		}
	}

	design := mat.NewDense(n, nCols, flat)
	yVec := mat.NewVecDense(n, portfolioReturns)

	// --- solve OLS: β = (XᵀX)⁻¹Xᵀy ---
	var xtx mat.Dense
	xtx.Mul(design.T(), design)

	var xtxInv mat.Dense
	if err := xtxInv.Inverse(&xtx); err != nil {
		return FF5Result{}, fmt.Errorf("OLS regression failed (singular matrix): %w", err)
	}

	var xty mat.VecDense
	xty.MulVec(design.T(), yVec)

	var beta mat.VecDense
	beta.MulVec(&xtxInv, &xty)

	// --- compute residuals and MSE ---
	var yHat mat.VecDense
	yHat.MulVec(design, &beta)

	mse := 0.0
	residuals := make([]float64, n)
	for i := range portfolioReturns {
		r := portfolioReturns[i] - yHat.AtVec(i)
		residuals[i] = r
		mse += r * r
	}
	mse /= float64(n - nCols) // degrees of freedom correction

	// --- compute t-statistics: t = β / SE(β) ---
	// SE(β_j) = sqrt(MSE × (XᵀX)⁻¹_jj)
	dof := float64(n - nCols)
	tStats := make([]float64, nCols)
	for j := range nCols {
		variance := mse * xtxInv.At(j, j)
		se := math.Sqrt(variance)
		if se > 0 {
			tStats[j] = beta.AtVec(j) / se
		}
	}

	// --- compute R² and adjusted R² ---
	var ssTot float64
	yMean := 0.0
	for _, v := range portfolioReturns {
		yMean += v
	}
	yMean /= float64(n)

	var ssRes float64
	for i := range portfolioReturns {
		ssRes += residuals[i] * residuals[i]
		ssTot += (portfolioReturns[i] - yMean) * (portfolioReturns[i] - yMean)
	}

	r2 := 0.0
	if ssTot > 0 {
		r2 = 1.0 - ssRes/ssTot
	}
	adjR2 := 1.0 - (1.0-r2)*float64(n-1)/dof

	// --- build result ---
	intercept := beta.AtVec(0)
	exposures := make(map[string]float64, k)
	factorTStats := make(map[string]float64, k)
	for j, fk := range available {
		exposures[fk] = beta.AtVec(1 + j)
		factorTStats[fk] = tStats[1+j]
	}

	return FF5Result{
		Alpha:         intercept * 252,
		AlphaTStat:    tStats[0],
		Exposures:     exposures,
		TStats:        factorTStats,
		R2:            r2,
		AdjR2:         adjR2,
		N:             n,
		AnnualizedVol: math.Sqrt(mse * 252),
	}, nil
}
