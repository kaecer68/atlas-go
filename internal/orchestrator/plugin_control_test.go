package orchestrator

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestControlLayerAppliesCROAndCIO(t *testing.T) {
	registry := SeedRegistry()
	plugins := NewPluginRegistry()

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 40, Side: domain.SideBuy, Reason: "weak"},
		{Agent: "b", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 80, Side: domain.SideBuy, Reason: "strong"},
		{Agent: "c", Skill: "ai_supply_chain_desk", Symbol: "2382.TW", Conviction: 70, Side: domain.SideBuy, Reason: "good"},
	}

	out, _ := applyControlLayerWithOutcomes(registry, plugins, recs, DefaultExecutionPolicy(), domain.RegimeRiskOn, nil, "", nil)
	if len(out) != 2 {
		t.Fatalf("expected 2 aggregated control outputs, got %d", len(out))
	}
	// CIOPortfolioExecutor preserves the original agent ID of the highest-conviction recommendation.
	agentSet := map[string]bool{}
	for _, rec := range out {
		agentSet[rec.Agent] = true
		if rec.Conviction < 50 {
			t.Fatalf("expected CRO to filter weak recommendations")
		}
	}
	if !agentSet["b"] || !agentSet["c"] {
		t.Fatalf("expected original agent IDs to be preserved (b for 2317.TW, c for 2382.TW), got %v", agentSet)
	}
}

func TestControlLayerCanBypassCROWhenPolicyAllows(t *testing.T) {
	registry := SeedRegistry()
	plugins := NewPluginRegistry()

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 40, Side: domain.SideBuy, Reason: "weak"},
	}

	out, _ := applyControlLayerWithOutcomes(registry, plugins, recs, domain.ExecutionPolicy{
		ConvictionFloor: 50,
		RequireCROPass:  false,
	}, domain.RegimeRiskOn, nil, "", nil)
	if len(out) != 1 {
		t.Fatalf("expected raw recommendation to bypass control when CRO pass is disabled")
	}
	if out[0].Agent != "a" {
		t.Fatalf("expected raw recommendation ownership preserved, got %s", out[0].Agent)
	}
}

func TestControlLayerProducesGuardOutcomes(t *testing.T) {
	registry := SeedRegistry()
	plugins := NewPluginRegistry()

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 40, Side: domain.SideBuy, Reason: "weak"},
		{Agent: "b", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 80, Side: domain.SideBuy, Reason: "strong"},
	}

	_, outcomes := applyControlLayerWithOutcomes(registry, plugins, recs, DefaultExecutionPolicy(), domain.RegimeRiskOn, nil, "", nil)
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 guard outcomes for CRO and CIO, got %d", len(outcomes))
	}
	if outcomes[0].GuardSkill != "cro_risk" {
		t.Fatalf("expected first guard to be CRO, got %s", outcomes[0].GuardSkill)
	}
	if outcomes[0].Severity != domain.GuardSeverityHard {
		t.Fatalf("expected CRO to be hard guard")
	}
}

func TestControlLayerHardGuardCanBlockAllRecommendations(t *testing.T) {
	registry := SeedRegistry()
	plugins := NewPluginRegistry()

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 40, Side: domain.SideBuy, Reason: "weak"},
		{Agent: "b", Skill: "growth_momentum", Symbol: "2382.TW", Conviction: 45, Side: domain.SideBuy, Reason: "weak-2"},
	}

	final, outcomes := applyControlLayerWithOutcomes(registry, plugins, recs, domain.ExecutionPolicy{
		ConvictionFloor: 50,
		RequireCROPass:  true,
	}, domain.RegimeRiskOn, nil, "", nil)
	if len(final) != 0 {
		t.Fatalf("expected hard guard to block all recommendations")
	}
	if len(outcomes) == 0 {
		t.Fatalf("expected guard outcomes")
	}
	if outcomes[0].Passed {
		t.Fatalf("expected hard guard outcome to fail when all recs are blocked")
	}
}

