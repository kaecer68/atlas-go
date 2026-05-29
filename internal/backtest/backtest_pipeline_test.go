package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ml"
)

// Compile-time verification that ml models satisfy the backtest.Model interface.
var _ Model = (*ml.OLS)(nil)

// dummyModel predicts the mean of training labels for all test inputs.
type dummyModel struct {
	mean     float64
	fitted   bool
	failFit  bool
	failPred bool
}

func (m *dummyModel) Fit(X [][]float64, y []float64) error {
	if m.failFit {
		return &testError{"dummy fit failure"}
	}
	if len(y) == 0 {
		m.mean = 0
	} else {
		sum := 0.0
		for _, v := range y {
			sum += v
		}
		m.mean = sum / float64(len(y))
	}
	m.fitted = true
	return nil
}

func (m *dummyModel) Predict(X [][]float64) ([]float64, error) {
	if m.failPred {
		return nil, &testError{"dummy predict failure"}
	}
	out := make([]float64, len(X))
	for i := range out {
		out[i] = m.mean
	}
	return out, nil
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// makeSyntheticBars generates daily bars from start to end (inclusive).
func makeSyntheticBars(start, end time.Time) []domain.DailyBar {
	var bars []domain.DailyBar
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		bars = append(bars, domain.DailyBar{
			Date:   d,
			Symbol: "TEST",
			Close:  100.0 + float64(d.YearDay()),
		})
	}
	return bars
}

// makeCloseLabel returns the close price as label.
func makeCloseLabel(bars []domain.DailyBar) []float64 {
	labels := make([]float64, len(bars))
	for i, b := range bars {
		labels[i] = b.Close
	}
	return labels
}

// makeCloseFeature returns a single-feature matrix from close prices.
func makeCloseFeature(bars []domain.DailyBar) [][]float64 {
	features := make([][]float64, len(bars))
	for i, b := range bars {
		features[i] = []float64{b.Close}
	}
	return features
}

func TestRollingWindowSplit_DefaultParams(t *testing.T) {
	split := NewRollingWindowSplit()
	if split.FirstTrainEnd.Year() != 2007 || split.FirstTrainEnd.Month() != 12 || split.FirstTrainEnd.Day() != 31 {
		t.Fatalf("FirstTrainEnd: expected 2007-12-31, got %s", split.FirstTrainEnd.Format("2006-01-02"))
	}
	if split.ValidLengthYears != 2 {
		t.Fatalf("ValidLengthYears: expected 2, got %d", split.ValidLengthYears)
	}
}

func TestRollingWindowSplit_Split(t *testing.T) {
	split := NewRollingWindowSplit()
	dataStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)

	windows, err := split.Split(dataStart)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	if len(windows) == 0 {
		t.Fatal("expected at least one window")
	}

	// First window: train ends at 2007-12-31, valid starts 2008-01-01.
	w0 := windows[0]
	if w0.TrainStart != dataStart {
		t.Errorf("window[0].TrainStart: expected %s, got %s", dataStart, w0.TrainStart)
	}
	if !w0.TrainEnd.Equal(time.Date(2007, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("window[0].TrainEnd: expected 2007-12-31, got %s", w0.TrainEnd)
	}
	expectedValidStart := time.Date(2008, 1, 1, 0, 0, 0, 0, time.UTC)
	if !w0.ValidStart.Equal(expectedValidStart) {
		t.Errorf("window[0].ValidStart: expected %s, got %s", expectedValidStart, w0.ValidStart)
	}
	expectedValidEnd := time.Date(2009, 12, 31, 0, 0, 0, 0, time.UTC)
	if !w0.ValidEnd.Equal(expectedValidEnd) {
		t.Errorf("window[0].ValidEnd: expected %s, got %s", expectedValidEnd, w0.ValidEnd)
	}
	expectedTestStart := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)
	if !w0.TestStart.Equal(expectedTestStart) {
		t.Errorf("window[0].TestStart: expected %s, got %s", expectedTestStart, w0.TestStart)
	}
	if !w0.TestEnd.Equal(split.TestEnd) {
		t.Errorf("window[0].TestEnd: expected %s, got %s", split.TestEnd, w0.TestEnd)
	}
}

