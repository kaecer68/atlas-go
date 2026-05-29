package main

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ml"
)

// makeSynth generates synthetic DailyBar data from start to end (inclusive).
func makeSynth(start, end time.Time) []domain.DailyBar {
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

func TestPipelineWithOLS_SyntheticData(t *testing.T) {
	bars := makeSynth(
		time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC),
	)

	pipeline := backtest.NewBacktestPipeline(bars)
	pipeline.ExtractFeatures = func(b []domain.DailyBar) [][]float64 {
		f := make([][]float64, len(b))
		for i, bar := range b {
			f[i] = []float64{bar.Close}
		}
		return f
	}
	pipeline.ExtractLabels = func(b []domain.DailyBar) []float64 {
		l := make([]float64, len(b))
		for i := 0; i < len(b)-1; i++ {
			if b[i].Close > 0 {
				l[i] = (b[i+1].Close - b[i].Close) / b[i].Close
			}
		}
		return l
	}

	model := ml.NewOLS()
	results, err := pipeline.Run(model)
	if err != nil {
		t.Fatalf("Run: %v", err)
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
		if r.WindowID == "" {
			t.Errorf("result[%d]: WindowID empty", i)
		}
		if samples, ok := r.Metrics["train_samples"]; !ok || samples <= 0 {
			t.Errorf("result[%d]: train_samples missing or zero", i)
		}
		if samples, ok := r.Metrics["test_samples"]; !ok || samples <= 0 {
			t.Errorf("result[%d]: test_samples missing or zero", i)
		}
		for j, p := range r.Predictions {
			if math.IsNaN(p) || math.IsInf(p, 0) {
				t.Errorf("result[%d].predictions[%d]: invalid value %f", i, j, p)
			}
		}
	}
}

func TestOOSR2(t *testing.T) {
	// Perfect prediction.
	if r2 := oosR2([]float64{1, 2, 3}, []float64{1, 2, 3}); r2 != 1.0 {
		t.Errorf("perfect R²: expected 1.0, got %f", r2)
	}

	// Mean prediction (R² should be 0).
	pred := []float64{0.0, 0.0, 0.0}
	act := []float64{1.0, 2.0, 3.0}
	if r2 := oosR2(pred, act); r2 > 0 {
		t.Errorf("mean-only R²: expected ≤0, got %f", r2)
	}

	// Single sample → NaN.
	if r2 := oosR2([]float64{5}, []float64{5}); !math.IsNaN(r2) {
		t.Errorf("single sample: expected NaN, got %f", r2)
	}
}

func TestAnnualizedSharpe(t *testing.T) {
	// All positive returns.
	pred := []float64{0.01, 0.02, -0.01, 0.005}
	act := []float64{0.01, 0.02, -0.01, 0.005}
	s := annualizedSharpe(pred, act)
	if math.IsNaN(s) {
		t.Error("sharpe should not be NaN")
	}
	if s <= 0 {
		t.Errorf("sharpe should be positive, got %f", s)
	}

	// Single sample → NaN.
	if s := annualizedSharpe([]float64{1}, []float64{1}); !math.IsNaN(s) {
		t.Errorf("single sample: expected NaN, got %f", s)
	}
}

func TestFeatureRegistry_AllDefined(t *testing.T) {
	// Verify that known feature names exist in registry.
	for _, name := range []string{"close", "volume", "return_1d", "return_5d", "hl_ratio"} {
		if _, ok := featureRegistry[name]; !ok {
			t.Errorf("feature %q not found in registry", name)
		}
	}
	// Verify that unknown features are caught.
	unknown := validateFeatures([]string{"close", "bogus_feature", "return_1d"})
	if len(unknown) != 1 || unknown[0] != "bogus_feature" {
		t.Errorf("expected [bogus_feature], got %v", unknown)
	}
}
