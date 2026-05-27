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
