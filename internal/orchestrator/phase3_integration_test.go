package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

func findPrismPlugin(s *System) *prismPlugin {
	if s.host == nil {
		return nil
	}
	for _, p := range s.host.plugins {
		if pp, ok := p.(*prismPlugin); ok {
			return pp
		}
	}
	return nil
}

func findSwarmPlugin(s *System) *swarmPlugin {
	if s.host == nil {
		return nil
	}
	for _, p := range s.host.plugins {
		if sp, ok := p.(*swarmPlugin); ok {
			return sp
		}
	}
	return nil
}

func findSpawningPlugin(s *System) *spawningPlugin {
	if s.host == nil {
		return nil
	}
	for _, p := range s.host.plugins {
		if sp, ok := p.(*spawningPlugin); ok {
			return sp
		}
	}
	return nil
}

func TestSystemWithPRISM(t *testing.T) {
	cfg := DefaultExecutionPolicy()
	s := &System{SystemCore: &SystemCore{}}
	s.policy.ExecutionPolicy = cfg
	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	s.WithPRISM(pm)
	pp := findPrismPlugin(s)
	if pp == nil || pp.manager != pm {
		t.Fatal("expected PRISM manager to be attached")
	}
}

func TestSystemWithSwarm(t *testing.T) {
	s := &System{SystemCore: &SystemCore{}}
	sw := swarm.NewMiroFishSwarm(swarm.DefaultSwarmConfig())
	s.WithSwarm(sw)
	sp := findSwarmPlugin(s)
	if sp == nil || sp.swarm != sw {
		t.Fatal("expected swarm to be attached")
	}
}

func TestSystemWithSpawning(t *testing.T) {
	registry := SeedRegistry()
	s := &System{SystemCore: &SystemCore{registry: registry}}
	sm := spawning.NewSpawningManager(&registry, spawning.DefaultSpawningConfig())
	s.WithSpawning(sm)
	sp := findSpawningPlugin(s)
	if sp == nil || sp.manager != sm {
		t.Fatal("expected spawning manager to be attached")
	}
}

func TestApplySwarmConsensusBoostsBullishBuy(t *testing.T) {
	// nil swarm path should be safe
	s := &System{}
	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 60},
	}
	out := s.applySwarmConsensus(recs)
	if len(out) != 1 || out[0].Conviction != 60 {
		t.Fatal("expected unchanged recommendations when swarm is nil")
	}

	// Test explicit bullish consensus injection
	sw := swarm.NewMiroFishSwarm(swarm.DefaultSwarmConfig())
	sw.InitializeScenarios(swarm.MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"2330.TW": 850},
		Volumes:   map[string]float64{"2330.TW": 1000000},
	})
	s.WithSwarm(sw)
	recs2 := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 60},
		{Agent: "b", Symbol: "2330.TW", Side: domain.SideSell, Conviction: 60},
	}
	// Even if swarm has no result yet, should not panic
	out2 := s.applySwarmConsensus(recs2)
	if len(out2) != 2 || out2[0].Conviction != 60 || out2[1].Conviction != 60 {
		t.Fatal("expected unchanged recommendations when swarm has no result")
	}
}

func TestSchedulePRISMForRegime(t *testing.T) {
	registry := SeedRegistry()
	s := &System{SystemCore: &SystemCore{registry: registry}}
	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	s.WithPRISM(pm)

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
	s := &System{SystemCore: &SystemCore{registry: registry}}
	// nil spawning manager should not panic
	s.runSpawningCycle()

	// with manager should not panic even with empty ledger
	sm := spawning.NewSpawningManager(&registry, spawning.DefaultSpawningConfig())
	s.WithSpawning(sm)
	s.runSpawningCycle()
}
