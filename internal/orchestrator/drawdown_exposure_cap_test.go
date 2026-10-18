package orchestrator

// End-to-end tests for the drawdown exposure ceiling at the order sizing layer
// (FU-20261016-32, k3 decision 5b, 2026-10-18).
//
// Acceptance criterion (audit §2.8 condition 3): a DrawdownEmergency decision
// must leave realized exposure at or below MaxExposure. The ceiling is a
// fraction of portfolio value, enforced while buy orders are sized, so the
// assertions below read the positions the engine actually produced.

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// capE2ECostTolerance absorbs the transaction-cost drag: the sizing budget
// bounds the cash spent, while realized exposure is measured at the fill price
// and the costs reduce the portfolio value used as the denominator (slippage +
// commission + impact, ~10-30 bps of the traded notional).
const capE2ECostTolerance = 1e-4

var capE2ESymbols = []string{
	"2330.TW", "2317.TW", "2454.TW", "2412.TW", "2881.TW",
	"1301.TW", "2603.TW", "2308.TW", "2882.TW", "1216.TW",
}

func capE2EAsOf() time.Time {
	return time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
}

// capE2EConstraints size a book that would otherwise deploy ~96% of the
// portfolio, so the ceiling is the only thing that can hold exposure down.
func capE2EConstraints() domain.SimulationConstraints {
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

// capE2EQuotes returns the equity quotes plus a VIX quote that drives the macro
// risk engine to MacroRiskRed (VIX >= 35 is a crisis level), which is what makes
// the production decision path emit a capped drawdown action.
func capE2EQuotes() []domain.Quote {
	quotes := []domain.Quote{{
		Symbol: "VIX", Last: 65, Open: 60, Volume: 1000, IsTradable: true, AsOf: capE2EAsOf(),
	}}
	for _, s := range capE2ESymbols {
		quotes = append(quotes, domain.Quote{
			Symbol: s, Last: 100, Open: 100, Volume: 10_000_000, IsTradable: true, AsOf: capE2EAsOf(),
		})
	}
	return quotes
}

func capE2ERecs(quotes []domain.Quote) []domain.Recommendation {
	recs := make([]domain.Recommendation, 0, len(quotes))
	for _, q := range quotes {
		if q.Symbol == "VIX" {
			continue
		}
		recs = append(recs, domain.Recommendation{
			Agent: "cap-e2e", Symbol: q.Symbol, Side: domain.SideBuy, Conviction: 90, Reason: "cap-e2e",
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

// TestDrawdownEmergencyDecisionCapsRealizedExposure is the audit's acceptance
// criterion: build a DrawdownEmergency decision from the shipped level table,
// push it through the production wire point, and require the session's realized
// exposure to stay at or below MaxExposure.
func TestDrawdownEmergencyDecisionCapsRealizedExposure(t *testing.T) {
	sys := newTestSystem(t)
	sys.Sim().engine = sim.NewEngine(capE2EConstraints())
	sys.risk.macroDrawdownEngine = risk.NewMacroAwareDrawdownEngine()

	emergency := risk.DefaultDrawdownLevels[risk.DrawdownEmergency]
	decision := &risk.MacroAwareDrawdownDecision{
		Action:      risk.DrawdownEmergency,
		Percentage:  emergency.Percentage,
		MaxExposure: emergency.MaxExposure,
	}
	sys.applyExposureCapForDecision(decision)

	quotes := capE2EQuotes()
	result := sys.Sim().engine.Run(domain.RegimeRiskOn, quotes, capE2ERecs(quotes))

	ratio := realizedExposureRatio(result)
	t.Logf("emergency ceiling %.2f: realized exposure %.6f over %d position(s)", decision.MaxExposure, ratio, len(result.Positions))
	if ratio > decision.MaxExposure+capE2ECostTolerance {
		t.Fatalf("realized exposure %.6f exceeds MaxExposure %.2f after a DrawdownEmergency decision", ratio, decision.MaxExposure)
	}
	if ratio <= 0 {
		t.Fatalf("ceiling blocked all buying (exposure %.6f); the ceiling must bound sizing, not halt the session", ratio)
	}
}

// TestDrawdownEmergencyDecisionWithoutWirePointExceedsCeiling is the negative
// control for the test above: the same session, same decision, but the wire
// point is never called. It pins the pre-wiring behavior (the reason this change
// exists) and keeps the assertion above from passing vacuously.
func TestDrawdownEmergencyDecisionWithoutWirePointExceedsCeiling(t *testing.T) {
	sys := newTestSystem(t)
	sys.Sim().engine = sim.NewEngine(capE2EConstraints())
	sys.risk.macroDrawdownEngine = risk.NewMacroAwareDrawdownEngine()

	emergency := risk.DefaultDrawdownLevels[risk.DrawdownEmergency]
	quotes := capE2EQuotes()
	result := sys.Sim().engine.Run(domain.RegimeRiskOn, quotes, capE2ERecs(quotes))

	ratio := realizedExposureRatio(result)
	t.Logf("no wire point: realized exposure %.6f (emergency MaxExposure %.2f)", ratio, emergency.MaxExposure)
	if ratio <= emergency.MaxExposure+capE2ECostTolerance {
		t.Fatalf("exposure %.6f did not exceed MaxExposure %.2f without the wire point; the ceiling test would be vacuous", ratio, emergency.MaxExposure)
	}
}

// TestRunDailySimulationRedMacroCeilingReachesOrderSizing covers the production
// entry point end to end: RunDailySimulation must evaluate the drawdown decision
// before the engine sizes orders and leave the decision's ceiling on the engine.
//
// The production recommendation pipeline is not guaranteed to emit buys for this
// fixture, so the assertion observes the ceiling the run left behind by sizing a
// probe session with the same engine. A run that skipped the wiring leaves no
// ceiling, and the probe then deploys ~96% of the portfolio.
func TestRunDailySimulationRedMacroCeilingReachesOrderSizing(t *testing.T) {
	quotes := capE2EQuotes()
	sys := newTestSystem(t)
	sys.Sim().provider = &capE2EProvider{quotes: quotes}
	sys.Sim().engine = sim.NewEngine(capE2EConstraints())
	sys.risk.macroRiskEngine = narrative.NewMacroRiskAssessmentEngine()
	sys.risk.macroDrawdownEngine = risk.NewMacroAwareDrawdownEngine()

	// The decision the production path must arrive at for this quote set.
	decision := sys.evaluateDrawdown(sys.assessMacroRisk(quotes), nil)
	if decision == nil {
		t.Fatal("expected a drawdown decision for a crisis-level VIX")
	}
	if decision.Action != risk.DrawdownSevere {
		t.Fatalf("drawdown action = %s, want severe (VIX >= 35 is a crisis level)", decision.Action.String())
	}

	if _, err := sys.RunDailySimulation(capE2EAsOf()); err != nil {
		t.Fatalf("RunDailySimulation: %v", err)
	}

	probe := sys.Sim().engine.Run(domain.RegimeRiskOn, quotes, capE2ERecs(quotes))
	ratio := realizedExposureRatio(probe)
	t.Logf("after RunDailySimulation: probe exposure %.6f (decision MaxExposure %.2f)", ratio, decision.MaxExposure)
	if ratio > decision.MaxExposure+capE2ECostTolerance {
		t.Fatalf("probe exposure %.6f exceeds the decision's MaxExposure %.2f — RunDailySimulation did not apply the ceiling before sizing", ratio, decision.MaxExposure)
	}
	if ratio <= 0.10 {
		t.Fatalf("probe exposure %.6f is at or below the emergency ceiling; the production path applied no ceiling (or the wrong one)", ratio)
	}
}

// TestApplyDrawdownExposureCapEvaluatesProductionDecision wires the assessment
// path directly: with macro and structural engines present, a crisis-level VIX
// quote must produce a capped decision and leave it on the engine.
func TestApplyDrawdownExposureCapEvaluatesProductionDecision(t *testing.T) {
	quotes := capE2EQuotes()
	sys := newTestSystem(t)
	sys.Sim().engine = sim.NewEngine(capE2EConstraints())
	sys.risk.macroRiskEngine = narrative.NewMacroRiskAssessmentEngine()
	sys.risk.macroDrawdownEngine = risk.NewMacroAwareDrawdownEngine()

	sys.applyDrawdownExposureCap(context.Background(), quotes)

	probe := sys.Sim().engine.Run(domain.RegimeRiskOn, quotes, capE2ERecs(quotes))
	ratio := realizedExposureRatio(probe)
	t.Logf("applyDrawdownExposureCap: probe exposure %.6f", ratio)
	if ratio > 0.40+capE2ECostTolerance {
		t.Fatalf("probe exposure %.6f exceeds the severe ceiling 0.40", ratio)
	}
	if ratio <= 0.10 {
		t.Fatalf("probe exposure %.6f is at or below the emergency ceiling; no ceiling was applied", ratio)
	}
}

type capE2EProvider struct{ quotes []domain.Quote }

func (p *capE2EProvider) Name() string { return "cap-e2e" }

func (p *capE2EProvider) GetQuotes(_ context.Context, _ time.Time, _ []string) ([]domain.Quote, error) {
	return p.quotes, nil
}
