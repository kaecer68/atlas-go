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
