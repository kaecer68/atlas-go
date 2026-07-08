package recommender

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestNewNarrativeAdapterFunc_NilNil_EmptyReturn(t *testing.T) {
	a := NewNarrativeAdapterFunc(nil, nil)
	if a == nil {
		t.Fatal("adapter must not be nil")
	}
	got, err := a.BuildMarketNarrativeData(context.Background())
	if err != nil {
		t.Errorf("nil-func BuildMarketNarrativeData should be graceful (no err), got %v", err)
	}
	if got.US10YChangeBps != 0 || got.VIXLevel != 0 || got.AICapexSentiment != 0 {
		t.Errorf("expected zero-value MarketNarrativeData, got %+v", got)
	}
	tcsi := a.GetCurrentStressIndex()
	if tcsi.Regime != "" || tcsi.Score != 0 {
		t.Errorf("expected zero-value TaiwanStressIndex, got %+v", tcsi)
	}
}

func TestNewCapitalFlowFunc_NilNil_Graceful(t *testing.T) {
	a := NewCapitalFlowFunc(nil)
	if a == nil {
		t.Fatal("adapter must not be nil")
	}
	got, err := a.LatestDaily(context.Background())
	if err != nil {
		t.Errorf("nil-func LatestDaily should be graceful, got %v", err)
	}
	if got.Summary != "" {
		t.Errorf("expected zero-value DailyReport, got %+v", got)
	}
}

func TestNewCapitalFlowFunc_PassThrough(t *testing.T) {
	want := capitalflow.DailyReport{
		Date:         time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		QualityLabel: "inflow",
		Summary:      "test pass-through",
	}
	a := NewCapitalFlowFunc(func(ctx context.Context) (capitalflow.DailyReport, error) {
		return want, nil
	})
	got, err := a.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	if got.Summary != want.Summary {
		t.Errorf("Summary = %q, want %q", got.Summary, want.Summary)
	}
}

func TestNewEventPredictorAdapter_NilPredictionReport_Graceful(t *testing.T) {
	a := NewEventPredictorAdapter(nil)
	if a == nil {
		t.Fatal("adapter must not be nil")
	}
	got, err := a.PredictToday()
	if err != nil {
		t.Errorf("nil predictor should be graceful, got %v", err)
	}
	if got.Direction != "" {
		t.Errorf("expected zero-value FlowPrediction, got %+v", got)
	}
	preds, err := a.NextNDays(5)
	if err != nil {
		t.Errorf("nil predictor NextNDays should be graceful, got %v", err)
	}
	if len(preds) != 0 {
		t.Errorf("expected empty slice, got %d items", len(preds))
	}
}

func TestNewEventPredictorAdapter_TakesFirstPrediction(t *testing.T) {
	p := &fakePredictor{
		report: eventdriven.PredictionReport{
			Predictions: []eventdriven.FlowPrediction{
				{Date: time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC), Direction: "inflow", Confidence: 0.7},
				{Date: time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC), Direction: "neutral", Confidence: 0.5},
			},
		},
	}
	a := NewEventPredictorAdapter(p)
	pred, err := a.PredictToday()
	if err != nil {
		t.Fatalf("PredictToday: %v", err)
	}
	if pred.Direction != "inflow" {
		t.Errorf("PredictToday.Direction = %q, want inflow", pred.Direction)
	}
	all, err := a.NextNDays(2)
	if err != nil {
		t.Fatalf("NextNDays: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("NextNDays returned %d, want 2", len(all))
	}
}

func TestNewComparisonEngineAdapter_NilProvider_Graceful(t *testing.T) {
	a := NewComparisonEngineAdapter(nil)
	if a == nil {
		t.Fatal("adapter must not be nil")
	}
	score, err := a.GetScore("growth")
	if err != nil {
		t.Errorf("nil provider should be graceful, got %v", err)
	}
	if score != 0 {
		t.Errorf("expected zero score, got %f", score)
	}
}

func TestNewComparisonEngineAdapter_PassThrough(t *testing.T) {
	a := NewComparisonEngineAdapter(fakeProvider{score: 0.85})
	got, err := a.GetScore("growth")
	if err != nil {
		t.Fatalf("GetScore: %v", err)
	}
	if got != 0.85 {
		t.Errorf("score = %f, want 0.85", got)
	}
}

// --- fakes ---

type fakePredictor struct {
	report eventdriven.PredictionReport
	err    error
}

func (f *fakePredictor) Predict(time.Time) eventdriven.PredictionReport {
	return f.report
}

type fakeProvider struct {
	score float64
	err   error
}

func (f fakeProvider) GetScore(string, int) (float64, error) {
	return f.score, f.err
}

var _ narrative.TaiwanStressIndex // doc-only reference to ensure import path stays
var _ = errors.New              // doc-only reference
