package eval

import (
	"fmt"
	"math"
	"sort"
)

// InteractionPair describes a detected pairwise interaction between two features.
type InteractionPair struct {
	FeatureA       string  `json:"feature_a"`
	FeatureB       string  `json:"feature_b"`
	HStatistic     float64 `json:"h_statistic"`
	Interpretation string  `json:"interpretation"`
}

// InteractionResult holds Friedman's H-statistic results for all feature pairs.
type InteractionResult struct {
	// FeatureNames lists the input feature names in order.
	FeatureNames []string `json:"feature_names"`
	// HMatrix is an n×n symmetric matrix where HMatrix[i][j] is the H-statistic
	// between feature i and feature j. The diagonal is zero.
	HMatrix [][]float64 `json:"h_matrix"`
	// SignificantPairs contains all pairs with H above threshold, sorted by H descending.
	SignificantPairs []InteractionPair `json:"significant_pairs"`
}

// interpretH returns a human-readable interpretation for an H-statistic value.
func interpretH(h float64) string {
	switch {
	case h < 0.1:
		return "negligible"
	case h < 0.3:
		return "weak"
	case h < 0.5:
		return "moderate"
	default:
		return "strong"
	}
}

// FriedmanH computes Friedman's H-statistic for all pairwise feature interactions.
//
// For each pair of features (i, j) with i < j:
//  1. Compute PD(x_i) and PD(x_j) — marginal partial dependence of each feature.
//  2. Compute PD(x_i, x_j) — joint partial dependence on a 2D grid.
//  3. Compute H²_ij = Σ(PD(x_i,x_j) - PD(x_i) - PD(x_j) + ȳ)² / Σ(PD(x_i,x_j) - ȳ)²
//  4. H_ij = sqrt(H²_ij), capped at 1.0
//
// H close to 0 indicates no interaction; H close to 1 indicates strong interaction.
//
// Parameters:
//   - predictor: the model under inspection
//   - X: input data [nSamples][nFeatures]
//   - y: target values (used to compute the overall mean)
//   - featureNames: names for each feature column; if empty, auto-generated
//   - gridResolution: number of grid points per feature dimension (clamped to [2, 50])
func FriedmanH(predictor Predictor, X [][]float64, y []float64, featureNames []string, gridResolution int) (InteractionResult, error) {
	nSamples := len(X)
	if nSamples == 0 || len(y) == 0 {
		return InteractionResult{}, nil
	}
	nFeatures := len(X[0])
	if nFeatures < 2 {
		return InteractionResult{}, nil
	}

	// Clamp grid resolution
	if gridResolution < 2 {
		gridResolution = 2
	}
	if gridResolution > 50 {
		gridResolution = 50
	}

	// Build feature names
	names := make([]string, nFeatures)
	if len(featureNames) >= nFeatures {
		copy(names, featureNames)
	} else {
		for i := range nFeatures {
			names[i] = fmt.Sprintf("feature_%d", i)
		}
	}

	// Compute overall mean of y
	var ySum float64
	for _, v := range y {
		ySum += v
	}
	yMean := ySum / float64(len(y))

	// Initialize H-matrix
	hMatrix := make([][]float64, nFeatures)
	for i := range hMatrix {
		hMatrix[i] = make([]float64, nFeatures)
	}

	var pairs []InteractionPair

	for fi := range nFeatures {
		for fj := fi + 1; fj < nFeatures; fj++ {
			h, err := computePairwiseH(predictor, X, fi, fj, yMean, gridResolution)
			if err != nil {
				return InteractionResult{}, fmt.Errorf("eval: H-statistic for features (%d, %d): %w", fi, fj, err)
			}
			hMatrix[fi][fj] = h
			hMatrix[fj][fi] = h

			pairs = append(pairs, InteractionPair{
				FeatureA:       names[fi],
				FeatureB:       names[fj],
				HStatistic:     h,
				Interpretation: interpretH(h),
			})
		}
	}

	// Sort pairs by H descending
	sort.Slice(pairs, func(a, b int) bool {
		return pairs[a].HStatistic > pairs[b].HStatistic
	})

	return InteractionResult{
		FeatureNames:     names,
		HMatrix:          hMatrix,
		SignificantPairs: pairs,
	}, nil
}

