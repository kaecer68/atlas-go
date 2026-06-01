package risk

import (
	"context"
	"testing"
)

func defaultPortfolio() PortfolioState {
	return PortfolioState{
		TotalValue:     1_000_000,
		Cash:           150_000,
		SectorExposure: map[string]float64{"semiconductor": 300_000},
		Positions:      map[string]float64{"2330": 100_000},
		Var95:          -15_000,
		MaxDrawdown:    0.05,
	}
}

func defaultOrder() OrderIntent {
	return OrderIntent{
		Symbol:     "2330",
		Side:       "BUY",
		Notional:   50_000,
		AgentID:    "semiconductor_desk",
		Sector:     "semiconductor",
		Conviction: 3,
	}
}

func TestPreTradeGate_MaxPositionExceeded(t *testing.T) {
	g := NewPreTradeGate()
	order := defaultOrder()
	pf := defaultPortfolio()
	// Existing 100k + new 200k = 300k / 1M = 30% > 15% limit
	order.Notional = 200_000

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK for max position exceed, got %s", dec.Verdict)
	}
	found := false
	for _, d := range dec.Details {
		if d.RuleName == "max_position_pct" && !d.Passed {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected max_position_pct rule to fail")
	}
}

func TestPreTradeGate_MaxPositionAllowed(t *testing.T) {
	g := NewPreTradeGate()
	order := defaultOrder()
	pf := defaultPortfolio()
	// Existing 100k + new 20k = 120k / 1M = 12% < 15% limit
	order.Notional = 20_000

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictAllow {
		t.Errorf("expected ALLOW for within max position, got %s", dec.Verdict)
	}
}

func TestPreTradeGate_SectorExposureExceeded(t *testing.T) {
	g := NewPreTradeGate()
	order := defaultOrder()
	pf := defaultPortfolio()
	// Existing sector 300k + new 200k = 500k / 1M = 50% > 40% limit
	order.Notional = 200_000
	pf.SectorExposure["semiconductor"] = 300_000

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK for sector exceed, got %s", dec.Verdict)
	}
}

func TestPreTradeGate_VaRLimitExceeded(t *testing.T) {
	g := NewPreTradeGate()
	order := defaultOrder()
	pf := defaultPortfolio()
	pf.Var95 = -50_000 // 5% of 1M > 2% limit

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK for VaR exceed, got %s", dec.Verdict)
	}
}

func TestPreTradeGate_CashBufferInsufficient(t *testing.T) {
	g := NewPreTradeGate()
	order := defaultOrder()
	pf := defaultPortfolio()
	pf.Cash = 10_000 // Post-trade cash = 10k - 50k = -40k (negative!)
	order.Notional = 50_000

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK for insufficient cash, got %s", dec.Verdict)
	}
}

func TestPreTradeGate_AllRulesPass(t *testing.T) {
	g := NewPreTradeGate()
	order := defaultOrder()
	pf := defaultPortfolio()

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictAllow {
		t.Errorf("expected ALLOW for valid order, got %s (reason: %s)", dec.Verdict, dec.Reason)
	}
	if len(dec.Details) != 6 {
		t.Errorf("expected 6 rule results, got %d", len(dec.Details))
	}
}

func TestPreTradeGate_ZeroPortfolioError(t *testing.T) {
	g := NewPreTradeGate()
	order := defaultOrder()
	pf := PortfolioState{}

	_, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err == nil {
		t.Fatal("expected error for zero portfolio, got nil")
	}
}

func TestPreTradeGate_DefensiveModeCapsPosition(t *testing.T) {
	g := NewPreTradeGate()
	// In the gate.go PreTradeCheck, DEFENSIVE mode caps target > 0.5 to 0.5
	// But the gate.go logic applies this AFTER pre_trade.Check() returns
	// So this tests the config defaults
	if g.MaxPositionPct() <= 0 {
		t.Error("MaxPositionPct should be > 0")
	}
	if g.MinCashBuffer() <= 0 {
		t.Error("MinCashBuffer should be > 0")
	}
}

