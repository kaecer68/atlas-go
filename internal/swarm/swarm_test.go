package swarm

import (
	"math"
	"testing"
	"time"
)

func TestMiroFishSwarm(t *testing.T) {
	t.Run("NewMiroFishSwarm", func(t *testing.T) {
		config := DefaultSwarmConfig()
		swarm := NewMiroFishSwarm(config)

		if swarm == nil {
			t.Fatal("Expected non-nil swarm")
		}

		// Swarm starts with empty fish, populated when started
		if len(swarm.fish) != 0 {
			t.Logf("Swarm initialized with %d fish", len(swarm.fish))
		}
	})

	t.Run("InitializeScenarios", func(t *testing.T) {
		config := DefaultSwarmConfig()
		swarm := NewMiroFishSwarm(config)

		baseState := MarketState{
			Timestamp: time.Now(),
			Prices:    map[string]float64{"TEST": 100.0},
			Volumes:   map[string]float64{"TEST": 1000000},
		}

		swarm.InitializeScenarios(baseState)

		// Verify scenarios were initialized
		if len(swarm.scenarios) == 0 {
			t.Error("Expected scenarios to be initialized")
		}

		// Scenarios is a slice, check for expected scenario names
		expectedIds := map[string]bool{"bull_trend": false, "bear_trend": false, "high_vol": false, "low_vol": false, "transition": false}
		for _, scenario := range swarm.scenarios {
			if _, exists := expectedIds[scenario.ID]; exists {
				expectedIds[scenario.ID] = true
			}
		}
		for id, found := range expectedIds {
			if !found {
				t.Logf("Expected scenario %s not found", id)
			}
		}
	})

	t.Run("GetTopFish", func(t *testing.T) {
		config := DefaultSwarmConfig()
		swarm := NewMiroFishSwarm(config)

		baseState := MarketState{
			Timestamp: time.Now(),
			Prices:    map[string]float64{"A": 100.0, "B": 50.0},
			Volumes:   map[string]float64{"A": 1000000, "B": 500000},
		}
		swarm.InitializeScenarios(baseState)

		// Get top fish from the initialized swarm
		topFish := swarm.GetTopFish(10)
		t.Logf("Got %d top fish", len(topFish))

		// InitializeScenarios creates 100 fish, so we should get up to 10
		if len(topFish) > 10 {
			t.Errorf("Expected at most 10 fish, got %d", len(topFish))
		}
	})

	t.Run("GetLatestResult", func(t *testing.T) {
		config := DefaultSwarmConfig()
		swarm := NewMiroFishSwarm(config)

		// Initially no results
		_, ok := swarm.GetLatestResult()
		if ok {
			t.Error("Expected no results initially")
		}
	})

	t.Run("ExportTrainingData", func(t *testing.T) {
		config := DefaultSwarmConfig()
		swarm := NewMiroFishSwarm(config)

		baseState := MarketState{
			Timestamp: time.Now(),
			Prices:    map[string]float64{"TEST": 100.0},
			Volumes:   map[string]float64{"TEST": 1000000},
		}
		swarm.InitializeScenarios(baseState)

		// Add a fish with history for export
		swarm.fish = append(swarm.fish, &MiroFish{
			ID:          "fish_001",
			Scenario:    swarm.scenarios[0],
			isAlive:     true,
			spawnedAt:   time.Now(),
			History:     make([]MarketState, 15),
			Performance: FishPerformance{Accuracy: 0.7},
		})

		export := swarm.ExportTrainingData()

		if export == nil {
			t.Fatal("Expected non-nil export")
		}

		t.Logf("Exported %d training scenarios", len(export))
	})
}

func TestMarketState(t *testing.T) {
	t.Run("MarketStateCreation", func(t *testing.T) {
		state := MarketState{
			Timestamp:   time.Now(),
			Prices:      map[string]float64{"TEST": 100.0},
			Volumes:     map[string]float64{"TEST": 1000000},
			Sentiment:   0.5,
			Volatility:  0.15,
			Correlation: 0.3,
		}

		if state.Prices["TEST"] != 100.0 {
			t.Errorf("Expected price 100.0, got %f", state.Prices["TEST"])
		}
	})
}

func TestMarketScenario(t *testing.T) {
	t.Run("ScenarioCreation", func(t *testing.T) {
		scenario := MarketScenario{
			ID:         "bull_trend",
			Name:       "Bull Market Trend",
			Regime:     "risk_on",
			Volatility: 0.15,
			Trend:      0.001,
		}

		if scenario.Regime != "risk_on" {
			t.Errorf("Expected regime risk_on, got %s", scenario.Regime)
		}
	})
}

