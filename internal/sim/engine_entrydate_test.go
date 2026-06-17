package sim

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TestRunDay_RecordsEntryDateOnBuy verifies that a Position created by a BUY
// order during RunDay carries the current trading day as EntryDate.
// Foundation for P2-T4 (MaxHoldingDays) and P2-T5 (T+2 settlement).
func TestRunDay_RecordsEntryDateOnBuy(t *testing.T) {
	day := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	constraints := domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           0.5,
		MaxOpenPositions:            5,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 1,
		TransactionCostBPS:          10,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
	}
	engine := NewEngine(constraints)
	state := domain.NewSimulationState(constraints.StartingCash)

	recs := []domain.Recommendation{{
		Agent:      "test",
		Symbol:     "2330",
		Side:       domain.SideBuy,
		Conviction: 5,
		Reason:     "test buy",
	}}
	quotes := []domain.Quote{{
		Symbol:    "2330",
		Last:      100.0,
		Open:      100.0,
		Volume:    100_000,
		IsTradable: true,
	}}

	_ = engine.RunDay(&state, day, domain.RegimeRiskOn, quotes, recs)

	var bought *domain.Position
	for i := range state.Positions {
		if state.Positions[i].Symbol == "2330" {
			bought = &state.Positions[i]
			break
		}
	}
	if bought == nil {
		t.Fatalf("expected position 2330 in state after buy, got %d positions", len(state.Positions))
	}
	if !bought.EntryDate.Equal(day) {
		t.Errorf("EntryDate mismatch: want %v, got %v", day, bought.EntryDate)
	}
}
