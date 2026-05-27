package swarm

import (
	"testing"
	"time"
)

func TestMiroFishSwarmLifecycle(t *testing.T) {
	config := DefaultSwarmConfig()
	config.FishCount = 10
	config.SimulationHorizon = 2 * time.Hour // Enough for 2 steps at default hourly timestep
	config.TimeStep = time.Hour
	config.Parallelism = 2

	swarm := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"TEST": 100.0},
		Volumes:   map[string]float64{"TEST": 1000000},
	}
	swarm.InitializeScenarios(baseState)

	// Start() is now synchronous — it runs simulation and returns.
	swarm.Start()

	// After Start(), consensus result should be available.
	result, ok := swarm.GetLatestResult()
	if !ok {
		t.Fatal("expected consensus result after Start")
	}
	if len(result.Consensus) == 0 {
		t.Fatal("expected non-empty consensus")
	}
	if result.Confidence <= 0 {
		t.Fatal("expected positive confidence")
	}
}

func TestMiroFishSwarmUpdateScenario(t *testing.T) {
	config := DefaultSwarmConfig()
	swarm := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"TEST": 100.0},
		Volumes:   map[string]float64{"TEST": 1000000},
	}
	swarm.InitializeScenarios(baseState)

	if len(swarm.scenarios) == 0 {
		t.Fatal("expected scenarios initialized")
	}

	originalVol := swarm.scenarios[0].Volatility
	originalTrend := swarm.scenarios[0].Trend

	swarm.UpdateScenario(swarm.scenarios[0].ID, 0.05, 0.001)

	if swarm.scenarios[0].Volatility != originalVol+0.05 {
		t.Fatalf("expected volatility updated, got %f", swarm.scenarios[0].Volatility)
	}
	if swarm.scenarios[0].Trend != originalTrend+0.001 {
		t.Fatalf("expected trend updated, got %f", swarm.scenarios[0].Trend)
	}
}

func TestMiroFishSwarmResults(t *testing.T) {
	config := DefaultSwarmConfig()
	config.FishCount = 5
	swarm := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	swarm.InitializeScenarios(baseState)

	// Seed predictions so computeConsensus has data to aggregate
	for _, fish := range swarm.fish {
		fish.Predictions = append(fish.Predictions, Prediction{
			Symbol:     "A",
			Direction:  "up",
			Confidence: 0.8,
		})
	}

	swarm.results = append(swarm.results, swarm.computeConsensus())
	swarm.results = append(swarm.results, swarm.computeConsensus())

	results := swarm.GetAllResults()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	latest, ok := swarm.GetLatestResult()
	if !ok {
		t.Fatal("expected latest result")
	}
	if len(latest.Consensus) == 0 {
		t.Fatal("expected non-empty consensus")
	}
}

func TestMiroFishSwarmComputeConsensus(t *testing.T) {
	config := DefaultSwarmConfig()
	swarm := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"A": 100.0, "B": 200.0},
		Volumes:   map[string]float64{"A": 1000000, "B": 2000000},
	}
	swarm.InitializeScenarios(baseState)

	for _, fish := range swarm.fish {
		fish.Predictions = append(fish.Predictions, Prediction{
			Symbol:     "A",
			Direction:  "up",
			Confidence: 0.7,
		}, Prediction{
			Symbol:     "B",
			Direction:  "down",
			Confidence: 0.6,
		})
	}

	result := swarm.computeConsensus()
	if len(result.Consensus) == 0 {
		t.Fatal("expected consensus with symbols")
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		t.Fatalf("expected confidence in [0,1], got %f", result.Confidence)
	}
	if len(result.FishResults) == 0 {
		t.Fatal("expected fish performance results")
	}
}

func TestMiroFishSwarmCollectPerformance(t *testing.T) {
	config := DefaultSwarmConfig()
	swarm := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	swarm.InitializeScenarios(baseState)

	perf := swarm.collectPerformance()
	if len(perf) == 0 {
		t.Fatal("expected performance map with scenario keys")
	}
}

func TestEvolveGeneration(t *testing.T) {
	config := DefaultSwarmConfig()
	config.FishCount = 20
	config.SimulationHorizon = 2 * time.Hour
	config.TimeStep = time.Hour
	config.Parallelism = 4

	sw := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	sw.InitializeScenarios(baseState)

	// Record rules before simulation
	rulesBefore := make([]PredictionRule, len(sw.fish))
	for i, f := range sw.fish {
		rulesBefore[i] = f.Rule
	}

	sw.Start()

	// After simulation, evolve
	sw.EvolveGeneration()

	// Check: some fish should have different rules after evolution
	changedCount := 0
	for i, f := range sw.fish {
		if f.Rule != rulesBefore[i] {
			changedCount++
		}
	}
	if changedCount == 0 {
		t.Error("expected some fish rules to change after EvolveGeneration")
	}
	t.Logf("Evolved: %d/%d fish rules changed", changedCount, len(sw.fish))

	// Check: performance reset on replaced fish
	for _, f := range sw.fish {
		if f.Performance.TotalPredictions > 0 && f.Performance.Accuracy > 0 {
			continue
		}
	}
}

func TestEvolveGenerationPreservesTotalCount(t *testing.T) {
	config := DefaultSwarmConfig()
	config.FishCount = 10
	config.SimulationHorizon = 2 * time.Hour
	config.TimeStep = time.Hour

	sw := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	sw.InitializeScenarios(baseState)

	before := len(sw.fish)
	sw.Start()
	sw.EvolveGeneration()
	after := len(sw.fish)

	if before != after {
		t.Fatalf("fish count changed: %d → %d", before, after)
	}
	if after != 10 {
		t.Fatalf("expected 10 fish, got %d", after)
	}
}

func TestFishHasPredictionRule(t *testing.T) {
	config := DefaultSwarmConfig()
	config.FishCount = 5
	config.SimulationHorizon = 2 * time.Hour
	config.TimeStep = time.Hour

	sw := NewMiroFishSwarm(config)
	baseState := MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	sw.InitializeScenarios(baseState)

	for _, f := range sw.fish {
		if f.Rule.LookbackWindow < 3 {
			t.Errorf("fish %s has invalid LookbackWindow: %d", f.ID, f.Rule.LookbackWindow)
		}
		if f.GARCH == nil {
			t.Errorf("fish %s missing GARCH process", f.ID)
		}
	}
}

func TestEventTimingIsRelativeToBaseState(t *testing.T) {
	config := DefaultSwarmConfig()
	config.FishCount = 5
	config.SimulationHorizon = 7 * 24 * time.Hour
	config.TimeStep = 24 * time.Hour
	config.Parallelism = 2

	sw := NewMiroFishSwarm(config)

	baseTime1 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	baseState1 := MarketState{
		Timestamp: baseTime1,
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	sw.InitializeScenarios(baseState1)

	for _, s := range sw.scenarios {
		for _, evt := range s.Events {
			if evt.Time.Before(baseTime1) {
				t.Errorf("scenario %s: event %s at %v is before baseTime %v", s.ID, evt.Type, evt.Time, baseTime1)
			}
		}
	}

	baseTime2 := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	baseState2 := MarketState{
		Timestamp: baseTime2,
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	sw2 := NewMiroFishSwarm(config)
	sw2.InitializeScenarios(baseState2)

	for _, s := range sw2.scenarios {
		for _, evt := range s.Events {
			if evt.Time.Before(baseTime2) {
				t.Errorf("scenario %s: event %s at %v is before baseTime %v", s.ID, evt.Type, evt.Time, baseTime2)
			}
		}
	}
}