func TestRollingWindowSplit_StopCondition(t *testing.T) {
	split := NewRollingWindowSplit()
	dataStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)

	windows, err := split.Split(dataStart)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	// The last window should have valid_start in 2020.
	last := windows[len(windows)-1]
	if last.ValidStart.Year() > 2020 {
		t.Errorf("last window ValidStart year %d > 2020 (stop condition violated)", last.ValidStart.Year())
	}
	// The first window after the last would have valid_start in 2021.
	if last.ValidStart.Year() != 2020 {
		t.Errorf("expected last ValidStart year to be 2020, got %d", last.ValidStart.Year())
	}

	// Verify that train_end expands by 1 year each window.
	for i := 1; i < len(windows); i++ {
		prev := windows[i-1]
		curr := windows[i]
		expectedTrainEnd := prev.TrainEnd.AddDate(1, 0, 0)
		if !curr.TrainEnd.Equal(expectedTrainEnd) {
			t.Errorf("window[%d].TrainEnd: expected %s (prev + 1 year), got %s",
				i, expectedTrainEnd, curr.TrainEnd)
		}
		expectedValidEnd := prev.ValidEnd.AddDate(1, 0, 0)
		if !curr.ValidEnd.Equal(expectedValidEnd) {
			t.Errorf("window[%d].ValidEnd: expected %s (prev + 1 year), got %s",
				i, expectedValidEnd, curr.ValidEnd)
		}
	}
}

func TestRollingWindowSplit_TrainStartIsDataStart(t *testing.T) {
	split := NewRollingWindowSplit()
	dataStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)

	windows, err := split.Split(dataStart)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	for i, w := range windows {
		if !w.TrainStart.Equal(dataStart) {
			t.Errorf("window[%d]: TrainStart expected %s, got %s",
				i, dataStart, w.TrainStart)
		}
	}
}

func TestRollingWindowSplit_NoDataBeforeTrainEnd(t *testing.T) {
	// Data starts after first_train_end — should still produce windows
	// with train_start = data_start.
	split := NewRollingWindowSplit()
	dataStart := time.Date(2010, 1, 1, 0, 0, 0, 0, time.UTC)

	windows, err := split.Split(dataStart)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	// First window uses data_start as train start.
	if !windows[0].TrainStart.Equal(dataStart) {
		t.Errorf("expected TrainStart to be data_start %s, got %s",
			dataStart, windows[0].TrainStart)
	}
}

func TestRollingWindowSplit_InvalidParams(t *testing.T) {
	// ValidLengthYears = 0
	split := NewRollingWindowSplit()
	split.ValidLengthYears = 0
	_, err := split.Split(time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for ValidLengthYears=0")
	}

	// StepYears = 0
	split = NewRollingWindowSplit()
	split.StepYears = 0
	_, err = split.Split(time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for StepYears=0")
	}

	// TestEnd before FirstTrainEnd
	split = NewRollingWindowSplit()
	split.TestEnd = time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = split.Split(time.Date(2004, 1, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for TestEnd before FirstTrainEnd")
	}
}

func TestBacktestPipeline_Run(t *testing.T) {
	// Generate daily data from 2005-01-01 to 2022-04-30 so that test windows have data.
	dataStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	dataEnd := time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC)
	bars := makeSyntheticBars(dataStart, dataEnd)

	pipeline := NewBacktestPipeline(bars)
	pipeline.ExtractFeatures = makeCloseFeature
	pipeline.ExtractLabels = makeCloseLabel

	model := &dummyModel{}
	results, err := pipeline.Run(model)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result window")
	}

	// Verify each result has predictions and actuals.
	for i, r := range results {
		if len(r.Predictions) == 0 {
			t.Errorf("result[%d]: predictions empty", i)
		}
		if len(r.Actuals) == 0 {
			t.Errorf("result[%d]: actuals empty", i)
		}
		if len(r.Predictions) != len(r.Actuals) {
			t.Errorf("result[%d]: predictions %d != actuals %d",
				i, len(r.Predictions), len(r.Actuals))
		}
		if r.WindowID == "" {
			t.Errorf("result[%d]: WindowID empty", i)
		}
		if samples, ok := r.Metrics["train_samples"]; !ok || samples <= 0 {
			t.Errorf("result[%d]: train_samples missing or zero", i)
		}
		if samples, ok := r.Metrics["test_samples"]; !ok || samples <= 0 {
			t.Errorf("result[%d]: test_samples missing or zero", i)
		}
	}
}

func TestBacktestPipeline_DummyModelPredictsMean(t *testing.T) {
	dataStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	dataEnd := time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC)
	bars := makeSyntheticBars(dataStart, dataEnd)

	pipeline := NewBacktestPipeline(bars)
	pipeline.ExtractFeatures = makeCloseFeature
	pipeline.ExtractLabels = makeCloseLabel

	model := &dummyModel{}
	results, err := pipeline.Run(model)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !model.fitted {
		t.Fatal("model was never fitted")
	}

	// The dummyModel predicts the mean of training labels for all test inputs.
	// All predictions in a single result window should be identical.
	for i, r := range results {
		if len(r.Predictions) == 0 {
			t.Errorf("result[%d]: empty predictions", i)
			continue
		}
		first := r.Predictions[0]
		for j := 1; j < len(r.Predictions); j++ {
			if r.Predictions[j] != first {
				t.Errorf("result[%d]: predictions not all equal: [0]=%f, [%d]=%f",
					i, first, j, r.Predictions[j])
			}
		}
	}
}

