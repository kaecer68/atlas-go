package robustness

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/eval"
)

// Model is the interface for a trainable model used in ablation analysis.
type Model interface {
	Fit(X [][]float64, y []float64) error
	Predict(X [][]float64) ([]float64, error)
}

// AblationReport contains the results of factor ablation analysis.
type AblationReport struct {
	FullModelR2      float64
	AblatedR2        float64
	R2Drop           float64
	R2DropPct        float64
	ExcludedFactors  []string
	RemainingFactors []string
}

// AblationAnalysis measures the impact of removing factors from a model.
//
// X is the feature matrix (rows = observations, cols = factors).
// y is the target vector.
// allFactors lists factor names in the same order as X columns.
// excludedFactors lists factor names to ablate.
// modelConstructor creates a new Model instance for training.
//
// Two models are trained: one on all factors, one without excluded factors.
// R2OOS is used as the evaluation metric. R2Drop measures how much performance
// is lost (positive = factors matter, negative = factors hurt).
func AblationAnalysis(X [][]float64, y []float64, allFactors []string, excludedFactors []string, modelConstructor func() Model) (AblationReport, error) {
	if len(X) == 0 {
		return AblationReport{}, fmt.Errorf("ablation: X is empty")
	}
	if len(y) == 0 {
		return AblationReport{}, fmt.Errorf("ablation: y is empty")
	}
	if len(y) != len(X) {
		return AblationReport{}, fmt.Errorf("ablation: X and y have different lengths (%d vs %d)", len(X), len(y))
	}
	if len(allFactors) == 0 {
		return AblationReport{}, fmt.Errorf("ablation: allFactors is empty")
	}
	if len(X[0]) != len(allFactors) {
		return AblationReport{}, fmt.Errorf("ablation: X has %d columns but allFactors has %d names", len(X[0]), len(allFactors))
	}

	// Build a set of excluded factor names for quick lookup
	excludedSet := make(map[string]bool, len(excludedFactors))
	for _, f := range excludedFactors {
		excludedSet[f] = true
	}

	// Identify column indices for excluded and remaining factors
	var excludedIndices []int
	var remainingFactors []string
	var remainingIndices []int

	for i, factor := range allFactors {
		if excludedSet[factor] {
			excludedIndices = append(excludedIndices, i)
		} else {
			remainingFactors = append(remainingFactors, factor)
			remainingIndices = append(remainingIndices, i)
		}
	}

	// Error if any excluded factor was not found
	notFound := make(map[string]bool)
	for _, f := range excludedFactors {
		notFound[f] = true
	}
	for _, f := range allFactors {
		delete(notFound, f)
	}
	if len(notFound) > 0 {
		var missingList []string
		for f := range notFound {
			missingList = append(missingList, f)
		}
		return AblationReport{}, fmt.Errorf("ablation: excluded factors not found in allFactors: %v", missingList)
	}

	// Build full X (all columns) and ablated X (without excluded columns)
	fullX := X // use original X for full model

	ablatedX := make([][]float64, len(X))
	for i, row := range X {
		ablatedRow := make([]float64, len(remainingIndices))
		for j, colIdx := range remainingIndices {
			ablatedRow[j] = row[colIdx]
		}
		ablatedX[i] = ablatedRow
	}

	// Train full model and compute R2OOS
	fullModel := modelConstructor()
	if err := fullModel.Fit(fullX, y); err != nil {
		return AblationReport{}, fmt.Errorf("ablation: full model fit: %w", err)
	}
	fullPred, err := fullModel.Predict(fullX)
	if err != nil {
		return AblationReport{}, fmt.Errorf("ablation: full model predict: %w", err)
	}
	fullR2 := eval.OOSR2(y, fullPred)

	// Train ablated model and compute R2OOS
	ablatedModel := modelConstructor()
	if err := ablatedModel.Fit(ablatedX, y); err != nil {
		return AblationReport{}, fmt.Errorf("ablation: ablated model fit: %w", err)
	}
	ablatedPred, err := ablatedModel.Predict(ablatedX)
	if err != nil {
		return AblationReport{}, fmt.Errorf("ablation: ablated model predict: %w", err)
	}
	ablatedR2 := eval.OOSR2(y, ablatedPred)

	// Calculate R2 drop
	r2Drop := fullR2 - ablatedR2
	var r2DropPct float64
	if fullR2 > 0 {
		r2DropPct = (r2Drop / fullR2) * 100
	}

	return AblationReport{
		FullModelR2:      fullR2,
		AblatedR2:        ablatedR2,
		R2Drop:           r2Drop,
		R2DropPct:        r2DropPct,
		ExcludedFactors:  excludedFactors,
		RemainingFactors: remainingFactors,
	}, nil
}
