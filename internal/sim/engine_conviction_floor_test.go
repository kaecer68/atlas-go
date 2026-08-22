package sim

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestWithConvictionFloorAdjustment_RaisesBuyBar verifies the CharterMode
// periodized conviction floor (C3): an adjustment raises the effective
// MinRecommendationConviction for buy execution, and clearing it restores
// the base constraint value (Phase A parity).
func TestWithConvictionFloorAdjustment_RaisesBuyBar(t *testing.T) {
	newEngine := func() *Engine {
		return NewEngine(domain.SimulationConstraints{
			StartingCash:                1_000_000,
			MaxPositionWeight:           0.25,
			MaxOpenPositions:            1,
			MinTradableVolume:           1000,
			MinRecommendationConviction: 60, // base floor
			RequireCROPass:              true,
			TransactionCostBPS:          1,
			SlippageBPS:                 1,
			ReserveCashFraction:         0.1,
		})
	}
	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1_000_000, IsTradable: true},
	}

	// Rec conviction 65: passes the base floor (60) but fails the +10
	// black-swan/RISK_OFF-adjusted floor (70).
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 65, Reason: "test"},
	}

	base := newEngine().Run(domain.RegimeRiskOn, quotes, recs)
	if len(base.Orders) == 0 {
		t.Fatal("conviction 65 ≥ base floor 60 → expected an order")
	}

	// +10 adjustment → effective floor 70 → rec 65 is gated out.
	raised := newEngine().WithConvictionFloorAdjustment(10).Run(domain.RegimeRiskOn, quotes, recs)
	if len(raised.Orders) != 0 {
		t.Errorf("conviction 65 < floor 70 → expected no orders, got %d", len(raised.Orders))
	}
	if raised.EndingCash != 1_000_000 {
		t.Errorf("gated buy must leave starting cash untouched, got %.2f", raised.EndingCash)
	}

	// A rec above the adjusted floor still executes.
	strongRecs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80, Reason: "test"},
	}
	strong := newEngine().WithConvictionFloorAdjustment(10).Run(domain.RegimeRiskOn, quotes, strongRecs)
	if len(strong.Orders) == 0 {
		t.Error("conviction 80 ≥ floor 70 → expected an order")
	}

	// Clearing the adjustment (delta ≤ 0) restores the base floor.
	cleared := newEngine().WithConvictionFloorAdjustment(10).WithConvictionFloorAdjustment(0).Run(domain.RegimeRiskOn, quotes, recs)
	if len(cleared.Orders) == 0 {
		t.Error("cleared adjustment must restore base floor 60 → expected an order")
	}
}

// TestWithConvictionFloorAdjustment_EffectiveFloorUnit pins the effective
// floor arithmetic (base + delta, cleared on non-positive delta).
func TestWithConvictionFloorAdjustment_EffectiveFloorUnit(t *testing.T) {
	e := NewEngine(domain.SimulationConstraints{MinRecommendationConviction: 60})
	if got := e.effectiveConvictionFloor(); got != 60 {
		t.Errorf("no adjustment → floor %d, want 60", got)
	}
	e.WithConvictionFloorAdjustment(20)
	if got := e.effectiveConvictionFloor(); got != 80 {
		t.Errorf("base 60 + delta 20 → floor %d, want 80", got)
	}
	e.WithConvictionFloorAdjustment(0)
	if got := e.effectiveConvictionFloor(); got != 60 {
		t.Errorf("cleared adjustment → floor %d, want 60", got)
	}
}
