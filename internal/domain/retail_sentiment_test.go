package domain

import (
	"math"
	"testing"
	"time"
)

func TestRetailSentimentSnapshot_CalculateSentimentScore(t *testing.T) {
	tests := []struct {
		name       string
		percentile float64
		want       float64
	}{
		{"neutral midpoint", 0.5, 0.0},
		{"high percentile", 0.75, 0.5},
		{"low percentile", 0.25, -0.5},
		{"max frenzy", 1.0, 1.0},
		{"max fear", 0.0, -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := RetailSentimentSnapshot{MarginPercentile: tt.percentile}
			got := rs.CalculateSentimentScore()
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("CalculateSentimentScore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetailSentimentSnapshot_ExtremeReading(t *testing.T) {
	tests := []struct {
		name       string
		percentile float64
		want       string
	}{
		{"frenzy at 90", 0.90, "frenzy"},
		{"frenzy above 90", 0.95, "frenzy"},
		{"fear at 10", 0.10, "fear"},
		{"fear below 10", 0.05, "fear"},
		{"neutral middle", 0.50, "neutral"},
		{"neutral near frenzy", 0.89, "neutral"},
		{"neutral near fear", 0.11, "neutral"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := RetailSentimentSnapshot{MarginPercentile: tt.percentile}
			if got := rs.ExtremeReading(); got != tt.want {
				t.Errorf("ExtremeReading() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetailSentimentSnapshot_StructTags(t *testing.T) {
	rs := RetailSentimentSnapshot{
		MarginBalance:    1500,
		MarginChangePct:  0.05,
		DayTradingRatio:  0.25,
		RetailFuturesOI:  10000,
		MarginPercentile: 0.85,
		SentimentScore:   0.7,
		Timestamp:        time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
	}

	_ = rs.MarginBalance
	_ = rs.MarginChangePct
	_ = rs.DayTradingRatio
	_ = rs.RetailFuturesOI
	_ = rs.MarginPercentile
	_ = rs.SentimentScore
	_ = rs.Timestamp
}