func TestCRORiskExecutorDynamicConcentrationThreshold(t *testing.T) {
	executor := CRORiskExecutor{}
	agent := domain.AgentSpec{ID: "cro-01", Skill: "cro_risk"}

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "semiconductor", Symbol: "2330.TW", Conviction: 80, Side: domain.SideBuy, Reason: "r1"},
		{Agent: "b", Skill: "semiconductor", Symbol: "2317.TW", Conviction: 75, Side: domain.SideBuy, Reason: "r2"},
		{Agent: "c", Skill: "semiconductor", Symbol: "2454.TW", Conviction: 70, Side: domain.SideBuy, Reason: "r3"},
		{Agent: "d", Skill: "semiconductor", Symbol: "2303.TW", Conviction: 65, Side: domain.SideBuy, Reason: "r4"},
		{Agent: "e", Skill: "financials", Symbol: "2884.TW", Conviction: 60, Side: domain.SideBuy, Reason: "r5"},
		{Agent: "f", Skill: "financials", Symbol: "2891.TW", Conviction: 55, Side: domain.SideBuy, Reason: "r6"},
		{Agent: "g", Skill: "shipping", Symbol: "2603.TW", Conviction: 50, Side: domain.SideBuy, Reason: "r7"},
		{Agent: "h", Skill: "consumer", Symbol: "2912.TW", Conviction: 50, Side: domain.SideBuy, Reason: "r8"},
		{Agent: "i", Skill: "consumer", Symbol: "1229.TW", Conviction: 50, Side: domain.SideBuy, Reason: "r9"},
		{Agent: "j", Skill: "consumer", Symbol: "1707.TW", Conviction: 50, Side: domain.SideBuy, Reason: "r10"},
		{Agent: "k", Skill: "consumer", Symbol: "2207.TW", Conviction: 50, Side: domain.SideBuy, Reason: "r11"},
	}

	out := executor.Apply(agent, recs, DefaultExecutionPolicy(), domain.RegimeRiskOn)
	if len(out) == 0 {
		t.Fatal("expected some recommendations to pass")
	}

	for _, rec := range out {
		if rec.Conviction < 50 {
			t.Fatalf("expected all convictions to be >= 50 after CRO filtering, got %d for %s", rec.Conviction, rec.Symbol)
		}
	}
}

func TestCIOPortfolioExecutorDeterministicTieBreak(t *testing.T) {
	executor := CIOPortfolioExecutor{}
	agent := domain.AgentSpec{ID: "cio-01", Skill: "cio_portfolio"}

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2330.TW", Conviction: 60, Side: domain.SideBuy, Reason: "r1"},
		{Agent: "b", Skill: "ai_supply_chain_desk", Symbol: "2317.TW", Conviction: 60, Side: domain.SideBuy, Reason: "r2"},
	}

	out := executor.Apply(agent, recs, DefaultExecutionPolicy(), domain.RegimeRiskOn)
	if len(out) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(out))
	}
	if out[0].Symbol != "2317.TW" || out[1].Symbol != "2330.TW" {
		t.Fatalf("expected deterministic symbol ordering for tie conviction, got %s then %s", out[0].Symbol, out[1].Symbol)
	}
}

// TestCIOPortfolioExecutorPreservesSide verifies the B02 fix: the aggregator
// must initialize bestSide from the first (highest-conviction) recommendation.
// Prior to the fix, a single-rec aggregation left Side as the zero value "",
// which made executeOptimizerBuys skip the order (order.Side != SideBuy) and
// produced a perpetual empty daily sim (perf-report-zero audit #B02).
func TestCIOPortfolioExecutorPreservesSide(t *testing.T) {
	executor := CIOPortfolioExecutor{}
	agent := domain.AgentSpec{ID: "cio-01", Skill: "cio_portfolio"}

	// Single SELL rec: highest conviction is the first rec, so bestSide must
	// come from initialization, not the strict `>` update.
	recs := []domain.Recommendation{
		{Agent: "etf-rotation-01", Skill: "etf_rotation_desk", Symbol: "00713.TW", Conviction: 61, Side: domain.SideSell, Reason: "rotation out"},
	}
	out := executor.Apply(agent, recs, DefaultExecutionPolicy(), domain.RegimeRiskOn)
	if len(out) != 1 {
		t.Fatalf("expected 1 aggregated rec, got %d", len(out))
	}
	if out[0].Side != domain.SideSell {
		t.Fatalf("expected Side preserved as SELL, got %q", out[0].Side)
	}
}

func TestSuperinvestorExecutorApply(t *testing.T) {
	executor := SuperinvestorExecutor{}
	agent := domain.AgentSpec{ID: "si-01", Skill: "superinvestor_quality", Layer: domain.LayerSuperinvestor}

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2330.TW", Conviction: 80, Side: domain.SideBuy, Reason: "strong"},
		{Agent: "b", Skill: "semiconductor", Symbol: "2317.TW", Conviction: 60, Side: domain.SideBuy, Reason: "medium"},
		{Agent: "c", Skill: "value_yield", Symbol: "2884.TW", Conviction: 40, Side: domain.SideBuy, Reason: "weak"},
	}

	out := executor.Apply(agent, recs, DefaultExecutionPolicy(), domain.RegimeRiskOn)
	if len(out) != 2 {
		t.Fatalf("expected 2 recommendations above SuperinvestorMinConviction(50), got %d", len(out))
	}
	if out[0].Symbol != "2330.TW" {
		t.Fatalf("expected 2330.TW to survive, got %s", out[0].Symbol)
	}
	if out[1].Symbol != "2317.TW" {
		t.Fatalf("expected 2317.TW to survive, got %s", out[1].Symbol)
	}
	if !strings.Contains(out[0].Reason, "[Superinvestor:") {
		t.Fatalf("expected reason to be tagged with [Superinvestor:...], got %q", out[0].Reason)
	}
}

