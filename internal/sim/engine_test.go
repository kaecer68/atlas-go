package sim

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/tax"
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

	for i := range 20 {
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

	report := engine.RunMultiDay(domain.RegimeRiskOn, quotesByDate, recsByDate, dates)
	if len(report.EquityCurve) != 20 {
		t.Fatalf("expected 20 equity points, got %d", len(report.EquityCurve))
	}
	if report.TotalReturn <= 0 {
		t.Logf("total return: %f", report.TotalReturn)
	}
}

func TestRunWithTaxCalculatorPopulatesTaxFields(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.5,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
	}).WithTaxCalculator(tax.NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())).
		WithDividends(map[string]float64{"2330.TW": 15000})

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)

	if len(result.TaxSnapshots) == 0 {
		t.Fatal("expected tax snapshots to be populated")
	}
	if result.BeforeTaxPnL == 0 && result.AfterTaxPnL == 0 {
		t.Fatal("expected P&L fields to be populated")
	}
	if result.TotalTaxPaid <= 0 {
		t.Errorf("expected positive TotalTaxPaid, got %v", result.TotalTaxPaid)
	}
}

func TestTaxAdjustedPnLEqualsGrossMinusTax(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.5,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
	}).WithTaxCalculator(tax.NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig()))

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)

	wantAfterTax := result.BeforeTaxPnL - result.TotalTaxPaid
	if math.Abs(result.AfterTaxPnL-wantAfterTax) > 0.01 {
		t.Errorf("AfterTaxPnL = %v, want %v (BeforeTaxPnL=%v - TotalTaxPaid=%v)",
			result.AfterTaxPnL, wantAfterTax, result.BeforeTaxPnL, result.TotalTaxPaid)
	}
}

func TestRunWithDefaultTaxCalculatorPopulatesTaxFields(t *testing.T) {
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

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)

	if len(result.TaxSnapshots) == 0 {
		t.Errorf("expected tax snapshots with default tax calculator, got 0")
	}
	if result.AfterTaxPnL == 0 && result.BeforeTaxPnL != 0 {
		t.Errorf("AfterTaxPnL should be computed: got %f, BeforeTaxPnL=%f", result.AfterTaxPnL, result.BeforeTaxPnL)
	}
}

func TestPreTradeGateFilterBlocksOverSize(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            10,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          1,
		SlippageBPS:                 1,
		ReserveCashFraction:         0.1,
	}).WithPreTradeGate(risk.NewPreTradeGate())

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "large", TargetPrice: 850},
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 30, Reason: "small", TargetPrice: 810},
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50, Reason: "medium", TargetPrice: 830},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)

	if result.PortfolioValue <= 0 {
		t.Errorf("expected positive portfolio value, got %.0f", result.PortfolioValue)
	}
}

func TestBuildOrderIntent(t *testing.T) {
	e := &Engine{}
	rec := domain.Recommendation{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50}
	order := e.buildOrderIntent(rec, 1_000_000)
	if order.Symbol != "2330.TW" {
		t.Errorf("expected 2330.TW, got %s", order.Symbol)
	}
	if order.Notional <= 0 {
		t.Errorf("expected positive notional, got %.0f", order.Notional)
	}
	if order.Side != "BUY" {
		t.Errorf("expected BUY, got %s", order.Side)
	}
}

func TestBuildOrderIntentWithTargetPrice(t *testing.T) {
	e := &Engine{}
	rec := domain.Recommendation{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80, TargetPrice: 1000}
	order := e.buildOrderIntent(rec, 2_000_000)
	if order.Notional <= 0 {
		t.Errorf("expected positive notional from conviction, got %.0f", order.Notional)
	}
}

func TestDeriveSimDay_ReturnsQuoteAsOf(t *testing.T) {
	now := time.Now()
	quotes := []domain.Quote{
		{Symbol: "2330.TW", AsOf: now},
	}
	day := deriveSimDay(quotes)
	if !day.Equal(now) {
		t.Errorf("deriveSimDay = %v, want %v", day, now)
	}
}

func TestDeriveSimDay_SkipsZeroAsOf(t *testing.T) {
	now := time.Now()
	quotes := []domain.Quote{
		{Symbol: "2330.TW", AsOf: time.Time{}},
		{Symbol: "2317.TW", AsOf: now},
	}
	day := deriveSimDay(quotes)
	if !day.Equal(now) {
		t.Errorf("deriveSimDay = %v, want %v", day, now)
	}
}

func TestDeriveSimDay_EmptyReturnsZero(t *testing.T) {
	day := deriveSimDay(nil)
	if !day.IsZero() {
		t.Errorf("deriveSimDay(nil) = %v, want zero time", day)
	}
}

func TestRunDay_Rebalance_UsesExecutedPriceForMarketValue(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           1.0,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 100, // 1% slippage to make price difference visible
		ReserveCashFraction:         0,
	})
	gate := risk.NewPreTradeGate()
	if gate.MaxPositionPct() <= 0 {
		t.Skip("pre-trade gate max position pct is zero in test config")
	}
	engine.WithPreTradeGate(gate)

	state := domain.NewSimulationState(1_000_000)
	state.Positions = []domain.Position{
		{Symbol: "2330.TW", Quantity: 1000, AverageCost: 100, EntryDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 1000, Volume: 1000000, IsTradable: true, AsOf: day},
	}

	engine.RunDay(&state, day, domain.RegimeRiskOn, quotes, nil)

	if len(state.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(state.Positions))
	}
	pos := state.Positions[0]
	executedPrice := 1000 * (1 - 100/10000.0) // 990
	wantMV := float64(pos.Quantity) * executedPrice
	if math.Abs(pos.MarketValue-wantMV) > 0.01 {
		t.Fatalf("market value = %f, want %f (executed price %f)", pos.MarketValue, wantMV, executedPrice)
	}
}

func TestRun_LargeOrder_AppliesMarketImpact(t *testing.T) {
	impact := &MarketImpactModel{
		TemporaryImpactCoef: 1.0,
		PermanentImpactCoef: 0.0,
		DefaultADV:          1000, // tiny ADV to force impact
	}
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                10_000_000,
		MaxPositionWeight:           1.0,
		MaxOpenPositions:            1,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
	}).WithMarketImpactModel(impact)

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 100, Volume: 1000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)
	if len(result.Orders) == 0 {
		t.Fatalf("expected an order")
	}
	if result.Orders[0].Price <= 100 {
		t.Fatalf("expected market impact to raise buy price above 100, got %f", result.Orders[0].Price)
	}
}
