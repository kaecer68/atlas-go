package eval

import (
	"fmt"
	"math/rand"
	"sort"
)

// Predictor wraps a model's predict method.
type Predictor interface {
	Predict(X [][]float64) ([]float64, error)
}

// ImportanceResult holds permutation-based feature importance results.
type ImportanceResult struct {
	FeatureNames []string  `json:"feature_names"`
	Importances  []float64 `json:"importances"`
	// Ranks is 1-indexed: rank 1 = most important feature.
	Ranks []int `json:"ranks"`
}

// PermutationImportance computes feature importance via permutation.
//
// For each feature column i, the function:
//  1. Computes a baseline metric from model.Predict(X)
//  2. Shuffles column i nRepeats times, each time re-evaluating
//  3. Records the drop in metric performance
//  4. Averages the drops across repeats → importance[i]
//
// The input X is never mutated; a copy is made before each shuffle.
// metric must be "r2" (the only metric currently supported).
func PermutationImportance(model Predictor, X [][]float64, y []float64, nRepeats int, metric string) (ImportanceResult, error) {
	if metric != "r2" {
		return ImportanceResult{}, fmt.Errorf("eval: unsupported metric %q, only \"r2\" is supported", metric)
	}
	nSamples := len(X)
	if nSamples == 0 {
		return ImportanceResult{}, fmt.Errorf("eval: X is empty")
	}
	if len(y) != nSamples {
		return ImportanceResult{}, fmt.Errorf("eval: len(y)=%d does not match len(X)=%d", len(y), nSamples)
	}
	nFeatures := len(X[0])
	if nFeatures == 0 {
		return ImportanceResult{}, fmt.Errorf("eval: X has zero features")
	}
	if nRepeats <= 0 {
		return ImportanceResult{}, fmt.Errorf("eval: nRepeats must be > 0, got %d", nRepeats)
	}

	// Baseline prediction and metric
	baselinePred, err := model.Predict(X)
	if err != nil {
		return ImportanceResult{}, fmt.Errorf("eval: baseline prediction failed: %w", err)
	}
	baselineMetric := oosR2Unsafe(y, baselinePred)

	importances := make([]float64, nFeatures)
	featureNames := make([]string, nFeatures)
	for fi := range nFeatures {
		featureNames[fi] = fmt.Sprintf("feature_%d", fi)

		var totalDrop float64
		for rep := range nRepeats {
			// Create a copy of X with only column fi shuffled
			Xcopy := deepCopyX(X)
			col := extractColumn(Xcopy, fi)
			shuffleSlice(col)
			setColumn(Xcopy, fi, col)

			pred, err := model.Predict(Xcopy)
			if err != nil {
				return ImportanceResult{}, fmt.Errorf("eval: prediction failed for feature %d, repeat %d: %w", fi, rep, err)
			}
			m := oosR2Unsafe(y, pred)
			drop := baselineMetric - m
			totalDrop += drop
		}
		importances[fi] = totalDrop / float64(nRepeats)
	}

	// Compute ranks (1-indexed, descending by importance)
	ranks := rankDescending(importances)

	return ImportanceResult{
		FeatureNames: featureNames,
		Importances:  importances,
		Ranks:        ranks,
	}, nil
}

// oosR2Unsafe is OOSR2 without length/zero guards; caller must ensure valid inputs.
func oosR2Unsafe(yTrue, yPred []float64) float64 {
	var ssRes, ssTot float64
	for i := range yTrue {
		diff := yTrue[i] - yPred[i]
		ssRes += diff * diff
		ssTot += yTrue[i] * yTrue[i]
	}
	if ssTot == 0 {
		return 0
	}
	return 1 - ssRes/ssTot
}

// extractColumn returns column col of X as a new slice.
func extractColumn(X [][]float64, col int) []float64 {
	out := make([]float64, len(X))
	for i, row := range X {
		out[i] = row[col]
	}
	return out
}

// setColumn writes values into column col of X.
func setColumn(X [][]float64, col int, vals []float64) {
	for i, row := range X {
		row[col] = vals[i]
	}
}

// shuffleSlice randomly shuffles a slice in place using Fisher-Yates.
func shuffleSlice(s []float64) {
	rand.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
}

// deepCopyX creates a deep copy of X.
func deepCopyX(X [][]float64) [][]float64 {
	out := make([][]float64, len(X))
	for i, row := range X {
		out[i] = make([]float64, len(row))
		copy(out[i], row)
	}
	return out
}

// rankDescending returns 1-indexed ranks where 1 = highest value.
func rankDescending(values []float64) []int {
	n := len(values)
	ranks := make([]int, n)
	type entry struct {
		idx int
		val float64
	}
	entries := make([]entry, n)
	for i, v := range values {
		entries[i] = entry{idx: i, val: v}
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].val > entries[b].val })
	for r, e := range entries {
		ranks[e.idx] = r + 1
	}
	return ranks
}

// Compile-time check that rnd is unused but available: we don't need a custom rand source for the
// permutation importances since rand is implicitly seeded for Go 1.25 benchmarks/tests. Tests
// may override the seed via rand.Seed if deterministic behavior is required.
