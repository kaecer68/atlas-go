package forecast_bridge

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/forecast"
)

func TestAdapter_ToTradeSignal_bullish_strong(t *testing.T) {
	adapter := NewAdapter()
	result := forecast.ForecastResult{
		Symbol:      "2330.TW",
		Conviction:  75,
		Direction:   forecast.DirectionBullish,
		Horizon:     "7d",
		GeneratedAt: time.Unix(1700000000, 0),
	}

	signal, err := adapter.ToTradeSignal(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if signal.Symbol != result.Symbol {
		t.Errorf("Symbol = %q, want %q", signal.Symbol, result.Symbol)
	}
	if signal.Action != "buy" {
		t.Errorf("Action = %q, want %q", signal.Action, "buy")
	}
	if signal.WeightMultiplier != 1.2 {
		t.Errorf("WeightMultiplier = %v, want 1.2", signal.WeightMultiplier)
	}
	wantID := "2330.TW_1700000000"
	if signal.SourceForecastID != wantID {
		t.Errorf("SourceForecastID = %q, want %q", signal.SourceForecastID, wantID)
	}
	if signal.GeneratedAt != result.GeneratedAt {
		t.Errorf("GeneratedAt = %v, want %v", signal.GeneratedAt, result.GeneratedAt)
	}
}

func TestAdapter_ToTradeSignal_bearish_strong(t *testing.T) {
	adapter := NewAdapter()
	result := forecast.ForecastResult{
		Symbol:      "2317.TW",
		Conviction:  25,
		Direction:   forecast.DirectionBearish,
		Horizon:     "7d",
		GeneratedAt: time.Unix(1700000001, 0),
	}

	signal, err := adapter.ToTradeSignal(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if signal.Action != "sell" {
		t.Errorf("Action = %q, want %q", signal.Action, "sell")
	}
	if signal.WeightMultiplier != 0.8 {
		t.Errorf("WeightMultiplier = %v, want 0.8", signal.WeightMultiplier)
	}
}

func TestAdapter_ToTradeSignal_hold_neutral(t *testing.T) {
	adapter := NewAdapter()
	result := forecast.ForecastResult{
		Symbol:      "2454.TW",
		Conviction:  50,
		Direction:   forecast.DirectionHold,
		Horizon:     "7d",
		GeneratedAt: time.Unix(1700000002, 0),
	}

	signal, err := adapter.ToTradeSignal(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if signal.Action != "hold" {
		t.Errorf("Action = %q, want %q", signal.Action, "hold")
	}
	if signal.WeightMultiplier != 1.0 {
		t.Errorf("WeightMultiplier = %v, want 1.0", signal.WeightMultiplier)
	}
}

func TestAdapter_ToTradeSignal_threshold_upper(t *testing.T) {
	adapter := NewAdapter()
	result := forecast.ForecastResult{
		Symbol:      "2303.TW",
		Conviction:  70,
		Direction:   forecast.DirectionBullish,
		Horizon:     "7d",
		GeneratedAt: time.Unix(1700000003, 0),
	}

	signal, err := adapter.ToTradeSignal(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if signal.Action != "buy" {
		t.Errorf("Action = %q, want %q", signal.Action, "buy")
	}
	if signal.WeightMultiplier != 1.2 {
		t.Errorf("WeightMultiplier = %v, want 1.2", signal.WeightMultiplier)
	}
}

func TestAdapter_ToTradeSignal_threshold_lower(t *testing.T) {
	adapter := NewAdapter()
	result := forecast.ForecastResult{
		Symbol:      "2881.TW",
		Conviction:  30,
		Direction:   forecast.DirectionBearish,
		Horizon:     "7d",
		GeneratedAt: time.Unix(1700000004, 0),
	}

	signal, err := adapter.ToTradeSignal(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if signal.Action != "sell" {
		t.Errorf("Action = %q, want %q", signal.Action, "sell")
	}
	if signal.WeightMultiplier != 0.8 {
		t.Errorf("WeightMultiplier = %v, want 0.8", signal.WeightMultiplier)
	}
}
