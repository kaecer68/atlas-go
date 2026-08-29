package eval

import "fmt"

// PDPResult holds a partial dependence plot: grid values (X) and average prediction (Y).
type PDPResult struct {
	X []float64
	Y []float64
}

// PartialDependence computes the partial dependence of a single feature.
//
// For each grid point g across the feature's range, it replaces that feature in every
// sample with g, averages the model predictions, and returns the result.
//
// Parameters:
//   - predictor: the model under inspection
//   - X: input data [nSamples][nFeatures]
//   - featureIdx: which column to analyze
//   - gridResolution: number of evenly-spaced grid points
func PartialDependence(predictor Predictor, X [][]float64, featureIdx int, gridResolution int) (PDPResult, error) {
	nSamples := len(X)
	if nSamples == 0 {
		return PDPResult{}, fmt.Errorf("eval: X is empty")
	}
	nFeatures := len(X[0])
	if featureIdx < 0 || featureIdx >= nFeatures {
		return PDPResult{}, fmt.Errorf("eval: featureIdx %d out of range [0, %d)", featureIdx, nFeatures)
	}
	if gridResolution <= 0 {
		return PDPResult{}, fmt.Errorf("eval: gridResolution must be > 0, got %d", gridResolution)
	}

	// Extract feature column and find range
	featureCol := make([]float64, nSamples)
	for i, row := range X {
		featureCol[i] = row[featureIdx]
	}
	minVal := featureCol[0]
	maxVal := featureCol[0]
	for _, v := range featureCol {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	// Build grid
	grid := make([]float64, gridResolution)
	if gridResolution == 1 {
		grid[0] = (minVal + maxVal) / 2
	} else {
		step := (maxVal - minVal) / float64(gridResolution-1)
		for i := range gridResolution {
			grid[i] = minVal + float64(i)*step
		}
		// Ensure last point is exactly maxVal to avoid floating-point drift
		grid[gridResolution-1] = maxVal
	}

	pdpY := make([]float64, gridResolution)
	for gi, g := range grid {
		// Create a copy of X with featureIdx replaced by g
		Xcopy := deepCopyX(X)
		for i := range Xcopy {
			Xcopy[i][featureIdx] = g
		}
		preds, err := predictor.Predict(Xcopy)
		if err != nil {
			return PDPResult{}, fmt.Errorf("eval: predict failed at grid point %d (value=%g): %w", gi, g, err)
		}
		var sum float64
		for _, p := range preds {
			sum += p
		}
		pdpY[gi] = sum / float64(len(preds))
	}

	return PDPResult{X: grid, Y: pdpY}, nil
}