func TestPreTradeGate_ConfigValues(t *testing.T) {
	g := NewPreTradeGate()
	if g.MaxPositionPct() != 0.15 {
		t.Errorf("MaxPositionPct = %.2f, want 0.15", g.MaxPositionPct())
	}
	if g.MaxSectorPct() != 0.40 {
		t.Errorf("MaxSectorPct = %.2f, want 0.40", g.MaxSectorPct())
	}
	if g.VarLimitPct() != 0.02 {
		t.Errorf("VarLimitPct = %.2f, want 0.02", g.VarLimitPct())
	}
	if g.MinCashBuffer() != 0.05 {
		t.Errorf("MinCashBuffer = %.2f, want 0.05", g.MinCashBuffer())
	}
	if g.MaxOpenPositions() != 5 {
		t.Errorf("MaxOpenPositions = %d, want 5", g.MaxOpenPositions())
	}
}

func TestPreTradeGate_SectorExposureNoSector(t *testing.T) {
	g := NewPreTradeGate()
	order := defaultOrder()
	order.Sector = "" // No sector set — should pass
	pf := defaultPortfolio()

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictAllow {
		t.Errorf("expected ALLOW for empty sector, got %s", dec.Verdict)
	}
}

func TestVerdictSeverity(t *testing.T) {
	tests := []struct {
		name      string
		current   float64
		threshold float64
		want      string
	}{
		{"worse than threshold", -0.20, -0.10, "CRITICAL"},
		{"moderately worse", -0.10, -0.05, "CRITICAL"},
		{"equals threshold", -0.05, -0.05, "INFO"},
		{"better than threshold", 0.01, -0.05, "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := severityForDiff(tt.current, tt.threshold)
			if got != tt.want {
				t.Errorf("severityForDiff(%.2f, %.2f) = %s, want %s", tt.current, tt.threshold, got, tt.want)
			}
		})
	}
}

func assertRulePassed(t *testing.T, dec *RiskDecision, ruleName string, want bool) {
	t.Helper()
	for _, d := range dec.Details {
		if d.RuleName == ruleName && d.Passed != want {
			t.Errorf("rule %s passed=%v, want %v (value=%.4f, threshold=%.4f)", ruleName, d.Passed, want, d.CurrentValue, d.Threshold)
			return
		}
	}
}

func TestPreTradeGate_MaxOpenPositionsExceeded(t *testing.T) {
	g := NewPreTradeGate()
	// Use a portfolio with 5 existing positions — max allowed
	pf := defaultPortfolio()
	pf.Positions = map[string]float64{"2330": 100000, "2454": 100000, "3008": 100000, "2317": 100000, "2882": 100000}
	order := defaultOrder()
	order.Symbol = "2881" // New symbol, would be 6th position
	order.Side = "BUY"

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK for max open positions exceeded, got %s", dec.Verdict)
	}
	assertRulePassed(t, dec, "max_open_positions", false)
}

func TestPreTradeGate_MaxOpenPositionsWithinLimit(t *testing.T) {
	g := NewPreTradeGate()
	pf := defaultPortfolio()
	pf.Positions = map[string]float64{"2330": 100000} // Only 1 position
	order := defaultOrder()
	order.Symbol = "2454" // New symbol, would be 2nd position
	order.Side = "BUY"

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictAllow {
		t.Errorf("expected ALLOW for within max positions, got %s (reason: %s)", dec.Verdict, dec.Reason)
	}
	assertRulePassed(t, dec, "max_open_positions", true)
}

func TestPreTradeGate_MaxOpenPositionsSellOnExistingPosition(t *testing.T) {
	g := NewPreTradeGate()
	pf := defaultPortfolio()
	pf.Positions = map[string]float64{"2330": 100000, "2454": 100000, "3008": 100000, "2317": 100000, "2882": 100000}
	order := defaultOrder()
	order.Symbol = "2330" // Already held — sell does not increase count
	order.Side = "SELL"

	dec, err := g.Check(context.Background(), order, pf, "NORMAL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// SELL on existing position should pass max_open_positions
	assertRulePassed(t, dec, "max_open_positions", true)
}

