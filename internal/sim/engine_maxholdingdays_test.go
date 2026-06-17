package sim

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestRunDay_ForceCloseAtMaxHoldingDays verifies that a Position held longer
// than MaxHoldingDays gets force-closed at the start of the next trading day
// with a SELL trade carrying Reason="max_holding_days".
func TestRunDay_ForceCloseAtMaxHoldingDays(t *testing.T) {
	day0 := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	constraints := domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           0.5,
		MaxOpenPositions:            5,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 1,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
		MaxHoldingDays:              5,
	}
	engine := NewEngine(constraints)
	state := domain.NewSimulationState(constraints.StartingCash)

	quotes := []domain.Quote{{
		Symbol: "2330", Last: 100.0, Open: 100.0, Volume: 1_000_000, IsTradable: true,
	}}
	recs := []domain.Recommendation{{
		Agent: "test", Symbol: "2330", Side: domain.SideBuy, Conviction: 5, Reason: "buy",
	}}

	// Day 0: BUY
	_ = engine.RunDay(&state, day0, domain.RegimeRiskOn, quotes, recs)
	if len(state.Positions) != 1 || state.Positions[0].Symbol != "2330" {
		t.Fatalf("day0: expected position 2330, got %d positions", len(state.Positions))
	}

	// Days 1..5: position still held (5 days <= MaxHoldingDays=5)
	for d := 1; d <= 5; d++ {
		day := day0.AddDate(0, 0, d)
		dayResult := engine.RunDay(&state, day, domain.RegimeRiskOn, quotes, nil)
		if d < 6 {
			if len(state.Positions) != 1 {
				t.Fatalf("day%d: expected 1 position, got %d (force-closed too early)", d, len(state.Positions))
			}
		}
		_ = dayResult
	}

	// Day 6: position held 6 days > MaxHoldingDays=5 → force close
	day6 := day0.AddDate(0, 0, 6)
	dayResult := engine.RunDay(&state, day6, domain.RegimeRiskOn, quotes, nil)

	if len(state.Positions) != 0 {
		t.Fatalf("day6: expected force close, still have %d positions: %+v", len(state.Positions), state.Positions)
	}
	foundForceClose := false
	for _, trade := range dayResult.Trades {
		if trade.Symbol == "2330" && trade.Side == domain.SideSell && trade.Reason == "max_holding_days" {
			foundForceClose = true
		}
	}
	if !foundForceClose {
		t.Errorf("day6: expected SELL trade with reason=max_holding_days, got trades: %+v", dayResult.Trades)
	}
}

// TestRunDay_ZeroMaxHoldingDaysSkipsForceClose documents that MaxHoldingDays=0
// disables the force-close behavior entirely (legacy mode).
func TestRunDay_ZeroMaxHoldingDaysSkipsForceClose(t *testing.T) {
	day0 := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	constraints := domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           0.5,
		MaxOpenPositions:            5,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 1,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
		MaxHoldingDays:              0,
	}
	engine := NewEngine(constraints)
	state := domain.NewSimulationState(constraints.StartingCash)

	quotes := []domain.Quote{{
		Symbol: "2330", Last: 100.0, Open: 100.0, Volume: 1_000_000, IsTradable: true,
	}}
	recs := []domain.Recommendation{{
		Agent: "test", Symbol: "2330", Side: domain.SideBuy, Conviction: 5, Reason: "buy",
	}}

	_ = engine.RunDay(&state, day0, domain.RegimeRiskOn, quotes, recs)
	for d := 1; d <= 30; d++ {
		day := day0.AddDate(0, 0, d)
		_ = engine.RunDay(&state, day, domain.RegimeRiskOn, quotes, nil)
	}
	if len(state.Positions) != 1 {
		t.Errorf("MaxHoldingDays=0 should not force-close; got %d positions after 30 days", len(state.Positions))
	}
}
