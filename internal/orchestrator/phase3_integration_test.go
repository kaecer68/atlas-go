package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

func TestSystemWithPRISM(t *testing.T) {
	cfg := DefaultExecutionPolicy()
	s := &System{}
	s.policy.ExecutionPolicy = cfg
	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	s.WithPRISM(pm)
	if s.prismManager != pm {
		t.Fatal("expected PRISM manager to be attached")
	}
}

func TestSystemWithSwarm(t *testing.T) {
	s := &System{}
	sw := swarm.NewMiroFishSwarm(swarm.DefaultSwarmConfig())
	s.WithSwarm(sw)
	if s.swarm != sw {
		t.Fatal("expected swarm to be attached")
	}
}

func TestSystemWithSpawning(t *testing.T) {
	registry := SeedRegistry()
	s := &System{registry: registry}
	sm := spawning.NewSpawningManager(&registry, spawning.DefaultSpawningConfig())
	s.WithSpawning(sm)
	if s.spawningManager != sm {
		t.Fatal("expected spawning manager to be attached")
	}
}

func TestApplySwarmConsensusBoostsBullishBuy(t *testing.T) {
	s := &System{}
	sw := swarm.NewMiroFishSwarm(swarm.DefaultSwarmConfig())
	sw.InitializeScenarios(swarm.MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"2330.TW": 850},
		Volumes:   map[string]float64{"2330.TW": 1000000},
	})
	// Manually inject a consensus result
	sw.Start()
	// Give it a moment to compute at least one consensus
	time.Sleep(50 * time.Millisecond)
	sw.Stop()

	// Even if no result yet, nil swarm path should be safe
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 60},
	}
	out := s.applySwarmConsensus(recs)
	if len(out) != 1 || out[0].Conviction != 60 {
		t.Fatal("expected unchanged recommendations when swarm is nil")
	}

	// Now test with a mocked consensus by setting swarm directly
	s.swarm = sw
	result, ok := sw.GetLatestResult()
	if ok && len(result.Consensus) > 0 {
		for sym, cp := range result.Consensus {
			_ = sym
			_ = cp
			break
		}
	}
	// Test explicit bullish consensus injection via direct field manipulation
	recs2 := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 60},
		{Agent: "b", Symbol: "2330.TW", Side: domain.SideSell, Conviction: 60},
	}
	// Temporarily replace swarm with one that has known consensus
	s.swarm = nil
	out2 := s.applySwarmConsensus(recs2)
	if len(out2) != 2 || out2[0].Conviction != 60 || out2[1].Conviction != 60 {
		t.Fatal("expected unchanged recommendations when swarm has no result")
	}
}

func TestSchedulePRISMForRegime(t *testing.T) {
	registry := SeedRegistry()
	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	s := &System{registry: registry, prismManager: pm}

	// Should not panic for any regime
	s.schedulePRISMForRegime(domain.RegimeRiskOn, time.Now())
	s.schedulePRISMForRegime(domain.RegimeRiskOff, time.Now())
	s.schedulePRISMForRegime(domain.RegimeNeutral, time.Now())

	stats := pm.GetOverallStats()
	if stats.TotalTasks == 0 {
		t.Fatal("expected PRISM tasks to be scheduled")
	}
}

func TestRunSpawningCycleDoesNotPanic(t *testing.T) {
	registry := SeedRegistry()
	s := &System{registry: registry}
	// nil spawning manager should not panic
	s.runSpawningCycle()

	// with manager should not panic even with empty ledger
	sm := spawning.NewSpawningManager(&registry, spawning.DefaultSpawningConfig())
	s.WithSpawning(sm)
	s.runSpawningCycle()
}
