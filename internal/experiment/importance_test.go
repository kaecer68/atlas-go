package experiment

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func makeSynthBars(n int) []domain.DailyBar {
	bars := make([]domain.DailyBar, n)
	start := time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		dayReturn := (rand.Float64() - 0.5) * 0.04 // [-2%, +2%] daily
		close := 100.0
		if i > 0 {
			close = bars[i-1].Close * (1.0 + dayReturn)
		}
		bars[i] = domain.DailyBar{
			Date:   start.AddDate(0, 0, i),
			Symbol: "TEST",
			Close:  close,
			High:   close * (1.0 + rand.Float64()*0.02),
			Low:    close * (1.0 - rand.Float64()*0.02),
			Volume: int64(500000 + rand.Int63n(2000000)),
		}
	}
	return bars
}

func TestComputeImportanceFromBars(t *testing.T) {
	bars := makeSynthBars(200)
	p := NewFactorPredictor()

	result, err := p.ComputeImportanceFromBars(bars, []string{"close", "volume"}, 5)
	if err != nil {
		t.Fatalf("ComputeImportanceFromBars: %v", err)
	}

	if len(result.FeatureNames) != 2 {
		t.Errorf("expected 2 feature names, got %d", len(result.FeatureNames))
	}
	if result.FeatureNames[0] != "close" || result.FeatureNames[1] != "volume" {
		t.Errorf("feature names: %v", result.FeatureNames)
	}
	if len(result.Importances) != 2 {
		t.Errorf("expected 2 importances, got %d", len(result.Importances))
	}
	if len(result.Ranks) != 2 {
		t.Errorf("expected 2 ranks, got %d", len(result.Ranks))
	}
	for _, imp := range result.Importances {
		if math.IsNaN(imp) || math.IsInf(imp, 0) {
			t.Errorf("importances contain invalid value: %f", imp)
		}
	}
}

func TestComputeImportanceFromBars_EmptyInput(t *testing.T) {
	p := NewFactorPredictor()

	_, err := p.ComputeImportanceFromBars(nil, []string{"close"}, 5)
	if err == nil {
		t.Error("expected error for empty bars")
	}

	bars := makeSynthBars(10)
	_, err = p.ComputeImportanceFromBars(bars, nil, 5)
	if err == nil {
		t.Error("expected error for empty feature names")
	}
}

func TestNewFactorPredictor_SatisfiesPredictor(t *testing.T) {
	p := NewFactorPredictor()
	if p == nil {
		t.Fatal("NewFactorPredictor returned nil")
	}

	// Train and predict on tiny synthetic data.
	p.Fit([][]float64{{1.0}, {2.0}, {3.0}}, []float64{2.0, 3.0, 4.0})
	pred, err := p.Predict([][]float64{{1.5}})
	if err != nil {
		t.Fatalf("Predict: %v", err)
	}
	if len(pred) != 1 {
		t.Errorf("expected 1 prediction, got %d", len(pred))
	}
}