// TestSuperinvestorExecutorApply_NonSuperinvestorUsesFloor verifies the B02
// fix: the superinvestor quality gate must only apply the higher
// SuperinvestorMinConviction bar to recommendations that originated from a
// superinvestor-layer agent. Sector/ETF recs (Agent != super-*), aggregated by
// CIO, must only clear the baseline ConvictionFloor — otherwise a conv 40-60
// ETF rec is starved and the daily sim is perpetually empty (perf-report-zero
// audit #B02).
func TestSuperinvestorExecutorApply_NonSuperinvestorUsesFloor(t *testing.T) {
	executor := SuperinvestorExecutor{}
	agent := domain.AgentSpec{ID: "super-dru-01", Skill: "druckenmiller_macro", Layer: domain.LayerSuperinvestor}

	// Thresholds vary by environment (test default 50, production config 65), so
	// derive values from the live config to keep the test environment-agnostic.
	params := config.GetParametersConfig().Orchestrator
	floor := 35
	siMin := params.SuperinvestorMinConviction.Value

	recs := []domain.Recommendation{
		// Sector agent rec aggregated by CIO: Agent preserved as etf-rotation-01,
		// conv just above floor but below SuperinvestorMinConviction. Must survive
		// (floor-gated), even though it would fail the old max(floor, siMin) gate.
		{Agent: "etf-rotation-01", Skill: "cio_portfolio", Layer: domain.LayerControl, Symbol: "00713.TW", Conviction: max(floor, siMin-1), Side: domain.SideBuy, Reason: "etf rotation"},
		// True superinvestor self rec at same conviction: must be filtered (siMin gate).
		{Agent: "super-dru-01", Skill: "druckenmiller_macro", Layer: domain.LayerSuperinvestor, Symbol: "2330.TW", Conviction: max(floor, siMin-1), Side: domain.SideBuy, Reason: "macro"},
	}

	out := executor.Apply(agent, recs, domain.ExecutionPolicy{ConvictionFloor: floor, RequireCROPass: true}, domain.RegimeRiskOn)
	if len(out) != 1 {
		t.Fatalf("expected only the sector rec (conv %d >= floor %d) to survive, got %d: %+v", max(floor, siMin-1), floor, len(out), out)
	}
	if out[0].Symbol != "00713.TW" {
		t.Fatalf("expected 00713.TW (sector, floor-gated) to survive, got %s", out[0].Symbol)
	}
}

func TestSuperinvestorExecutorIntegrationInControlLayer(t *testing.T) {
	registry := domain.AgentRegistry{
		Version: 1,
		Agents: []domain.AgentSpec{
			{ID: "si-01", Name: "Superinvestor", Layer: domain.LayerSuperinvestor, Skill: "superinvestor_quality", Enabled: true},
			{ID: "cro-01", Name: "CRO", Layer: domain.LayerControl, Skill: "cro_risk", Enabled: true},
		},
	}
	plugins := NewPluginRegistry()

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2330.TW", Conviction: 80, Side: domain.SideBuy, Reason: "strong"},
		{Agent: "b", Skill: "semiconductor", Symbol: "2317.TW", Conviction: 60, Side: domain.SideBuy, Reason: "medium"},
	}

	_, outcomes := applyControlLayerWithOutcomes(registry, plugins, recs, DefaultExecutionPolicy(), domain.RegimeRiskOn, nil, "", nil)

	found := false
	for _, o := range outcomes {
		if o.GuardSkill == "superinvestor_quality" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected guard outcome for superinvestor agent, got %d outcomes", len(outcomes))
	}
}

func TestSeverityForSuperinvestorIsSoft(t *testing.T) {
	agent := domain.AgentSpec{ID: "si-01", Skill: "superinvestor_quality", Layer: domain.LayerSuperinvestor}
	severity := severityForControlAgent(agent)
	if severity != domain.GuardSeveritySoft {
		t.Fatalf("expected soft severity for superinvestor, got %s", severity)
	}
}