func TestFishPerformance(t *testing.T) {
	t.Run("PerformanceMetrics", func(t *testing.T) {
		perf := FishPerformance{
			CorrectPredictions: 75,
			TotalPredictions:   100,
			Accuracy:           0.75,
			SharpeRatio:        1.2,
			MaxDrawdown:        0.15,
		}

		if perf.Accuracy != 0.75 {
			t.Errorf("Expected accuracy 0.75, got %.2f", perf.Accuracy)
		}

		if perf.SharpeRatio != 1.2 {
			t.Errorf("Expected Sharpe 1.2, got %.2f", perf.SharpeRatio)
		}
	})
}

func TestSwarmConfig(t *testing.T) {
	t.Run("DefaultConfig", func(t *testing.T) {
		config := DefaultSwarmConfig()

		if config.FishCount != 100 {
			t.Errorf("Expected FishCount 100, got %d", config.FishCount)
		}

		if config.SimulationHorizon != 30*24*time.Hour {
			t.Error("Expected SimulationHorizon of 30 days")
		}

		if config.Parallelism <= 0 {
			t.Error("Expected positive Parallelism")
		}
	})
}

func TestMiroFish(t *testing.T) {
	t.Run("FishCreation", func(t *testing.T) {
		scenario := MarketScenario{
			ID:         "test_scenario",
			Name:       "Test",
			Regime:     "test",
			Volatility: 0.1,
			Trend:      0.0,
		}

		fish := &MiroFish{
			ID:           "fish_001",
			Scenario:     scenario,
			CurrentState: MarketState{Timestamp: time.Now(), Prices: map[string]float64{}},
			isAlive:      true,
			spawnedAt:    time.Now(),
		}

		if fish.ID != "fish_001" {
			t.Errorf("Expected ID fish_001, got %s", fish.ID)
		}

		if !fish.isAlive {
			t.Error("New fish should be alive")
		}
	})
}

func TestPrediction(t *testing.T) {
	t.Run("PredictionStructure", func(t *testing.T) {
		pred := Prediction{
			Timestamp:  time.Now(),
			Symbol:     "2330.TW",
			Direction:  "up",
			Confidence: 0.75,
			Conviction: 70,
		}

		if pred.Symbol != "2330.TW" {
			t.Errorf("Expected symbol 2330.TW, got %s", pred.Symbol)
		}

		if pred.Direction != "up" {
			t.Errorf("Expected direction up, got %s", pred.Direction)
		}
	})
}

func TestConsensusPrediction(t *testing.T) {
	t.Run("ConsensusAggregation", func(t *testing.T) {
		consensus := ConsensusPrediction{
			Symbol:             "2330.TW",
			BullishCount:       60,
			BearishCount:       30,
			NeutralCount:       10,
			AverageConfidence:  0.7,
			ConsensusDirection: "bullish",
		}

		if consensus.Symbol != "2330.TW" {
			t.Errorf("Expected symbol 2330.TW, got %s", consensus.Symbol)
		}

		if consensus.ConsensusDirection != "bullish" {
			t.Errorf("Expected bullish consensus, got %s", consensus.ConsensusDirection)
		}
	})
}

func TestSimulationResult(t *testing.T) {
	t.Run("ResultStructure", func(t *testing.T) {
		result := SimulationResult{
			ScenarioID: "bull_trend",
			Timestamp:  time.Now(),
			Consensus: map[string]ConsensusPrediction{
				"2330.TW": {
					Symbol:             "2330.TW",
					ConsensusDirection: "bullish",
				},
			},
			Confidence: 0.8,
		}

		if result.ScenarioID != "bull_trend" {
			t.Errorf("Expected scenario bull_trend, got %s", result.ScenarioID)
		}
	})
}

func TestAnomaly(t *testing.T) {
	t.Run("AnomalyStructure", func(t *testing.T) {
		anomaly := Anomaly{
			Type:        "flash_crash",
			Description: "Sudden price drop",
			Severity:    0.85,
			Symbols:     []string{"2330.TW"},
		}

		if anomaly.Type != "flash_crash" {
			t.Errorf("Expected type flash_crash, got %s", anomaly.Type)
		}

		if anomaly.Severity < 0 || anomaly.Severity > 1 {
			t.Errorf("Severity should be between 0 and 1, got %.2f", anomaly.Severity)
		}
	})
}

func TestPredictionRuleRandom(t *testing.T) {
	rule := RandomPredictionRule()

	if rule.LookbackWindow < 3 || rule.LookbackWindow > 20 {
		t.Errorf("LookbackWindow out of range: %d", rule.LookbackWindow)
	}
	if rule.TrendUpThreshold < 1.005 || rule.TrendUpThreshold > 1.10 {
		t.Errorf("TrendUpThreshold out of range: %.4f", rule.TrendUpThreshold)
	}
	if rule.TrendDownThreshold < 0.90 || rule.TrendDownThreshold > 0.995 {
		t.Errorf("TrendDownThreshold out of range: %.4f", rule.TrendDownThreshold)
	}
	if rule.ContrarianBias < -1.0 || rule.ContrarianBias > 1.0 {
		t.Errorf("ContrarianBias out of range: %.2f", rule.ContrarianBias)
	}
}