func TestBacktestPipeline_MissingExtractors(t *testing.T) {
	bars := makeSyntheticBars(time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2005, 1, 10, 0, 0, 0, 0, time.UTC))

	// Missing ExtractFeatures
	pipeline := NewBacktestPipeline(bars)
	pipeline.ExtractLabels = makeCloseLabel
	_, err := pipeline.Run(&dummyModel{})
	if err == nil {
		t.Fatal("expected error for missing ExtractFeatures")
	}

	// Missing ExtractLabels
	pipeline = NewBacktestPipeline(bars)
	pipeline.ExtractFeatures = makeCloseFeature
	_, err = pipeline.Run(&dummyModel{})
	if err == nil {
		t.Fatal("expected error for missing ExtractLabels")
	}
}

func TestBacktestPipeline_EmptyData(t *testing.T) {
	pipeline := NewBacktestPipeline([]domain.DailyBar{})
	pipeline.ExtractFeatures = makeCloseFeature
	pipeline.ExtractLabels = makeCloseLabel
	_, err := pipeline.Run(&dummyModel{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestFilterBars(t *testing.T) {
	bars := makeSyntheticBars(
		time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2005, 1, 10, 0, 0, 0, 0, time.UTC),
	)
	// Inclusive range matches all.
	filtered := filterBars(bars, bars[0].Date, bars[len(bars)-1].Date)
	if len(filtered) != len(bars) {
		t.Errorf("expected %d bars, got %d", len(bars), len(filtered))
	}
	// Range outside data.
	filtered = filterBars(bars, time.Date(2006, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2006, 1, 2, 0, 0, 0, 0, time.UTC))
	if len(filtered) != 0 {
		t.Errorf("expected 0 bars, got %d", len(filtered))
	}
}

func TestExistingRunnerBackwardCompatible(t *testing.T) {
	// Verify the existing Runner type is unchanged and compiles.
	// (Full integration test requires real data — covered by window_test.go.)
	var r Runner
	_ = r // compiler-check only: Runner still exists
}

func TestBacktestPipeline_WithOLS(t *testing.T) {
	dataStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	dataEnd := time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC)
	bars := makeSyntheticBars(dataStart, dataEnd)

	pipeline := NewBacktestPipeline(bars)
	pipeline.ExtractFeatures = makeCloseFeature
	pipeline.ExtractLabels = makeCloseLabel

	model := &ml.OLS{FitIntercept: true}
	results, err := pipeline.Run(model)
	if err != nil {
		t.Fatalf("OLS Run: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result window")
	}

	for i, r := range results {
		if len(r.Predictions) == 0 {
			t.Errorf("result[%d]: predictions empty", i)
		}
		if len(r.Actuals) == 0 {
			t.Errorf("result[%d]: actuals empty", i)
		}
		if len(r.Predictions) != len(r.Actuals) {
			t.Errorf("result[%d]: predictions %d != actuals %d",
				i, len(r.Predictions), len(r.Actuals))
		}
		for j, p := range r.Predictions {
			if math.IsNaN(p) || math.IsInf(p, 0) {
				t.Errorf("result[%d].predictions[%d]: invalid value %f", i, j, p)
			}
		}
		if samples, ok := r.Metrics["train_samples"]; !ok || samples <= 0 {
			t.Errorf("result[%d]: train_samples missing or zero", i)
		}
	}
}

func TestBacktestPipeline_WithPCR(t *testing.T) {
	dataStart := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	dataEnd := time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC)
	bars := makeSyntheticBars(dataStart, dataEnd)

	pipeline := NewBacktestPipeline(bars)
	pipeline.ExtractFeatures = makeCloseFeature
	pipeline.ExtractLabels = makeCloseLabel

	model := &ml.PCR{NComponents: 3, VarianceThreshold: 0.95}
	results, err := pipeline.Run(model)
	if err != nil {
		t.Fatalf("PCR Run: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result window")
	}

	for i, r := range results {
		if len(r.Predictions) == 0 {
			t.Errorf("result[%d]: predictions empty", i)
		}
		if len(r.Actuals) == 0 {
			t.Errorf("result[%d]: actuals empty", i)
		}
		for j, p := range r.Predictions {
			if math.IsNaN(p) || math.IsInf(p, 0) {
				t.Errorf("result[%d].predictions[%d]: invalid value %f", i, j, p)
			}
		}
	}
}