func TestSuperinvestorRecommendDruckenmiller(t *testing.T) {
	exec := SuperinvestorExecutor{}
	agent := domain.AgentSpec{ID: "super-dru-01", Skill: "druckenmiller_macro", Layer: domain.LayerSuperinvestor}
	quote := domain.Quote{Symbol: "2330.TW", Last: 600, Open: 590, High: 610, Volume: 15_000_000, IsTradable: true}

	rec, ok := exec.Recommend(agent, quote, "momentum macro asymmetric", "risk_on", &FactorSnapshot{})
	if !ok {
		t.Fatal("expected recommendation for Druckenmiller")
	}
	if rec.Agent != "super-dru-01" {
		t.Fatalf("expected agent super-dru-01, got %s", rec.Agent)
	}
	if rec.Conviction < 65 {
		t.Fatalf("expected conviction >= 65 (SuperinvestorMinConviction), got %d", rec.Conviction)
	}
	if rec.ConvictionBreakdown == nil {
		t.Fatal("expected conviction breakdown for transparency")
	}
}

func TestSuperinvestorRecommendAschenbrenner(t *testing.T) {
	exec := SuperinvestorExecutor{}
	agent := domain.AgentSpec{ID: "super-asc-01", Skill: "aschenbrenner_ai_compute", Layer: domain.LayerSuperinvestor}
	quote := domain.Quote{Symbol: "2382.TW", Last: 700, Open: 690, High: 710, Volume: 12_000_000, IsTradable: true}

	rec, ok := exec.Recommend(agent, quote, "ai_capex compute datacenter", "risk_on", &FactorSnapshot{})
	if !ok {
		t.Fatal("expected recommendation for Aschenbrenner")
	}
	if rec.Conviction < 65 {
		t.Fatalf("expected conviction >= 65, got %d", rec.Conviction)
	}
}

func TestSuperinvestorRecommendBaker(t *testing.T) {
	exec := SuperinvestorExecutor{}
	agent := domain.AgentSpec{ID: "super-bak-01", Skill: "baker_deep_tech", Layer: domain.LayerSuperinvestor}
	quote := domain.Quote{Symbol: "2454.TW", Last: 500, Open: 495, High: 505, Volume: 8_000_000, IsTradable: true}

	rec, ok := exec.Recommend(agent, quote, "ip_moat patent differentiation", "risk_on", &FactorSnapshot{})
	if !ok {
		t.Fatal("expected recommendation for Baker")
	}
	if rec.Conviction < 65 {
		t.Fatalf("expected conviction >= 65, got %d", rec.Conviction)
	}
}

func TestSuperinvestorRecommendAckman(t *testing.T) {
	exec := SuperinvestorExecutor{}
	agent := domain.AgentSpec{ID: "super-ack-01", Skill: "ackman_quality", Layer: domain.LayerSuperinvestor}
	quote := domain.Quote{Symbol: "2881.TW", Last: 80, Open: 79, High: 81, Volume: 10_000_000, IsTradable: true}

	rec, ok := exec.Recommend(agent, quote, "quality catalyst compounder", "risk_on", &FactorSnapshot{})
	if !ok {
		t.Fatal("expected recommendation for Ackman")
	}
	if rec.Conviction < 65 {
		t.Fatalf("expected conviction >= 65, got %d", rec.Conviction)
	}
}

func TestSuperinvestorRecommendRejectsWeakSignal(t *testing.T) {
	exec := SuperinvestorExecutor{}
	agent := domain.AgentSpec{ID: "super-dru-01", Skill: "druckenmiller_macro", Layer: domain.LayerSuperinvestor}
	// Weak signal: down day, far from high, low volume, no keywords, risk_off.
	// With floor=50: dynamicSignalStrength(60) + premium(5) - weak_close(-10) - far_from_high(-8) = 47 < 50 = REJECTED.
	quote := domain.Quote{Symbol: "2330.TW", Last: 560, Open: 590, High: 595, Volume: 100_000, IsTradable: true}

	_, ok := exec.Recommend(agent, quote, "", "risk_off", &FactorSnapshot{})
	if ok {
		t.Fatal("expected rejection for weak signal on down day with no keywords")
	}
}

func TestSuperinvestorDualRole(t *testing.T) {
	var _ AgentExecutor = SuperinvestorExecutor{}
	var _ ControlExecutor = SuperinvestorExecutor{}
}

// ─── A6 (perf audit 2026-08-21): RISK_OFF 感知 conviction 門檻 ───
// RISK_OFF 期間有效 floor = max(policy.ConvictionFloor, 70)：conv 60 被擋、
// conv 75 通過；RISK_ON 維持原 floor。

