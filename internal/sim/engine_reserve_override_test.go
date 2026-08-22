package sim

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestWithReserveCashFraction_OverrideReducesDeployment verifies the
// CharterMode reserve-cash override scales down deployable cash: a higher
// reserve fraction must leave more cash unspent for the same recommendation.
func TestWithReserveCashFraction_OverrideReducesDeployment(t *testing.T) {
	newEngine := func() *Engine {
		return NewEngine(domain.SimulationConstraints{
			StartingCash:                1_000_000,
			MaxPositionWeight:           0.25,
			MaxOpenPositions:            1,
			MinTradableVolume:           1000,
			MinRecommendationConviction: 0,
			RequireCROPass:              true,
			TransactionCostBPS:          1,
			SlippageBPS:                 1,
			ReserveCashFraction:         0.1,
		})
	}
	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1_000_000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	// Baseline: base constraint reserve 0.1 → 90% deployable.
	base := newEngine().Run(domain.RegimeRiskOn, quotes, recs)
	if len(base.Orders) == 0 {
		t.Fatal("expected baseline orders")
	}

	// Override: 0.5 reserve → only 50% deployable → smaller order, more cash left.
	overridden := newEngine().WithReserveCashFraction(0.5).Run(domain.RegimeRiskOn, quotes, recs)
	if len(overridden.Orders) == 0 {
		t.Fatal("expected orders with override")
	}
	if overridden.EndingCash <= base.EndingCash {
		t.Errorf("override 0.5 should leave more cash than baseline 0.1: override cash=%.2f, base cash=%.2f",
			overridden.EndingCash, base.EndingCash)
	}

	// Exact sizing check: maxPerPosition = 1_000_000 * (1-0.5) * 0.25 = 125_000
	// → qty = floor(125_000/800/100)*100 = 100 shares → cost 80_000 (+fees).
	wantQty := 100
	if overridden.Orders[0].Quantity != wantQty {
		t.Errorf("override order quantity = %d, want %d", overridden.Orders[0].Quantity, wantQty)
	}

	// Clearing the override (negative) restores base constraint behavior.
	cleared := newEngine().WithReserveCashFraction(0.5).WithReserveCashFraction(-1).Run(domain.RegimeRiskOn, quotes, recs)
	if cleared.EndingCash != base.EndingCash {
		t.Errorf("cleared override should match baseline: cleared cash=%.2f, base cash=%.2f",
			cleared.EndingCash, base.EndingCash)
	}
	if len(cleared.Orders) != len(base.Orders) || cleared.Orders[0].Quantity != base.Orders[0].Quantity {
		t.Errorf("cleared override orders mismatch: got %+v, want %+v", cleared.Orders, base.Orders)
	}
}