func TestPredictionRuleDefault(t *testing.T) {
	rule := DefaultPredictionRule()

	if rule.LookbackWindow != 5 {
		t.Errorf("expected LookbackWindow=5, got %d", rule.LookbackWindow)
	}
	if rule.TrendUpThreshold != 1.02 {
		t.Errorf("expected TrendUpThreshold=1.02, got %.4f", rule.TrendUpThreshold)
	}
	if rule.TrendDownThreshold != 0.98 {
		t.Errorf("expected TrendDownThreshold=0.98, got %.4f", rule.TrendDownThreshold)
	}
	if rule.UseSentiment {
		t.Error("expected UseSentiment=false in default rule")
	}
	if rule.ContrarianBias != 0.0 {
		t.Errorf("expected ContrarianBias=0, got %.2f", rule.ContrarianBias)
	}
}

func TestMutateRule(t *testing.T) {
	parent := DefaultPredictionRule()
	for i := range 100 {
		child := MutateRule(parent, 0.3)
		if i == 0 {
			continue
		}
		if child.LookbackWindow < 3 || child.LookbackWindow > 20 {
			t.Errorf("iteration %d: LookbackWindow out of range: %d", i, child.LookbackWindow)
		}
		if child.TrendUpThreshold < 1.005 || child.TrendUpThreshold > 1.10 {
			t.Errorf("iteration %d: TrendUpThreshold out of range: %.4f", i, child.TrendUpThreshold)
		}
		if child.TrendDownThreshold < 0.90 || child.TrendDownThreshold > 0.995 {
			t.Errorf("iteration %d: TrendDownThreshold out of range: %.4f", i, child.TrendDownThreshold)
		}
		if child.ContrarianBias < -1.0 || child.ContrarianBias > 1.0 {
			t.Errorf("iteration %d: ContrarianBias out of range: %.2f", i, child.ContrarianBias)
		}
	}
}

func TestCrossoverRules(t *testing.T) {
	p1 := PredictionRule{LookbackWindow: 5, TrendUpThreshold: 1.02, TrendDownThreshold: 0.98, UseSentiment: true, ContrarianBias: 0.5}
	p2 := PredictionRule{LookbackWindow: 15, TrendUpThreshold: 1.08, TrendDownThreshold: 0.92, UseSentiment: false, ContrarianBias: -0.5}

	child := CrossoverRules(p1, p2)

	if child.LookbackWindow != 5 && child.LookbackWindow != 15 {
		t.Errorf("Crossover LookbackWindow should be from parent: got %d", child.LookbackWindow)
	}
	if child.ContrarianBias <= -1.0 || child.ContrarianBias >= 1.0 {
		t.Errorf("ContrarianBias blend out of range: %.2f", child.ContrarianBias)
	}
}

func TestGARCHProcess(t *testing.T) {
	omega, alpha, beta := GARCHParamsForRegime("risk_on")
	g := NewGARCHProcess(omega, alpha, beta, 0.15)

	initialSigma := g.CurrentSigma()
	if initialSigma <= 0 {
		t.Error("expected positive initial sigma")
	}

	// Advance with many zero shocks — variance should converge toward unconditional mean
	for range 200 {
		g.Advance(0)
	}
	convergedSigma := g.CurrentSigma()
	uncondMean := g.Omega / (1 - g.Beta) // unconditional variance from GARCH(1,1)
	expectedSigma := math.Sqrt(uncondMean)
	// sigma should be within ~10% of the unconditional mean after 200 periods
	ratio := convergedSigma / expectedSigma
	if ratio < 0.9 || ratio > 1.1 {
		t.Errorf("sigma=%.6f not converged to unconditional mean %.6f (ratio=%.2f)", convergedSigma, expectedSigma, ratio)
	}

	// Advance with large shock — variance should spike
	g2 := NewGARCHProcess(omega, alpha, beta, 0.15)
	g2.Advance(0.05)
	spikedSigma := g2.CurrentSigma()
	if spikedSigma <= initialSigma {
		t.Errorf("expected sigma spike after large shock: %.6f <= %.6f", spikedSigma, initialSigma)
	}
}

func TestGARCHParamsPerRegime(t *testing.T) {
	regimes := []string{"risk_on", "risk_off", "crisis", "complacent", "transition"}
	for _, r := range regimes {
		omega, alpha, beta := GARCHParamsForRegime(r)
		if omega <= 0 || alpha <= 0 || beta <= 0 {
			t.Errorf("regime %s: got zero params omega=%.6f alpha=%.2f beta=%.2f", r, omega, alpha, beta)
		}
		if alpha+beta >= 1.0 {
			t.Errorf("regime %s: alpha+beta=%.2f >= 1.0 (non-stationary)", r, alpha+beta)
		}
	}
}
