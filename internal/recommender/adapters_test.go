package recommender

import (
	"context"
	"errors"
	"reflect"
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
	a := NewCapitalFlowFunc(nil, nil, nil)
	if a == nil {
		t.Fatal("adapter must not be nil")
	}
	daily, err := a.LatestDaily(context.Background())
	if err != nil {
		t.Errorf("nil-func LatestDaily should be graceful, got %v", err)
	}
	if daily.Summary != "" {
		t.Errorf("expected zero-value DailyReport, got %+v", daily)
	}
	summary, err := a.Summary(context.Background())
	if err != nil {
		t.Errorf("nil-func Summary should be graceful, got %v", err)
	}
	if summary.DominantForce != "" || summary.QualityLabel != "" {
		t.Errorf("expected zero-value SummaryReport, got %+v", summary)
	}
	assessment, err := a.LatestAssessment(context.Background())
	if err != nil {
		t.Errorf("nil-func LatestAssessment should be graceful, got %v", err)
	}
	if !reflect.DeepEqual(assessment, capitalflow.CapitalFlowAssessment{}) {
		t.Errorf("expected zero-value CapitalFlowAssessment, got %+v", assessment)
	}
}

func TestNewCapitalFlowFunc_PassThrough(t *testing.T) {
	wantAssessment := capitalflow.CapitalFlowAssessment{
		AsOfTradingDate:   "2026-07-08",
		CalibrationStatus: capitalflow.CalibrationEligible,
		Institutional: capitalflow.DirectionalAssessment{
			Available: true,
			Direction: "bullish",
			Aligned:   []capitalflow.ForceName{capitalflow.ForceForeign},
		},
	}
	wantDaily := capitalflow.DailyReport{
		Date:         time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		QualityLabel: "inflow",
		Summary:      "test pass-through",
		Assessment:   wantAssessment,
	}
	wantSummary := capitalflow.SummaryReport{
		Date:          wantDaily.Date,
		QualityLabel:  "inflow",
		DominantForce: capitalflow.ForceForeign,
		Summary:       "foreign-led inflow",
		Assessment:    wantAssessment,
	}
	a := NewCapitalFlowFunc(
		func(ctx context.Context) (capitalflow.DailyReport, error) { return wantDaily, nil },
		func(ctx context.Context) (capitalflow.SummaryReport, error) { return wantSummary, nil },
		func(ctx context.Context) (capitalflow.CapitalFlowAssessment, error) { return wantAssessment, nil },
	)
	gotDaily, err := a.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	if gotDaily.Summary != wantDaily.Summary {
		t.Errorf("DailyReport.Summary = %q, want %q", gotDaily.Summary, wantDaily.Summary)
	}
	gotSummary, err := a.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if gotSummary.DominantForce != wantSummary.DominantForce {
		t.Errorf("SummaryReport.DominantForce = %q, want %q", gotSummary.DominantForce, wantSummary.DominantForce)
	}
	gotAssessment, err := a.LatestAssessment(context.Background())
	if err != nil {
		t.Fatalf("LatestAssessment: %v", err)
	}
	if !reflect.DeepEqual(gotAssessment, wantAssessment) {
		t.Errorf("LatestAssessment = %+v, want %+v", gotAssessment, wantAssessment)
	}
}

func TestNewCapitalFlowFunc_LatestOnlyOptIn(t *testing.T) {
	wantDaily := capitalflow.DailyReport{
		Date:    time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		Summary: "latest only",
	}
	a := NewCapitalFlowFunc(
		func(ctx context.Context) (capitalflow.DailyReport, error) { return wantDaily, nil },
		nil,
		nil,
	)
	got, err := a.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("LatestDaily: %v", err)
	}
	if got.Summary != wantDaily.Summary {
		t.Errorf("LatestDaily.Summary = %q, want %q", got.Summary, wantDaily.Summary)
	}
	summary, err := a.Summary(context.Background())
	if err != nil {
		t.Errorf("nil Summary func should be graceful, got %v", err)
	}
	if summary.DominantForce != "" {
		t.Errorf("Summary fallback should be zero-value, got %+v", summary)
	}
}

func TestNewCapitalFlowAdapter_NilProvider_Graceful(t *testing.T) {
	a := NewCapitalFlowAdapter(nil)
	if a == nil {
		t.Fatal("adapter must not be nil")
	}
	daily, err := a.LatestDaily(context.Background())
	if err != nil {
		t.Errorf("nil provider LatestDaily should be graceful, got %v", err)
	}
	if daily.Summary != "" {
		t.Errorf("expected zero-value DailyReport, got %+v", daily)
	}
	summary, err := a.Summary(context.Background())
	if err != nil {
		t.Errorf("nil provider Summary should be graceful, got %v", err)
	}
	if summary.DominantForce != "" {
		t.Errorf("expected zero-value SummaryReport, got %+v", summary)
	}
	assessment, err := a.LatestAssessment(context.Background())
	if err != nil {
		t.Errorf("nil provider LatestAssessment should be graceful, got %v", err)
	}
	if !reflect.DeepEqual(assessment, capitalflow.CapitalFlowAssessment{}) {
		t.Errorf("expected zero-value CapitalFlowAssessment, got %+v", assessment)
	}
}

func TestNewCapitalFlowAdapter_SummaryAndAssessmentPassThrough(t *testing.T) {
	p := &fakeCapitalFlowProvider{
		summary: capitalflow.SummaryReport{
			Date:          time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
			QualityLabel:  "strong_inflow",
			DominantForce: capitalflow.ForceForeign,
		},
		assessment: capitalflow.CapitalFlowAssessment{
			CalibrationStatus: capitalflow.CalibrationEligible,
			Reasons:           []string{"validated"},
		},
	}
	a := NewCapitalFlowAdapter(p)
	got, err := a.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.DominantForce != p.summary.DominantForce {
		t.Errorf("DominantForce = %q, want %q", got.DominantForce, p.summary.DominantForce)
	}
	if got.QualityLabel != p.summary.QualityLabel {
		t.Errorf("QualityLabel = %q, want %q", got.QualityLabel, p.summary.QualityLabel)
	}
	assessment, err := a.LatestAssessment(context.Background())
	if err != nil {
		t.Fatalf("LatestAssessment: %v", err)
	}
	if !reflect.DeepEqual(assessment, p.assessment) {
		t.Errorf("LatestAssessment = %+v, want %+v", assessment, p.assessment)
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

type fakeCapitalFlowProvider struct {
	daily         capitalflow.DailyReport
	dailyErr      error
	summary       capitalflow.SummaryReport
	summaryErr    error
	assessment    capitalflow.CapitalFlowAssessment
	assessmentErr error
}

func (f *fakeCapitalFlowProvider) LatestDaily(context.Context) (capitalflow.DailyReport, error) {
	return f.daily, f.dailyErr
}

func (f *fakeCapitalFlowProvider) Summary(context.Context) (capitalflow.SummaryReport, error) {
	return f.summary, f.summaryErr
}

func (f *fakeCapitalFlowProvider) LatestAssessment(context.Context) (capitalflow.CapitalFlowAssessment, error) {
	return f.assessment, f.assessmentErr
}

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

var (
	_ narrative.TaiwanStressIndex // doc-only reference to ensure import path stays
	_ = errors.New                // doc-only reference
)
