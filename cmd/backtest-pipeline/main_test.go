package main

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ml"
)

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
	bars := makeSynth(time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC))
	p := backtest.NewBacktestPipeline(bars)
	p.ExtractFeatures = func(b []domain.DailyBar) [][]float64 {
		f := make([][]float64, len(b))
		for i, bar := range b {
			f[i] = []float64{bar.Close}
		}
		return f
	}
	p.ExtractLabels = func(b []domain.DailyBar) []float64 {
		l := make([]float64, len(b))
		for i := 0; i < len(b)-1; i++ {
			if b[i].Close > 0 {
				l[i] = (b[i+1].Close - b[i].Close) / b[i].Close
			}
		}
		return l
	}
	results, err := p.Run(ml.NewOLS())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no windows")
	}
}

func TestOOSR2(t *testing.T) {
	if r2 := oosR2([]float64{1, 2, 3}, []float64{1, 2, 3}); r2 != 1.0 {
		t.Errorf("R2: %f", r2)
	}
	if r2 := oosR2([]float64{0, 0, 0}, []float64{1, 2, 3}); r2 > 0 {
		t.Errorf("R2: %f", r2)
	}
	if r2 := oosR2([]float64{5}, []float64{5}); !math.IsNaN(r2) {
		t.Errorf("R2: %f", r2)
	}
}

func TestAnnualizedSharpe(t *testing.T) {
	s := annualizedSharpe([]float64{0.01, 0.02, -0.01, 0.005}, []float64{0.01, 0.02, -0.01, 0.005})
	if math.IsNaN(s) {
		t.Error("NaN")
	}
	if s <= 0 {
		t.Errorf("s=%f", s)
	}
	if s := annualizedSharpe([]float64{1}, []float64{1}); !math.IsNaN(s) {
		t.Error("single NaN")
	}
}

func TestFeatureRegistry_AllDefined(t *testing.T) {
	for _, n := range []string{"close", "volume", "return_1d", "return_5d", "hl_ratio", "ma_ratio", "volume_ratio"} {
		if _, ok := featureRegistry[n]; !ok {
			t.Errorf("%q missing", n)
		}
	}
}

func TestNewModel_ValidNames(t *testing.T) {
	for _, n := range []string{"ols", "pcr", "pls", "elasticnet"} {
		m, err := newModel(n)
		if err != nil {
			t.Errorf("newModel(%q): %v", n, err)
		}
		if m == nil {
			t.Errorf("newModel(%q): nil", n)
		}
	}
}

func TestNewModel_InvalidName(t *testing.T) {
	m, err := newModel("xgboost")
	if err == nil {
		t.Error("expected error")
	}
	if m != nil {
		t.Error("expected nil")
	}
}

func TestPipelineWithPCR_CustomSplit(t *testing.T) {
	bars := makeSynth(time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC))
	p := backtest.NewBacktestPipeline(bars)
	p.Split.FirstTrainEnd = time.Date(2009, 12, 31, 0, 0, 0, 0, time.UTC)
	p.Split.ValidLengthYears = 1
	p.Split.StepYears = 2
	p.ExtractFeatures = func(b []domain.DailyBar) [][]float64 {
		f := make([][]float64, len(b))
		for i, bar := range b {
			f[i] = []float64{bar.Close}
		}
		return f
	}
	p.ExtractLabels = func(b []domain.DailyBar) []float64 {
		l := make([]float64, len(b))
		for i := 0; i < len(b)-1; i++ {
			if b[i].Close > 0 {
				l[i] = (b[i+1].Close - b[i].Close) / b[i].Close
			}
		}
		return l
	}
	results, err := p.Run(&ml.PCR{NComponents: 3, VarianceThreshold: 0.95})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) < 3 {
		t.Errorf("windows: %d", len(results))
	}
}

func TestWriteResultsCSV(t *testing.T) {
	bars := makeSynth(time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2022, 4, 30, 0, 0, 0, 0, time.UTC))
	p := backtest.NewBacktestPipeline(bars)
	p.ExtractFeatures = func(b []domain.DailyBar) [][]float64 {
		f := make([][]float64, len(b))
		for i, bar := range b {
			f[i] = []float64{bar.Close}
		}
		return f
	}
	p.ExtractLabels = func(b []domain.DailyBar) []float64 {
		l := make([]float64, len(b))
		for i := 0; i < len(b)-1; i++ {
			if b[i].Close > 0 {
				l[i] = (b[i+1].Close - b[i].Close) / b[i].Close
			}
		}
		return l
	}
	results, err := p.Run(ml.NewOLS())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	tmp := t.TempDir()
	if err := writeResultsCSV(results, []string{"close"}, tmp+"/r.csv"); err != nil {
		t.Fatalf("CSV: %v", err)
	}
}