// computePairwiseH computes the H-statistic for a single pair of features (fi, fj).
func computePairwiseH(predictor Predictor, X [][]float64, fi, fj int, yMean float64, gridResolution int) (float64, error) {
	// Extract feature ranges
	minFi, maxFi := featureRange(X, fi)
	minFj, maxFj := featureRange(X, fj)

	// Build 1D grids
	gridFi := buildGrid(minFi, maxFi, gridResolution)
	gridFj := buildGrid(minFj, maxFj, gridResolution)

	// Compute 1D partial dependence for fi: PD(x_fi)[g] for each grid point g
	pdFi, err := partialDependence1D(predictor, X, fi, gridFi)
	if err != nil {
		return 0, fmt.Errorf("1D PD for feature %d: %w", fi, err)
	}

	// Compute 1D partial dependence for fj
	pdFj, err := partialDependence1D(predictor, X, fj, gridFj)
	if err != nil {
		return 0, fmt.Errorf("1D PD for feature %d: %w", fj, err)
	}

	// Compute 2D joint partial dependence: PD(x_fi, x_fj)[g_i][g_j]
	pd2D, err := partialDependence2D(predictor, X, fi, fj, gridFi, gridFj)
	if err != nil {
		return 0, fmt.Errorf("2D PD for features (%d, %d): %w", fi, fj, err)
	}

	// Compute H² = numerator / denominator
	// numerator = Σ_{gi,gj} (PD(gi,gj) - PD(gi) - PD(gj) + ȳ)²
	// denominator = Σ_{gi,gj} (PD(gi,gj) - ȳ)²
	var numerator, denominator float64
	for gi := range gridResolution {
		for gj := range gridResolution {
			noInteraction := pd2D[gi][gj] - pdFi[gi] - pdFj[gj] + yMean
			numerator += noInteraction * noInteraction
			fromMean := pd2D[gi][gj] - yMean
			denominator += fromMean * fromMean
		}
	}

	if denominator == 0 {
		return 0, nil
	}

	hSquared := numerator / denominator

	if hSquared < 0 {
		return 0, nil
	}

	h := math.Sqrt(hSquared)
	if h > 1.0 {
		h = 1.0
	}
	return h, nil
}

// partialDependence1D computes 1D partial dependence for a single feature on a given grid.
// Returns pdp[gi] = average prediction when feature fi is fixed at gridFi[gi].
func partialDependence1D(predictor Predictor, X [][]float64, fi int, grid []float64) ([]float64, error) {
	gridRes := len(grid)
	pdp := make([]float64, gridRes)

	for gi, g := range grid {
		Xcopy := deepCopyX(X)
		for i := range Xcopy {
			Xcopy[i][fi] = g
		}
		preds, err := predictor.Predict(Xcopy)
		if err != nil {
			return nil, fmt.Errorf("predict at grid[%d]=%g: %w", gi, g, err)
		}
		var sum float64
		for _, p := range preds {
			sum += p
		}
		pdp[gi] = sum / float64(len(preds))
	}

	return pdp, nil
}

// partialDependence2D computes 2D joint partial dependence for features fi and fj.
// Returns pdp[gi][gj] = average prediction when fi=gridFi[gi] and fj=gridFj[gj].
func partialDependence2D(predictor Predictor, X [][]float64, fi, fj int, gridFi, gridFj []float64) ([][]float64, error) {
	resI := len(gridFi)
	resJ := len(gridFj)
	pdp := make([][]float64, resI)
	for i := range pdp {
		pdp[i] = make([]float64, resJ)
	}

	for gi, gI := range gridFi {
		for gj, gJ := range gridFj {
			Xcopy := deepCopyX(X)
			for k := range Xcopy {
				Xcopy[k][fi] = gI
				Xcopy[k][fj] = gJ
			}
			preds, err := predictor.Predict(Xcopy)
			if err != nil {
				return nil, fmt.Errorf("predict at grid(%d,%d)=(%g,%g): %w", gi, gj, gI, gJ, err)
			}
			var sum float64
			for _, p := range preds {
				sum += p
			}
			pdp[gi][gj] = sum / float64(len(preds))
		}
	}

	return pdp, nil
}

// featureRange returns the min and max of column col in X.
func featureRange(X [][]float64, col int) (float64, float64) {
	minVal := X[0][col]
	maxVal := X[0][col]
	for _, row := range X {
		v := row[col]
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	return minVal, maxVal
}

// buildGrid creates evenly-spaced grid points from min to max with given resolution.
func buildGrid(minVal, maxVal float64, resolution int) []float64 {
	grid := make([]float64, resolution)
	if resolution == 1 {
		grid[0] = (minVal + maxVal) / 2
		return grid
	}
	step := (maxVal - minVal) / float64(resolution-1)
	for i := range resolution {
		grid[i] = minVal + float64(i)*step
	}
	grid[resolution-1] = maxVal
	return grid
}
