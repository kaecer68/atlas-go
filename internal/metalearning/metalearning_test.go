package metalearning

import (
	"testing"
	"time"
)

func TestMetaLearner(t *testing.T) {
	t.Run("NewMetaLearner", func(t *testing.T) {
		config := DefaultMetaLearningConfig()
		ml := NewMetaLearner(config)

		if ml == nil {
			t.Fatal("Expected non-nil MetaLearner")
		}

		if len(ml.strategies) == 0 {
			t.Error("Expected strategies to be initialized")
		}

		if len(ml.population) == 0 {
			t.Error("Expected population to be initialized")
		}
	})

	t.Run("DefaultConfig", func(t *testing.T) {
		config := DefaultMetaLearningConfig()

		if config.PopulationSize != 20 {
			t.Errorf("Expected PopulationSize 20, got %d", config.PopulationSize)
		}

		if config.EliteRatio != 0.2 {
			t.Errorf("Expected EliteRatio 0.2, got %f", config.EliteRatio)
		}

		if config.MutationRate != 0.15 {
			t.Errorf("Expected MutationRate 0.15, got %f", config.MutationRate)
		}
	})

	t.Run("GetBestStrategy", func(t *testing.T) {
		config := DefaultMetaLearningConfig()
		ml := NewMetaLearner(config)

		// Initially should return nil or first strategy
		best := ml.GetBestStrategy()
		if best == nil {
			t.Log("No best strategy yet (expected before training)")
		} else {
			t.Logf("Best strategy: %s", best.ID)
		}
	})

	t.Run("SubmitSwarmData", func(t *testing.T) {
		config := DefaultMetaLearningConfig()
		ml := NewMetaLearner(config)

		data := SwarmLearningData{
			FishID:           "fish_001",
			Scenario:         "bull",
			LearningRate:     0.01,
			BatchSize:        32,
			Epochs:           10,
			FinalAccuracy:    0.78,
			ConvergenceSpeed: 50.0,
			Stability:        0.85,
			Timestamp:        time.Now(),
			StrategyParams: map[string]float64{
				"learning_rate": 0.01,
				"momentum":      0.9,
			},
		}

		// Should not panic
		ml.SubmitSwarmData(data)

		// Data should be queued
		t.Log("Swarm data submitted successfully")
	})

	t.Run("SubmitTrainingResult", func(t *testing.T) {
		config := DefaultMetaLearningConfig()
		ml := NewMetaLearner(config)

		result := TrainingResult{
			AgentID:      "agent_001",
			StrategyID:   "strategy_001",
			InitialScore: 0.5,
			FinalScore:   0.75,
			Improvement:  0.25,
			TrainingTime: 3600,
			Converged:    true,
			Timestamp:    time.Now(),
		}

		// Should not panic
		ml.SubmitTrainingResult(result)

		t.Log("Training result submitted successfully")
	})

	t.Run("StrategyTypes", func(t *testing.T) {
		types := []StrategyType{
			StrategyMomentum,
			StrategyAdaptive,
			StrategyCurriculum,
			StrategyEnsemble,
			StrategyEvolutionary,
		}

		for _, st := range types {
			if st == "" {
				t.Error("Strategy type should not be empty")
			}
		}
	})

	t.Run("RecommendStrategyForAgent", func(t *testing.T) {
		config := DefaultMetaLearningConfig()
		ml := NewMetaLearner(config)

		strategy := ml.RecommendStrategyForAgent("agent_001", StrategyMomentum)

		if strategy == nil {
			t.Fatal("Expected a recommended strategy")
		}

		t.Logf("Recommended strategy: %s (type: %s)", strategy.Name, strategy.Type)
	})

	t.Run("GetTopStrategies", func(t *testing.T) {
		config := DefaultMetaLearningConfig()
		ml := NewMetaLearner(config)

		top := ml.GetTopStrategies(5)

		if len(top) == 0 {
			t.Error("Expected some top strategies")
		}

		if len(top) > 5 {
			t.Errorf("Expected at most 5 strategies, got %d", len(top))
		}

		t.Logf("Got %d top strategies", len(top))
	})

	t.Run("GenerateReport", func(t *testing.T) {
		config := DefaultMetaLearningConfig()
		ml := NewMetaLearner(config)

		report := ml.GenerateReport()

		if report == nil {
			t.Fatal("Expected non-nil report")
		}

		if report.TotalStrategies == 0 {
			t.Error("Expected non-zero total strategies")
		}

		t.Logf("Report: %d strategies, %d population size",
			report.TotalStrategies, report.PopulationSize)
	})
}

