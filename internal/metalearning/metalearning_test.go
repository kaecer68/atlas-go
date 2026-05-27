package metalearning

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/swarm"
)

func TestTrainingScenarioBridge(t *testing.T) {
	config := swarm.DefaultSwarmConfig()
	config.FishCount = 10
	config.SimulationHorizon = 12 * time.Hour
	config.TimeStep = time.Hour
	config.Parallelism = 2

	sw := swarm.NewMiroFishSwarm(config)
	baseState := swarm.MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	sw.InitializeScenarios(baseState)
	sw.Start()

	trainingData := sw.ExportTrainingData()
	if len(trainingData) == 0 {
		t.Fatal("expected training data from swarm")
	}

	ml := NewMetaLearner(DefaultMetaLearningConfig())
	ml.SubmitTrainingScenarios(trainingData)

	// After submission, top strategies should be available
	top := ml.GetTopStrategies(3)
	if len(top) == 0 {
		t.Fatal("expected top strategies after training data submission")
	}
	t.Logf("Top strategies: %d, population: %d, users: %d", len(top), len(ml.population), len(ml.strategies))

	// Verify strategies have non-zero performance data
	for _, s := range top {
		t.Logf("Strategy %s (type=%s) score=%.4f", s.ID, s.Type, ml.calculateStrategyScore(s))
	}
}

func TestMetaLearnerPersistenceCycle(t *testing.T) {
	config := swarm.DefaultSwarmConfig()
	config.FishCount = 5
	config.SimulationHorizon = 12 * time.Hour
	config.TimeStep = time.Hour

	sw := swarm.NewMiroFishSwarm(config)
	baseState := swarm.MarketState{
		Timestamp: time.Now(),
		Prices:    map[string]float64{"A": 100.0},
		Volumes:   map[string]float64{"A": 1000000},
	}
	sw.InitializeScenarios(baseState)
	sw.Start()

	trainingData := sw.ExportTrainingData()

	ml1 := NewMetaLearner(DefaultMetaLearningConfig())
	ml1.SubmitTrainingScenarios(trainingData)

	// Save state
	path := t.TempDir() + "/metalearner_state.json"
	if err := ml1.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load into fresh MetaLearner
	ml2 := NewMetaLearner(DefaultMetaLearningConfig())
	if err := ml2.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(ml2.strategies) == 0 {
		t.Fatal("expected strategies restored from persistence")
	}
	t.Logf("Restored %d strategies from disk", len(ml2.strategies))
}

func TestBridgeFromTrainingScenario(t *testing.T) {
	scenario := swarm.TrainingScenario{
		ID:       "fish_bull_0",
		Scenario: "Bull Market Trend",
		Performance: swarm.FishPerformance{
			Accuracy:    0.75,
			SharpeRatio: 1.2,
			MaxDrawdown: 0.12,
		},
		States: make([]swarm.MarketState, 200),
	}

	data := scenarioToLearningData(scenario)
	if data.FinalAccuracy != 0.75 {
		t.Errorf("expected accuracy 0.75, got %f", data.FinalAccuracy)
	}
	if data.LearningRate != 0.01 {
		t.Errorf("expected lr 0.01 for acc 0.75, got %f", data.LearningRate)
	}
	if data.BatchSize != 32 {
		t.Errorf("expected batch size 32 for 200 steps, got %d", data.BatchSize)
	}
	if data.Stability <= 0 {
		t.Errorf("expected positive stability from drawdown %.2f, got %.2f", scenario.Performance.MaxDrawdown, data.Stability)
	}
}
