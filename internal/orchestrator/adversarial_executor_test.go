package orchestrator

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func TestAdversarialScenarioRunnerWithRealReplay(t *testing.T) {
	ds, err := replay.LoadTWSEOpenDataCSV("../../data/replay/tw_extended_90days.csv")
	if err != nil {
		t.Fatalf("cannot load replay data: %v", err)
	}
	registry := SeedRegistry()
	runner := NewAdversarialScenarioRunner(ds, registry)

	agent := domain.AgentSpec{ID: "semi-desk-01", Skill: "semiconductor_desk", Enabled: true}
	result := runner.RunStressTest(agent.ID, agent)

	if len(result.Scenarios) == 0 {
		t.Fatal("expected at least one scenario result")
	}
	t.Logf("Adversarial stress test for %s: overall=%.2f passed=%v", agent.ID, result.OverallScore, result.Passed)
	for _, sr := range result.Scenarios {
		t.Logf("  %s: score=%.2f passed=%v details=%s", sr.ScenarioType, sr.Score, sr.Passed, sr.Details)
	}
}

func TestAdversarialQuoteMutators(t *testing.T) {
	runner := NewAdversarialScenarioRunner(nil, SeedRegistry())
	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 100, High: 105, Low: 95, Volume: 1000000},
		{Symbol: "2317.TW", Last: 50, High: 52, Low: 48, Volume: 500000},
	}

	flashCrash := runner.mutateFlashCrash(quotes)
	if flashCrash[0].Last != 80 {
		t.Fatalf("expected flash crash 20%% drop, got %.2f", flashCrash[0].Last)
	}

	liquidity := runner.mutateLiquidityCrisis(quotes)
	if liquidity[0].Volume != 1 || liquidity[1].Volume != 1 {
		t.Fatal("expected liquidity crisis volume=1")
	}

	correlation := runner.mutateCorrelationSpike(quotes)
	if math.Abs(correlation[0].Last-97) > 0.01 || math.Abs(correlation[1].Last-48.5) > 0.01 {
		t.Fatalf("expected correlation spike uniform -3%%, got %.2f / %.2f", correlation[0].Last, correlation[1].Last)
	}

	rally := runner.mutateFlashRally(quotes)
	if math.Abs(rally[0].Last-115) > 0.01 {
		t.Fatalf("expected flash rally 15%% up, got %.2f", rally[0].Last)
	}

	rotation := runner.mutateSectorRotation(quotes)
	if rotation[0].Last != 108 {
		t.Fatalf("expected sector rotation boost for 2330.TW, got %.2f", rotation[0].Last)
	}
}

func TestPhase3ControllerRunsAdversarialStressTests(t *testing.T) {
	registry := SeedRegistry()
	ds, _ := replay.LoadTWSEOpenDataCSV("../../data/replay/tw_extended_90days.csv")
	runner := NewAdversarialScenarioRunner(ds, registry)

	ctrl := NewPhase3Controller(&registry, nil, nil, nil, nil, nil)
	ctrl.WithAdversarialRunner(runner)
	ctrl.prismWeightCache["semi-desk-01"] = 0.1 // mark as weakest

	// Should not panic even with nil swarm/prism
	ctrl.runAdversarialStressTests()
}
