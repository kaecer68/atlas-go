package risk

import (
	"context"
	"testing"
)

func pos(symbol string, entry, current, highest float64, atr float64, qty int) InTradePosition {
	unrealized := (current - entry) / entry
	return InTradePosition{
		Symbol:           symbol,
		EntryPrice:       entry,
		CurrentPrice:     current,
		Quantity:         qty,
		UnrealizedPnLPct: unrealized,
		ATR:              atr,
		HighestPrice:     highest,
	}
}

func TestInTradeGate_StopLossTriggered(t *testing.T) {
	g := NewInTradeGate()
	pos := pos("2330", 100.0, 85.0, 100.0, 0, 1000) // -15% loss

	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 0.2, -0.01, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK for stop-loss, got %s", dec.Verdict)
	}
	if dec.Action.Type != ActionSell {
		t.Errorf("expected SELL action, got %s", dec.Action.Type)
	}
	assertRulePassed(t, dec, "stop_loss", false)
}

func TestInTradeGate_StopLossNotTriggered(t *testing.T) {
	g := NewInTradeGate()
	pos := pos("2330", 100.0, 95.0, 100.0, 0, 1000) // -5% loss, within stop-loss

	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 0.2, -0.01, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict == VerdictBlock {
		t.Errorf("unexpected BLOCK, position within stop-loss threshold")
	}
	assertRulePassed(t, dec, "stop_loss", true)
}

func TestInTradeGate_TakeProfitTriggered(t *testing.T) {
	g := NewInTradeGate()
	pos := pos("2330", 100.0, 140.0, 140.0, 0, 1000) // +40% gain

	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 0.2, -0.01, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictReduce {
		t.Errorf("expected REDUCE for take-profit, got %s", dec.Verdict)
	}
	assertRulePassed(t, dec, "take_profit", false)
}

func TestInTradeGate_TrailingStopTriggered(t *testing.T) {
	g := NewInTradeGate()
	// Stock went from 100 to 150 (50% runup), then dropped back to 115
	// Trail level = 1 - (10*2)/100 = 1 - 0.2 = 0.8
	// Current ratio = 115/150 = 0.767 < 0.8 => triggered!
	pos := pos("2330", 100.0, 115.0, 150.0, 10.0, 1000)

	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 0.2, -0.01, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK for trailing-stop, got %s (reason: %s)", dec.Verdict, dec.Reason)
	}
}

func TestInTradeGate_TrailingStopNoRunup(t *testing.T) {
	g := NewInTradeGate()
	pos := pos("2330", 100.0, 102.0, 101.0, 5.0, 1000) // Only 1% runup

	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 0.2, -0.01, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRulePassed(t, dec, "trailing_stop", true)
}

func TestInTradeGate_VolatilitySpike(t *testing.T) {
	g := NewInTradeGate()
	pos := pos("2330", 100.0, 101.0, 101.0, 0, 1000)

	// currentVol=1.0, histVol=0.2 => ratio=5x > 3x threshold
	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 1.0, -0.01, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRulePassed(t, dec, "volatility_spike", false)
}

func TestInTradeGate_CircuitBreakerTriggered(t *testing.T) {
	g := NewInTradeGate()
	pos := pos("2330", 100.0, 101.0, 101.0, 0, 1000)

	// Daily loss -8% exceeds -5% circuit breaker
	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 0.2, -0.08, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictHalt {
		t.Errorf("expected HALT for circuit breaker, got %s", dec.Verdict)
	}
	assertRulePassed(t, dec, "circuit_breaker", false)
}

func TestInTradeGate_CircuitBreakerNotTriggered(t *testing.T) {
	g := NewInTradeGate()
	pos := pos("2330", 100.0, 101.0, 101.0, 0, 1000)

	// Daily loss -2% within -5% limit
	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 0.2, -0.02, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertRulePassed(t, dec, "circuit_breaker", true)
}

func TestInTradeGate_AllNormal(t *testing.T) {
	g := NewInTradeGate()
	pos := pos("2330", 100.0, 102.0, 103.0, 2.0, 1000) // +2%, small vol

	dec, err := g.Evaluate(context.Background(), []InTradePosition{pos}, 0.2, 0.2, -0.01, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictAllow {
		t.Errorf("expected ALLOW for normal position, got %s", dec.Verdict)
	}
}

func TestInTradeGate_MultiplePositionsOneStopLoss(t *testing.T) {
	g := NewInTradeGate()
	healthy := pos("2330", 100.0, 105.0, 106.0, 0, 1000) // +5% gain
	bad := pos("2317", 100.0, 82.0, 100.0, 0, 500)       // -18% loss

	dec, err := g.Evaluate(context.Background(), []InTradePosition{healthy, bad}, 0.2, 0.2, -0.01, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK due to stop-loss, got %s", dec.Verdict)
	}
}
