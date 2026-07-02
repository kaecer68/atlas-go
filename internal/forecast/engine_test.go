package forecast

import (
	"testing"
	"time"
)

func TestForecastEngine_Predict_ReturnsResult(t *testing.T) {
	eng := NewForecastEngine()
	result, err := eng.Predict("2330.TW", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("Predict returned unexpected error: %v", err)
	}
	if result.Symbol != "2330.TW" {
		t.Errorf("Symbol = %q, want %q", result.Symbol, "2330.TW")
	}
	if result.Horizon != "7d" {
		t.Errorf("Horizon = %q, want %q", result.Horizon, "7d")
	}
	if result.Direction != DirectionBullish && result.Direction != DirectionBearish && result.Direction != DirectionHold {
		t.Errorf("Direction = %q, want a valid Direction", result.Direction)
	}
	if result.GeneratedAt.IsZero() {
		t.Error("GeneratedAt must be set")
	}
}

func TestForecastEngine_Predict_ConvictionInRange(t *testing.T) {
	eng := NewForecastEngine()
	result, err := eng.Predict("2317.TW", 30*24*time.Hour)
	if err != nil {
		t.Fatalf("Predict returned unexpected error: %v", err)
	}
	if result.Conviction < 0 || result.Conviction > 100 {
		t.Errorf("Conviction = %d, want [0,100]", result.Conviction)
	}
}

func TestForecastEngine_Predict_EmptySymbol(t *testing.T) {
	eng := NewForecastEngine()
	_, err := eng.Predict("", 7*24*time.Hour)
	if err == nil {
		t.Error("Predict with empty symbol must return an error")
	}
}
