package sim

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
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

func TestRunWithOptimizerProducesOrders(t *testing.T) {
	optimizer := portfolio.NewOptimizer()
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            2,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          1,
		SlippageBPS:                 1,
		ReserveCashFraction:         0.1,
	}).WithOptimizer(optimizer)

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
		{Symbol: "2317.TW", Last: 160, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
		{Agent: "a", Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 80, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)
	if len(result.Orders) == 0 {
		t.Fatalf("expected orders when optimizer is enabled")
	}
	if result.EndingCash >= 1000000 {
		t.Fatalf("expected cash to be deployed")
	}
}

func TestRunWithOptimizerRespectsMaxOpenPositions(t *testing.T) {
	optimizer := portfolio.NewOptimizer()
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            1,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          1,
		SlippageBPS:                 1,
		ReserveCashFraction:         0.1,
	}).WithOptimizer(optimizer)

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
		{Symbol: "2317.TW", Last: 160, Volume: 1000000, IsTradable: true},
		{Symbol: "2382.TW", Last: 300, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
		{Agent: "a", Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 80, Reason: "test"},
		{Agent: "a", Symbol: "2382.TW", Side: domain.SideBuy, Conviction: 70, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)
	if len(result.Positions) > 1 {
		t.Fatalf("expected at most 1 position due to MaxOpenPositions, got %d", len(result.Positions))
	}
}

func TestRunDayStopLoss(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           1.0,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
		StopLossPct:                 0.10,
	})

	state := domain.NewSimulationState(1_000_000)
	state.Positions = []domain.Position{
		{Symbol: "2330.TW", Quantity: 1000, AverageCost: 100},
	}
	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 89, Volume: 1000000, IsTradable: true}, // down 11%
	}
	recs := []domain.Recommendation{}

	dayResult := engine.RunDay(&state, quotes[0].AsOf, domain.RegimeRiskOn, quotes, recs)
	if len(dayResult.Orders) != 1 {
		t.Fatalf("expected 1 sell order, got %d", len(dayResult.Orders))
	}
	if dayResult.Orders[0].Side != domain.SideSell {
		t.Fatalf("expected SELL order, got %s", dayResult.Orders[0].Side)
	}
	if len(state.Positions) != 0 {
		t.Fatalf("expected position to be closed, got %d", len(state.Positions))
	}
}

func TestRunDayTakeProfit(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           1.0,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
		TakeProfitPct:               0.20,
	})

	state := domain.NewSimulationState(1_000_000)
	state.Positions = []domain.Position{
		{Symbol: "2330.TW", Quantity: 1000, AverageCost: 100},
	}
	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 121, Volume: 1000000, IsTradable: true}, // up 21%
	}
	recs := []domain.Recommendation{}

	dayResult := engine.RunDay(&state, quotes[0].AsOf, domain.RegimeRiskOn, quotes, recs)
	if len(dayResult.Orders) != 1 || dayResult.Orders[0].Side != domain.SideSell {
		t.Fatalf("expected 1 SELL order for take-profit, got %+v", dayResult.Orders)
	}
}

func TestRunDayConvictionReversal(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           1.0,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
	})

	state := domain.NewSimulationState(1_000_000)
	state.Positions = []domain.Position{
		{Symbol: "2330.TW", Quantity: 1000, AverageCost: 100},
	}
	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 100, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideSell, Conviction: 80},
	}

	dayResult := engine.RunDay(&state, quotes[0].AsOf, domain.RegimeRiskOn, quotes, recs)
	if len(dayResult.Orders) != 1 || dayResult.Orders[0].Reason != "conviction_reversal" {
		t.Fatalf("expected conviction_reversal sell, got %+v", dayResult.Orders)
	}
}

func TestRunMultiDayTwentyDays(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.5,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
	})

	quotesByDate := make(map[string][]domain.Quote)
	recsByDate := make(map[string][]domain.Recommendation)
	dates := make([]time.Time, 20)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ {
		d := base.AddDate(0, 0, i)
		dates[i] = d
		key := d.Format("2006-01-02")
		quotesByDate[key] = []domain.Quote{
			{Symbol: "2330.TW", Last: float64(100 + i), Volume: 1000000, IsTradable: true},
		}
		recsByDate[key] = []domain.Recommendation{
			{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90},
		}
	}

	report := engine.RunMultiDay(quotesByDate, recsByDate, dates)
	if len(report.EquityCurve) != 20 {
		t.Fatalf("expected 20 equity points, got %d", len(report.EquityCurve))
	}
	if report.TotalReturn <= 0 {
		t.Logf("total return: %f", report.TotalReturn)
	}
}
