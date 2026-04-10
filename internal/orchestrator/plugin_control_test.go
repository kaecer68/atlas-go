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
	for _, rec := range out {
		if rec.Agent != "cio-01" {
			t.Fatalf("expected CIO to own final recommendations, got %s", rec.Agent)
		}
		if rec.Conviction < 50 {
			t.Fatalf("expected CRO to filter weak recommendations")
		}
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

	_, outcomes := applyControlLayerWithOutcomes(registry, plugins, recs, DefaultExecutionPolicy())
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
	})
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