func TestLearningStrategy(t *testing.T) {
	t.Run("StrategyCreation", func(t *testing.T) {
		strategy := &LearningStrategy{
			ID:   "test_strategy",
			Name: "Test Strategy",
			Type: StrategyMomentum,
			Parameters: map[string]float64{
				"learning_rate": 0.01,
				"momentum":      0.9,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if strategy.ID != "test_strategy" {
			t.Errorf("Expected ID test_strategy, got %s", strategy.ID)
		}

		if strategy.Type != StrategyMomentum {
			t.Errorf("Expected type momentum, got %s", strategy.Type)
		}
	})

	t.Run("StrategyPerformance", func(t *testing.T) {
		perf := &StrategyPerformance{
			StrategyID:        "perf_test",
			TotalApplications: 100,
			SuccessCount:      75,
			FailureCount:      25,
			AvgImprovement:    0.15,
			BestImprovement:   0.5,
			WorstImprovement:  -0.1,
			ConvergenceRate:   0.8,
			StabilityScore:    0.75,
			LastEvaluated:     time.Now(),
		}

		successRate := float64(perf.SuccessCount) / float64(perf.TotalApplications)
		if successRate != 0.75 {
			t.Errorf("Expected success rate 0.75, got %f", successRate)
		}
	})
}

func TestMetaLearningPersistence(t *testing.T) {
	t.Run("SaveAndLoad", func(t *testing.T) {
		config := DefaultMetaLearningConfig()
		ml1 := NewMetaLearner(config)

		ml1.SubmitTrainingResult(TrainingResult{
			AgentID:     "agent_001",
			StrategyID:  "strategy_momentum_0",
			Improvement: 0.2,
			Converged:   true,
			Timestamp:   time.Now(),
		})

		tempFile := "/tmp/test_metalearning.json"
		err := ml1.Save(tempFile)
		if err != nil {
			t.Fatalf("Failed to save: %v", err)
		}

		ml2 := NewMetaLearner(config)
		err = ml2.Load(tempFile)
		if err != nil {
			t.Fatalf("Failed to load: %v", err)
		}

		if len(ml2.strategies) == 0 {
			t.Error("Expected strategies to be loaded")
		}

		t.Logf("Loaded %d strategies", len(ml2.strategies))
	})
}

func TestCrossover(t *testing.T) {
	config := DefaultMetaLearningConfig()
	ml := NewMetaLearner(config)

	parent1 := &LearningStrategy{
		ID:   "parent1",
		Name: "Parent 1",
		Type: StrategyMomentum,
		Parameters: map[string]float64{
			"learning_rate": 0.01,
			"momentum":     0.9,
		},
	}
	parent2 := &LearningStrategy{
		ID:   "parent2",
		Name: "Parent 2",
		Type: StrategyMomentum,
		Parameters: map[string]float64{
			"learning_rate": 0.02,
			"momentum":     0.95,
		},
	}

	child := ml.crossover(parent1, parent2, "child1")

	if child.ID != "child1" {
		t.Errorf("Expected child ID 'child1', got %s", child.ID)
	}
	if child.Type != StrategyMomentum {
		t.Errorf("Expected type StrategyMomentum, got %s", child.Type)
	}
	if len(child.Parameters) == 0 {
		t.Error("Expected child to have parameters")
	}
}

func TestCalculateParameterSimilarity(t *testing.T) {
	config := DefaultMetaLearningConfig()
	ml := NewMetaLearner(config)

	p1 := map[string]float64{"a": 1.0, "b": 2.0}
	p2 := map[string]float64{"a": 1.0, "b": 3.0}

	sim := ml.calculateParameterSimilarity(p1, p2)
	if sim <= 0 {
		t.Errorf("Expected positive similarity, got %f", sim)
	}
	if sim >= 1 {
		t.Errorf("Expected similarity < 1 for different params, got %f", sim)
	}

	sim2 := ml.calculateParameterSimilarity(p1, p1)
	if sim2 != 1.0 {
		t.Errorf("Expected similarity 1.0 for identical params, got %f", sim2)
	}

	sim3 := ml.calculateParameterSimilarity(map[string]float64{}, p1)
	if sim3 != 0.0 {
		t.Errorf("Expected 0 for empty first param, got %f", sim3)
	}

	sim4 := ml.calculateParameterSimilarity(p1, map[string]float64{})
	if sim4 != 0.0 {
		t.Errorf("Expected 0 for empty second param, got %f", sim4)
	}
}

func TestProcessSwarmData(t *testing.T) {
	config := DefaultMetaLearningConfig()
	ml := NewMetaLearner(config)

	data := SwarmLearningData{
		FishID:         "fish_001",
		Scenario:       "bull",
		LearningRate:   0.01,
		BatchSize:      32,
		Epochs:         10,
		FinalAccuracy:  0.85,
		ConvergenceSpeed: 30.0,
		Stability:      0.9,
		Timestamp:      time.Now(),
		StrategyParams: map[string]float64{
			"learning_rate": 0.01,
			"momentum":      0.9,
		},
	}

	ml.processSwarmData(data)
}

func TestProcessTrainingResult(t *testing.T) {
	config := DefaultMetaLearningConfig()
	ml := NewMetaLearner(config)

	result := TrainingResult{
		AgentID:      "agent_001",
		StrategyID:   "strategy_momentum_0",
		InitialScore: 0.5,
		FinalScore:   0.7,
		Improvement:  0.2,
		TrainingTime: 3600,
		Converged:    true,
		Timestamp:    time.Now(),
	}

	ml.processTrainingResult(result)

	if len(ml.strategies) == 0 {
		t.Error("Expected strategies to be populated")
	}
}
