package sim

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRun_TPlusTwoLock(t *testing.T) {
	day0 := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	constraints := domain.SimulationConstraints{
		StartingCash:                200000,
		MaxPositionWeight:           0.5,
		MaxOpenPositions:            5,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 1,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
		MaxHoldingDays:              0,
		TakeProfitPct:               0.5,
	}
	engine := NewEngine(constraints)
	state := domain.NewSimulationState(constraints.StartingCash)

	quotes := []domain.Quote{{
		Symbol: "2330", Last: 100.0, Open: 100.0, Volume: 1_000_000, IsTradable: true,
	}}

	buyRec := []domain.Recommendation{{
		Agent: "test", Symbol: "2330", Side: domain.SideBuy, Conviction: 5, Reason: "buy",
	}}
	sellRec := []domain.Recommendation{{
		Agent: "test", Symbol: "2330", Side: domain.SideSell, Conviction: 5, Reason: "take_profit",
	}}

	_ = engine.RunDay(&state, day0, domain.RegimeRiskOn, quotes, buyRec)
	if len(state.Positions) != 1 {
		t.Fatalf("day0 buy: want 1 position, got %d", len(state.Positions))
	}

	_ = engine.RunDay(&state, day0, domain.RegimeRiskOn, quotes, sellRec)
	if len(state.Positions) != 0 {
		t.Fatalf("day0 sell: want 0 positions, got %d", len(state.Positions))
	}

	if got := state.AvailableCash(day0); got != 100000 {
		t.Errorf("day0 after sell: want available cash 100000 (200000 - 100000 locked), got %v", got)
	}

	expensiveQuotes := []domain.Quote{{
		Symbol: "2330", Last: 1000.0, Open: 1000.0, Volume: 1_000_000, IsTradable: true,
	}}
	day1 := day0.AddDate(0, 0, 1)
	_ = engine.RunDay(&state, day1, domain.RegimeRiskOn, expensiveQuotes, buyRec)
	if len(state.Positions) != 0 {
		t.Errorf("day1: should NOT buy at 1000 (maxPerPosition=50000 < 1 lot cost 100000), got %d positions", len(state.Positions))
	}

	day2 := day0.AddDate(0, 0, 2)
	_ = engine.RunDay(&state, day2, domain.RegimeRiskOn, expensiveQuotes, buyRec)
	if len(state.Positions) != 1 {
		t.Errorf("day2: should buy at 1000 (locked cash released, available 200000), got %d positions", len(state.Positions))
	}
}

func TestSimulationState_AvailableCash_UnitBoundary(t *testing.T) {
	day0 := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	state := domain.NewSimulationState(200000)
	state.LockedCash = []domain.LockedCashEntry{
		{UnlockDay: day0.AddDate(0, 0, 2), Amount: 100000},
	}

	cases := []struct {
		name string
		day  time.Time
		want float64
	}{
		{"day0 locked", day0, 100000},
		{"day1 still locked", day0.AddDate(0, 0, 1), 100000},
		{"day2 released", day0.AddDate(0, 0, 2), 200000},
		{"day3 still released", day0.AddDate(0, 0, 3), 200000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := state.AvailableCash(tc.day); got != tc.want {
				t.Errorf("day %v: want %v, got %v", tc.day, tc.want, got)
			}
		})
	}
}
