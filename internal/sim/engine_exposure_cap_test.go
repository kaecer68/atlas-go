package sim

import (
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// Tests for the drawdown exposure ceiling at the buy sizing layer
// (FU-20261016-32, k3 decision 5b).
//
// The ceiling is a fraction of portfolio value: total position exposure must
// stay at or below cap × portfolio value. exposureCostTolerance absorbs the
// transaction-cost drag: the budget bounds the cash spent, while exposure is
// measured at the fill price, and costs reduce the portfolio value used as the
// denominator (slippage + commission + impact, ~10-30 bps of the traded
// notional).
const exposureCostTolerance = 1e-4

func capTestConstraints() domain.SimulationConstraints {
	return domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            10,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          1,
		SlippageBPS:                 1,
		ReserveCashFraction:         0,
	}
}

func capTestQuotes() []domain.Quote {
	symbols := []string{
		"2330.TW", "2317.TW", "2454.TW", "2412.TW", "2881.TW",
		"1301.TW", "2603.TW", "2308.TW", "2882.TW", "1216.TW",
	}
	quotes := make([]domain.Quote, 0, len(symbols))
	for _, s := range symbols {
		quotes = append(quotes, domain.Quote{
			Symbol: s, Last: 100, Open: 100, Volume: 10_000_000, IsTradable: true,
			AsOf: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC),
		})
	}
	return quotes
}

func capTestRecs(quotes []domain.Quote) []domain.Recommendation {
	recs := make([]domain.Recommendation, 0, len(quotes))
	for _, q := range quotes {
		recs = append(recs, domain.Recommendation{
			Agent: "cap-test", Symbol: q.Symbol, Side: domain.SideBuy, Conviction: 90, Reason: "cap-test",
		})
	}
	return recs
}

func realizedExposureRatio(result domain.SimulationResult) float64 {
	if result.PortfolioValue <= 0 {
		return 0
	}
	var exposure float64
	for _, p := range result.Positions {
		exposure += p.MarketValue
	}
	return exposure / result.PortfolioValue
}

func TestExposureBudget_NoCeilingIsUnbounded(t *testing.T) {
	state := domain.NewSimulationState(1_000_000)
	state.Positions = []domain.Position{{Symbol: "2330.TW", MarketValue: 100_000}}

	engine := NewEngine(domain.SimulationConstraints{})
	if got := engine.exposureBudget(&state); got != math.MaxFloat64 {
		t.Errorf("default engine budget = %v, want MaxFloat64 (no ceiling before wiring)", got)
	}

	engine.WithExposureCap(1)
	if got := engine.exposureBudget(&state); got != math.MaxFloat64 {
		t.Errorf("budget with cap 1.0 = %v, want MaxFloat64 (100%% exposure is no ceiling)", got)
	}

	engine.WithExposureCap(1.5)
	if got := engine.exposureBudget(&state); got != math.MaxFloat64 {
		t.Errorf("budget with cap 1.5 = %v, want MaxFloat64 (clamped to 1.0)", got)
	}
}

func TestExposureBudget_IsCeilingMinusCurrentExposure(t *testing.T) {
	state := domain.NewSimulationState(1_000_000)
	state.Positions = []domain.Position{{Symbol: "2330.TW", MarketValue: 100_000}}

	engine := NewEngine(domain.SimulationConstraints{}).WithExposureCap(0.25)
	// 0.25 × 1_100_000 - 100_000 = 175_000
	if got := engine.exposureBudget(&state); got != 175_000 {
		t.Errorf("budget = %v, want 175000", got)
	}

	engine.WithExposureCap(0.05) // ceiling already exceeded
	if got := engine.exposureBudget(&state); got != 0 {
		t.Errorf("budget above ceiling = %v, want 0", got)
	}
}

