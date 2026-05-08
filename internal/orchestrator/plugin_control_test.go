package orchestrator

import (
	"testing"

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

	out := applyControlLayer(registry, plugins, recs, DefaultExecutionPolicy())
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

	out := applyControlLayer(registry, plugins, recs, domain.ExecutionPolicy{
		ConvictionFloor: 50,
		RequireCROPass:  false,
	})
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

	_, outcomes := applyControlLayerWithOutcomes(registry, plugins, recs, DefaultExecutionPolicy(), nil, "")
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
	}, nil, "")
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

	out := executor.Apply(agent, recs, DefaultExecutionPolicy())
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

	out := executor.Apply(agent, recs, DefaultExecutionPolicy())
	if len(out) != 2 {
		t.Fatalf("expected 2 outputs, got %d", len(out))
	}
	if out[0].Symbol != "2317.TW" || out[1].Symbol != "2330.TW" {
		t.Fatalf("expected deterministic symbol ordering for tie conviction, got %s then %s", out[0].Symbol, out[1].Symbol)
	}
}
