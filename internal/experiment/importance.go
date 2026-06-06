package experiment

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
	"github.com/kaecer68/atlas-go/internal/feature"
	"github.com/kaecer68/atlas-go/internal/ml"
)

// FactorPredictor wraps an OLS model trained to predict returns from features.
// It satisfies the eval.Predictor interface and is used with PermutationImportance
// to compute factor importance rankings.
type FactorPredictor struct {
	model *ml.OLS
}

// NewFactorPredictor returns a FactorPredictor with an untrained OLS model.
func NewFactorPredictor() *FactorPredictor {
	return &FactorPredictor{model: ml.NewOLS()}
}

// Fit trains the underlying OLS model on feature matrix X and labels y.
func (p *FactorPredictor) Fit(X [][]float64, y []float64) error {
	return p.model.Fit(X, y)
}

// Predict satisfies the eval.Predictor interface.
func (p *FactorPredictor) Predict(X [][]float64) ([]float64, error) {
	return p.model.Predict(X)
}

// ComputeImportance computes permutation-based feature importance.
//
// X is the feature matrix (samples × features), y is the label vector
// (typically forward returns). featureNames provides display names for
// each column. nRepeats controls how many times each feature is permuted
// for averaging (5–10 recommended).
//
// Returns an ImportanceResult or an error if the computation fails.
func (p *FactorPredictor) ComputeImportance(X [][]float64, y []float64, featureNames []string, nRepeats int) (eval.ImportanceResult, error) {
	imp, err := eval.PermutationImportance(p, X, y, nRepeats, "r2")
	if err != nil {
		return eval.ImportanceResult{}, fmt.Errorf("compute importance: %w", err)
	}
	imp.FeatureNames = featureNames
	return imp, nil
}

// ComputeImportanceFromBars extracts features from market bars, trains an OLS
// model on forward returns, and computes permutation-based feature importance.
//
// bars are sorted by date ascending. featureNames selects which named features
// from the feature.Registry to extract as columns of X. nRepeats controls how
// many permutation repeats per feature (5–10 recommended).
func (p *FactorPredictor) ComputeImportanceFromBars(bars []domain.DailyBar, featureNames []string, nRepeats int) (eval.ImportanceResult, error) {
	if len(bars) == 0 {
		return eval.ImportanceResult{}, fmt.Errorf("compute importance: bars is empty")
	}
	if len(featureNames) == 0 {
		return eval.ImportanceResult{}, fmt.Errorf("compute importance: featureNames is empty")
	}

	extract := feature.MakeExtractor(featureNames)
	X := extract(bars)
	y := feature.ForwardReturnLabel()(bars)

	if len(X) != len(y) || len(X) == 0 {
		return eval.ImportanceResult{}, fmt.Errorf("compute importance: feature/label mismatch (%d vs %d)", len(X), len(y))
	}

	if err := p.Fit(X, y); err != nil {
		return eval.ImportanceResult{}, fmt.Errorf("compute importance: fit: %w", err)
	}

	return p.ComputeImportance(X, y, featureNames, nRepeats)
}