func TestRunDay_ExposureCapBoundsRealizedExposure(t *testing.T) {
	const cap = 0.10
	quotes := capTestQuotes()
	engine := NewEngine(capTestConstraints()).WithExposureCap(cap)

	result := engine.Run(domain.RegimeRiskOn, quotes, capTestRecs(quotes))

	ratio := realizedExposureRatio(result)
	t.Logf("capped session: exposure %.6f, portfolio value %.2f, orders %d", ratio, result.PortfolioValue, len(result.Orders))
	if ratio > cap+exposureCostTolerance {
		t.Fatalf("realized exposure %.6f exceeds ceiling %.2f", ratio, cap)
	}
	if ratio <= 0 {
		t.Fatalf("ceiling blocked all buying (exposure %.6f); the ceiling must only bound sizing", ratio)
	}
	if len(result.Orders) == 0 {
		t.Fatal("expected at least one buy order under the ceiling")
	}
}

func TestRunDay_WithoutCeilingDeploysPastEmergencyCeiling(t *testing.T) {
	// Pre-wiring behavior, and the reason the ceiling has to be wired: without
	// it the same session deploys far past the emergency ceiling of 0.10.
	quotes := capTestQuotes()
	engine := NewEngine(capTestConstraints())

	result := engine.Run(domain.RegimeRiskOn, quotes, capTestRecs(quotes))

	ratio := realizedExposureRatio(result)
	t.Logf("uncapped session: exposure %.6f, portfolio value %.2f, orders %d", ratio, result.PortfolioValue, len(result.Orders))
	if ratio <= 0.10+exposureCostTolerance {
		t.Fatalf("uncapped exposure %.6f did not exceed the emergency ceiling 0.10; the cap test would be vacuous", ratio)
	}
}

func TestRunDay_ExposureCapBlocksNewBuysAboveCeiling(t *testing.T) {
	// Portfolio value 2_000_000 (cash 1_000_000 + 10_000 shares at 100), so the
	// existing book is already at 50% exposure; an emergency ceiling of 0.10
	// leaves no room for new buys.
	state := domain.NewSimulationState(1_000_000)
	state.Positions = []domain.Position{{Symbol: "2330.TW", Quantity: 10_000, AverageCost: 100}}

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 100, Open: 100, Volume: 10_000_000, IsTradable: true,
			AsOf: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)},
		{Symbol: "2317.TW", Last: 100, Open: 100, Volume: 10_000_000, IsTradable: true,
			AsOf: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)},
	}
	recs := []domain.Recommendation{
		{Agent: "cap-test", Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 90, Reason: "cap-test"},
	}

	engine := NewEngine(capTestConstraints()).WithExposureCap(0.10)
	dayResult := engine.RunDay(&state, quotes[0].AsOf, domain.RegimeRiskOn, quotes, recs)
	if len(dayResult.Orders) != 0 {
		t.Fatalf("expected no orders while exposure is above the ceiling, got %d", len(dayResult.Orders))
	}

	// Same session without the ceiling buys the recommended symbol.
	uncapped := NewEngine(capTestConstraints())
	uncappedResult := uncapped.RunDay(&state, quotes[0].AsOf, domain.RegimeRiskOn, quotes, recs)
	if len(uncappedResult.Orders) == 0 {
		t.Fatal("expected the uncapped engine to place the buy; the ceiling is what blocked it")
	}
}

func TestRunDay_ExposureCapBoundsRealizedExposureWithOptimizer(t *testing.T) {
	const cap = 0.10
	quotes := capTestQuotes()
	engine := NewEngine(capTestConstraints()).WithOptimizer(portfolio.NewOptimizer()).WithExposureCap(cap)

	result := engine.Run(domain.RegimeRiskOn, quotes, capTestRecs(quotes))

	ratio := realizedExposureRatio(result)
	if ratio > cap+exposureCostTolerance {
		t.Fatalf("optimizer path realized exposure %.6f exceeds ceiling %.2f", ratio, cap)
	}
	if len(result.Orders) == 0 {
		t.Fatal("expected at least one buy order under the ceiling on the optimizer path")
	}
}
