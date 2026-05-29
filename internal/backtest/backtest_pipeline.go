package backtest

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Model is the interface for ML models that can be fit and used for prediction
// in a rolling backtest pipeline.
type Model interface {
	// Fit trains the model on feature matrix X and target vector y.
	Fit(X [][]float64, y []float64) error

	// Predict returns predictions for the given feature matrix X.
	Predict(X [][]float64) ([]float64, error)
}

// FeatureExtractor converts a slice of DailyBar entries into a feature matrix
// where each row corresponds to one bar and each column is a feature.
type FeatureExtractor func(bars []domain.DailyBar) [][]float64

// LabelExtractor converts a slice of DailyBar entries into a label vector
// where each element corresponds to one bar.
type LabelExtractor func(bars []domain.DailyBar) []float64

// BacktestResult holds the results of a single backtest window.
type BacktestResult struct {
	// WindowID is a human-readable identifier for this window.
	WindowID string

	// TrainRange is the date range used for training.
	TrainRange WindowRange

	// TestRange is the date range used for out-of-sample testing.
	TestRange WindowRange

	// Predictions are the model's predictions on the test set.
	Predictions []float64

	// Actuals are the ground-truth labels on the test set.
	Actuals []float64

	// Metrics holds per-window evaluation metrics (e.g., MSE, MAE, Sharpe-like).
	Metrics map[string]float64
}

// BacktestPipeline executes a rolling window backtest using the SK-03 split
// specification and an ML Model.
type BacktestPipeline struct {
	// Split defines the rolling window parameters.
	Split RollingWindowSplit

	// Data is the full dataset used for the backtest.
	Data []domain.DailyBar

	// ExtractFeatures converts raw bars into a feature matrix.
	// Required: must be set before calling Run.
	ExtractFeatures FeatureExtractor

	// ExtractLabels converts raw bars into a label vector.
	// Required: must be set before calling Run.
	ExtractLabels LabelExtractor
}

// NewBacktestPipeline creates a BacktestPipeline with SK-03 default split
// parameters.
func NewBacktestPipeline(data []domain.DailyBar) *BacktestPipeline {
	return &BacktestPipeline{
		Split: NewRollingWindowSplit(),
		Data:  data,
	}
}

// Run executes the rolling backtest. For each split window it:
//  1. Filters data to the train, valid, and test periods
//  2. Extracts features and labels
//  3. Fits the model on training data
//  4. Predicts on test data
//  5. Records the window result
//
// Returns results for all windows, or an error if any step fails.
func (p *BacktestPipeline) Run(model Model) ([]BacktestResult, error) {
	if p.ExtractFeatures == nil {
		return nil, fmt.Errorf("backtest_pipeline: ExtractFeatures is nil")
	}
	if p.ExtractLabels == nil {
		return nil, fmt.Errorf("backtest_pipeline: ExtractLabels is nil")
	}
	if len(p.Data) == 0 {
		return nil, fmt.Errorf("backtest_pipeline: Data is empty")
	}

	dataStart := p.Data[0].Date
	for _, bar := range p.Data[1:] {
		if bar.Date.Before(dataStart) {
			dataStart = bar.Date
		}
	}

	windows, err := p.Split.Split(dataStart)
	if err != nil {
		return nil, fmt.Errorf("backtest_pipeline: split: %w", err)
	}

	results := make([]BacktestResult, 0, len(windows))
	for _, w := range windows {
		trainBars := filterBars(p.Data, w.TrainStart, w.TrainEnd)
		testBars := filterBars(p.Data, w.TestStart, w.TestEnd)

		if len(trainBars) == 0 {
			return nil, fmt.Errorf("backtest_pipeline: window %s has empty training data", windowID(w))
		}
		if len(testBars) == 0 {
			// Skip windows without test data (data may not extend to test_end).
			continue
		}

		X := p.ExtractFeatures(trainBars)
		y := p.ExtractLabels(trainBars)

		if len(X) != len(y) {
			return nil, fmt.Errorf("backtest_pipeline: window %s feature count %d != label count %d",
				windowID(w), len(X), len(y))
		}

		if err := model.Fit(X, y); err != nil {
			return nil, fmt.Errorf("backtest_pipeline: window %s fit: %w", windowID(w), err)
		}

		XTest := p.ExtractFeatures(testBars)
		if len(XTest) == 0 {
			return nil, fmt.Errorf("backtest_pipeline: window %s has empty test features", windowID(w))
		}

		predictions, err := model.Predict(XTest)
		if err != nil {
			return nil, fmt.Errorf("backtest_pipeline: window %s predict: %w", windowID(w), err)
		}

		actuals := p.ExtractLabels(testBars)
		if len(predictions) != len(actuals) {
			return nil, fmt.Errorf("backtest_pipeline: window %s prediction count %d != actual count %d",
				windowID(w), len(predictions), len(actuals))
		}

		results = append(results, BacktestResult{
			WindowID:   windowID(w),
			TrainRange: w,
			TestRange:  w,
			Predictions: predictions,
			Actuals:     actuals,
			Metrics: map[string]float64{
				"train_samples": float64(len(trainBars)),
				"test_samples":  float64(len(testBars)),
			},
		})
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("backtest_pipeline: no windows produced results")
	}

	return results, nil
}

// windowID returns a human-readable identifier for a WindowRange.
func windowID(w WindowRange) string {
	return fmt.Sprintf("train_%s_valid_%s_test_%s",
		w.TrainEnd.Format("20060102"),
		w.ValidEnd.Format("20060102"),
		w.TestEnd.Format("20060102"))
}

// filterBars returns bars whose Date is in the inclusive range [start, end].
func filterBars(bars []domain.DailyBar, start, end time.Time) []domain.DailyBar {
	var out []domain.DailyBar
	for _, b := range bars {
		if !b.Date.Before(start) && !b.Date.After(end) {
			out = append(out, b)
		}
	}
	return out
}