func TestCRORiskExecutorRiskOffRaisesFloor(t *testing.T) {
	executor := CRORiskExecutor{}
	agent := domain.AgentSpec{ID: "cro-01", Skill: "cro_risk"}
	policy := domain.ExecutionPolicy{ConvictionFloor: 60, RequireCROPass: true}
	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 60, Side: domain.SideBuy, Reason: "floor-level"},
		{Agent: "b", Skill: "growth_momentum", Symbol: "2382.TW", Conviction: 75, Side: domain.SideBuy, Reason: "high"},
	}

	// RISK_OFF: effective floor = max(60, 70) = 70 → conv 60 blocked, conv 75 passes.
	out := executor.Apply(agent, recs, policy, domain.RegimeRiskOff)
	if len(out) != 1 {
		t.Fatalf("RISK_OFF: expected 1 rec to survive (conv 75), got %d", len(out))
	}
	if out[0].Symbol != "2382.TW" || out[0].Conviction != 75 {
		t.Fatalf("RISK_OFF: expected conv-75 rec 2382.TW to survive, got %+v", out[0])
	}

	// RISK_ON: floor stays 60 → both conv 60 and conv 75 pass.
	out = executor.Apply(agent, recs, policy, domain.RegimeRiskOn)
	if len(out) != 2 {
		t.Fatalf("RISK_ON: expected both recs to survive (floor 60), got %d", len(out))
	}
}

func TestCRORiskExecutorRiskOffKeepsHigherPolicyFloor(t *testing.T) {
	executor := CRORiskExecutor{}
	agent := domain.AgentSpec{ID: "cro-01", Skill: "cro_risk"}
	policy := domain.ExecutionPolicy{ConvictionFloor: 80, RequireCROPass: true}
	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 75, Side: domain.SideBuy, Reason: "below-80"},
		{Agent: "b", Skill: "growth_momentum", Symbol: "2382.TW", Conviction: 85, Side: domain.SideBuy, Reason: "above-80"},
	}
	out := executor.Apply(agent, recs, policy, domain.RegimeRiskOff)
	if len(out) != 1 || out[0].Symbol != "2382.TW" {
		t.Fatalf("RISK_OFF with floor 80: expected only conv-85 rec to survive, got %+v", out)
	}
}

func TestCRORiskExecutorRiskOffFloorAppliesWithNormalization(t *testing.T) {
	executor := NewCRORiskExecutor()
	agent := domain.AgentSpec{ID: "cro-01", Skill: "cro_risk"}
	policy := domain.ExecutionPolicy{ConvictionFloor: 60, RequireCROPass: true, EnableConvictionNormalization: true}
	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 60, Side: domain.SideBuy, Reason: "floor-level"},
		{Agent: "b", Skill: "growth_momentum", Symbol: "2382.TW", Conviction: 75, Side: domain.SideBuy, Reason: "high"},
	}
	// Single observation per agent → z-score gate passes both; the RISK_OFF
	// absolute-conviction gate (>= 70) must still block conv 60.
	out := executor.Apply(agent, recs, policy, domain.RegimeRiskOff)
	if len(out) != 1 || out[0].Symbol != "2382.TW" {
		t.Fatalf("RISK_OFF with normalization: expected only conv-75 rec to survive, got %+v", out)
	}
}

func TestControlLayerRiskOffBlocksLowConviction(t *testing.T) {
	registry := SeedRegistry()
	plugins := NewPluginRegistry()

	recs := []domain.Recommendation{
		{Agent: "a", Skill: "growth_momentum", Symbol: "2317.TW", Conviction: 60, Side: domain.SideBuy, Reason: "weak-risk-off"},
		{Agent: "b", Skill: "growth_momentum", Symbol: "2382.TW", Conviction: 75, Side: domain.SideBuy, Reason: "strong"},
	}
	final, outcomes := applyControlLayerWithOutcomes(registry, plugins, recs, domain.ExecutionPolicy{ConvictionFloor: 60, RequireCROPass: true}, domain.RegimeRiskOff, nil, "", nil)
	if len(final) != 1 || final[0].Symbol != "2382.TW" {
		t.Fatalf("RISK_OFF control layer: expected only 2382.TW to survive, got %+v", final)
	}
	if len(outcomes) == 0 || outcomes[0].GuardSkill != "cro_risk" || outcomes[0].OutputCount != 1 {
		t.Fatalf("expected CRO guard outcome recording 1 output, got %+v", outcomes)
	}
}
