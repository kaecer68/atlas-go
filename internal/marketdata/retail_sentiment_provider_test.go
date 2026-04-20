package marketdata

import (
	"context"
	"math"
	"testing"
)

func TestTWSERetailSentimentProvider_Name(t *testing.T) {
	p := NewTWSERetailSentimentProvider("")
	if p.Name() != "twse_retail_sentiment" {
		t.Errorf("unexpected name: %s", p.Name())
	}
}

func TestTWSERetailSentimentProvider_CalculatePercentile(t *testing.T) {
	history := []float64{1000, 1200, 1100, 1300, 1400, 1500, 1600}
	p := &TWSERetailSentimentProvider{}

	percentile := p.calculatePercentile(1500, history)
	expected := 6.0 / 7.0
	if math.Abs(percentile-expected) > 1e-9 {
		t.Errorf("expected percentile %f, got %f", expected, percentile)
	}
}

func TestTWSERetailSentimentProvider_CalculatePercentile_EmptyHistory(t *testing.T) {
	p := &TWSERetailSentimentProvider{}
	percentile := p.calculatePercentile(1500, []float64{})
	if percentile != 0.5 {
		t.Errorf("expected 0.5 for empty history, got %f", percentile)
	}
}

func TestTWSERetailSentimentProvider_FetchSnapshot_Mock(t *testing.T) {
	p := NewTWSERetailSentimentProvider("")
	p.fetchMarginBalanceFunc = func(ctx context.Context) (float64, error) {
		return 1500.0, nil
	}
	p.fetchDayTradingRatioFunc = func(ctx context.Context) (float64, error) {
		return 0.25, nil
	}
	p.marginHistory = []float64{1000, 1200, 1100, 1300, 1400, 1500}

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap.MarginBalance != 1500 {
		t.Errorf("expected margin balance 1500, got %d", snap.MarginBalance)
	}
	if snap.MarginPercentile <= 0 {
		t.Errorf("expected positive percentile, got %f", snap.MarginPercentile)
	}
	if snap.DayTradingRatio != 0.25 {
		t.Errorf("expected day trading ratio 0.25, got %f", snap.DayTradingRatio)
	}
}
