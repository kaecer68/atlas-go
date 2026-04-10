package sim

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRunBuildsPositions(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            2,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		RequireCROPass:              true,
		TransactionCostBPS:          1,
		SlippageBPS:                 1,
		ReserveCashFraction:         0.1,
	})

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
		{Symbol: "2317.TW", Last: 160, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
		{Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 80, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)
	if len(result.Orders) == 0 {
		t.Fatalf("expected orders to be created")
	}
	if result.EndingCash >= 1000000 {
		t.Fatalf("expected cash to be deployed")
	}
}

func TestRunDeterministicTieBreakForEqualConviction(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            1,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		RequireCROPass:              true,
		TransactionCostBPS:          1,
		SlippageBPS:                 1,
		ReserveCashFraction:         0.1,
	})

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
		{Symbol: "2317.TW", Last: 160, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Agent: "b", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80, Reason: "tie"},
		{Agent: "a", Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 80, Reason: "tie"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)
	if len(result.Orders) != 1 {
		t.Fatalf("expected one order, got %d", len(result.Orders))
	}
	if result.Orders[0].Symbol != "2317.TW" {
		t.Fatalf("expected deterministic tie-break to pick lexicographically smaller symbol, got %s", result.Orders[0].Symbol)
	}
}
