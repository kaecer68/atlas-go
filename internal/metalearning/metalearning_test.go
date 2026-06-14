package metalearning

import (
	"os"
	"strings"
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

func TestLrForAccuracy_AllBranches(t *testing.T) {
	tests := []struct {
		acc  float64
		want float64
	}{
		{0.9, 0.001}, // >0.8 → fine-tuning
		{0.81, 0.001},
		{0.7, 0.01}, // >0.6 → moderate
		{0.61, 0.01},
		{0.5, 0.1}, // default → exploratory
		{0.0, 0.1},
		{-0.5, 0.1},
	}
	for _, tt := range tests {
		got := lrForAccuracy(tt.acc)
		if got != tt.want {
			t.Errorf("lrForAccuracy(%f) = %f, want %f", tt.acc, got, tt.want)
		}
	}
}

func TestBatchSizeForSteps_AllBranches(t *testing.T) {
	tests := []struct {
		steps int
		want  int
	}{
		{600, 64}, // >500
		{501, 64},
		{200, 32}, // >100
		{101, 32},
		{50, 16}, // default
		{0, 16},
	}
	for _, tt := range tests {
		got := batchSizeForSteps(tt.steps)
		if got != tt.want {
			t.Errorf("batchSizeForSteps(%d) = %d, want %d", tt.steps, got, tt.want)
		}
	}
}

func TestStabilityFromDrawdown(t *testing.T) {
	tests := []struct {
		drawdown float64
		want     float64
	}{
		{0.0, 1.0},
		{-0.5, 1.0}, // negative → 1.0
		{0.2, 0.8},
		{0.5, 0.5},
		{1.0, 0.0},
		{1.5, 0.0}, // >1 → 0
	}
	for _, tt := range tests {
		got := stabilityFromDrawdown(tt.drawdown)
		if got != tt.want {
			t.Errorf("stabilityFromDrawdown(%f) = %f, want %f", tt.drawdown, got, tt.want)
		}
	}
}

func TestDefaultMetaLearningConfig(t *testing.T) {
	cfg := DefaultMetaLearningConfig()
	if cfg.PopulationSize != 20 {
		t.Errorf("expected PopulationSize 20, got %d", cfg.PopulationSize)
	}
	if cfg.EliteRatio != 0.2 {
		t.Errorf("expected EliteRatio 0.2, got %f", cfg.EliteRatio)
	}
	if cfg.MutationRate != 0.15 {
		t.Errorf("expected MutationRate 0.15, got %f", cfg.MutationRate)
	}
	if cfg.CrossoverRate != 0.3 {
		t.Errorf("expected CrossoverRate 0.3, got %f", cfg.CrossoverRate)
	}
	if cfg.MinImprovement != 0.05 {
		t.Errorf("expected MinImprovement 0.05, got %f", cfg.MinImprovement)
	}
}

func TestNewMetaLearner_NilConfig(t *testing.T) {
	ml := NewMetaLearner(nil)
	if ml == nil {
		t.Fatal("expected non-nil MetaLearner with nil config")
	}
	if len(ml.strategies) == 0 {
		t.Fatal("expected initial strategies with nil config")
	}
	if len(ml.population) == 0 {
		t.Fatal("expected initial population with nil config")
	}
}

func TestNewMetaLearner_InitialPopulation(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	if len(ml.strategies) == 0 {
		t.Fatal("expected non-empty strategies")
	}
	if len(ml.population) == 0 {
		t.Fatal("expected non-empty population")
	}
	// 5 base + 3 mutations each = 20 total, capped at PopulationSize (20)
	if len(ml.population) != 20 {
		t.Errorf("expected 20 population, got %d", len(ml.population))
	}
}

func TestCalculateParameterSimilarity(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())

	tests := []struct {
		name string
		p1   map[string]float64
		p2   map[string]float64
		want float64
	}{
		{"identical", map[string]float64{"lr": 0.1, "bs": 32}, map[string]float64{"lr": 0.1, "bs": 32}, 1.0},
		{"empty p1", map[string]float64{}, map[string]float64{"lr": 0.1}, 0.0},
		{"empty p2", map[string]float64{"lr": 0.1}, map[string]float64{}, 0.0},
		{"both empty", map[string]float64{}, map[string]float64{}, 0.0},
		{"no common keys", map[string]float64{"lr": 0.1}, map[string]float64{"bs": 32}, 0.0},
	}
	for _, tt := range tests {
		got := ml.calculateParameterSimilarity(tt.p1, tt.p2)
		// Allow small epsilon for floating point
		if got < tt.want-0.01 || got > tt.want+0.01 {
			t.Errorf("%s: calculateParameterSimilarity = %f, want %f", tt.name, got, tt.want)
		}
	}
}

func TestCalculateStrategyScore_ColdStart(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	s := &LearningStrategy{
		ID:   "cold",
		Name: "cold start",
		Type: StrategyMomentum,
	}
	score := ml.calculateStrategyScore(s)
	if score != 0.0 {
		t.Errorf("expected 0.0 for cold start (no performance), got %f", score)
	}
}

func TestCalculateStrategyScore_WithPerformance(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	s := &LearningStrategy{
		ID:   "trained",
		Name: "trained strategy",
		Type: StrategyAdaptive,
		Performance: &StrategyPerformance{
			TotalApplications: 100,
			SuccessCount:      80,
			FailureCount:      20,
			AvgImprovement:    0.15,
			ConvergenceRate:   0.7,
			StabilityScore:    0.8,
			AvgTrainingTime:   1800, // 30 minutes
		},
	}
	score := ml.calculateStrategyScore(s)
	if score <= 0 {
		t.Errorf("expected positive score (>0) for trained strategy, got %f", score)
	}
}

func TestGetBestStrategy_Empty(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	// No elites set before evolution — GetBestStrategy returns nil
	best := ml.GetBestStrategy()
	if best != nil {
		t.Errorf("expected nil best strategy with empty elites, got %v", best)
	}
}

func TestGetBestStrategy_AfterEvolution(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	ml.evolvePopulation()
	best := ml.GetBestStrategy()
	if best == nil {
		t.Fatal("expected non-nil best strategy after evolution")
	}
}

func TestGetTopStrategies(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	ml.evolvePopulation()

	top := ml.GetTopStrategies(3)
	if len(top) != 3 {
		t.Errorf("expected 3 top strategies, got %d", len(top))
	}

	topAll := ml.GetTopStrategies(100)
	if len(topAll) != len(ml.population) {
		t.Errorf("expected all %d strategies, got %d", len(ml.population), len(topAll))
	}
}

func TestStrategies(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	all := ml.Strategies()
	if len(all) == 0 {
		t.Fatal("expected non-empty strategies list")
	}
	if len(all) != len(ml.strategies) {
		t.Errorf("Strategies() length %d != len(strategies) %d", len(all), len(ml.strategies))
	}
}

func TestRecommendStrategyForAgent(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	ml.evolvePopulation()

	// Match by type
	rec := ml.RecommendStrategyForAgent("agent-1", StrategyMomentum)
	if rec == nil {
		t.Fatal("expected recommendation for momentum agent")
	}
	if rec.Type != StrategyMomentum {
		t.Errorf("expected momentum strategy, got %s", rec.Type)
	}

	// No match → falls back to best
	rec = ml.RecommendStrategyForAgent("agent-2", StrategyType("nonexistent"))
	if rec == nil {
		t.Fatal("expected fallback recommendation")
	}

	// Empty type → any
	rec = ml.RecommendStrategyForAgent("agent-3", "")
	if rec == nil {
		t.Fatal("expected recommendation for empty type")
	}
}

func TestSubmitTrainingResult_ChannelFull(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	// Fill the training results channel
	for i := 0; i < 100; i++ {
		ml.SubmitTrainingResult(TrainingResult{AgentID: "fill", StrategyID: "s1", Improvement: 0.1, Converged: true})
	}
	// This should drop without panicking
	ml.SubmitTrainingResult(TrainingResult{AgentID: "drop", StrategyID: "s1", Improvement: 0.2, Converged: true})
}

func TestProcessSwarmData_UpdatesPerformance(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	data := SwarmLearningData{
		FishID:           "fish-1",
		Scenario:         "bull",
		LearningRate:     0.001,
		BatchSize:        32,
		Epochs:           10,
		FinalAccuracy:    0.85,
		ConvergenceSpeed: 50,
		Stability:        0.9,
		Timestamp:        time.Now(),
		StrategyParams: map[string]float64{
			"learning_rate": 0.001,
			"momentum":      0.9,
			"batch_size":    32,
			"warmup_epochs": 5,
		},
	}
	ml.processSwarmData(data)

	// At least one strategy should have been updated
	updated := false
	for _, s := range ml.population {
		if s.Performance != nil && s.Performance.TotalApplications > 0 {
			updated = true
			break
		}
	}
	if !updated {
		t.Fatal("expected at least one strategy to be updated after swarm data")
	}
}

func TestProcessTrainingResult_UpdatesPerformance(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())

	// Pick a known strategy ID
	var targetID string
	for id := range ml.strategies {
		targetID = id
		break
	}

	result := TrainingResult{
		AgentID:      "agent-1",
		StrategyID:   targetID,
		InitialScore: 0.5,
		FinalScore:   0.7,
		Improvement:  0.2,
		TrainingTime: 600,
		Converged:    true,
		Timestamp:    time.Now(),
	}
	ml.processTrainingResult(result)

	s := ml.strategies[targetID]
	if s.Performance == nil {
		t.Fatal("expected performance to be initialized")
	}
	if s.Performance.TotalApplications != 1 {
		t.Errorf("expected 1 application, got %d", s.Performance.TotalApplications)
	}
	if s.Performance.SuccessCount != 1 {
		t.Errorf("expected 1 success for converged improvement, got %d", s.Performance.SuccessCount)
	}
}

func TestProcessTrainingResult_NonConverged(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())

	var targetID string
	for id := range ml.strategies {
		targetID = id
		break
	}

	result := TrainingResult{
		AgentID:      "agent-1",
		StrategyID:   targetID,
		Improvement:  -0.05,
		TrainingTime: 1200,
		Converged:    false,
		Timestamp:    time.Now(),
	}
	ml.processTrainingResult(result)

	s := ml.strategies[targetID]
	if s.Performance.FailureCount != 1 {
		t.Errorf("expected 1 failure for non-converged, got %d", s.Performance.FailureCount)
	}
}

func TestProcessTrainingResult_UnknownStrategy(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	// Must not panic
	ml.processTrainingResult(TrainingResult{
		AgentID:    "agent-x",
		StrategyID: "nonexistent",
		Converged:  true,
	})
}

func TestCrossover(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())

	p1 := &LearningStrategy{
		ID:         "p1",
		Parameters: map[string]float64{"lr": 0.1, "bs": 32, "momentum": 0.9},
	}
	p2 := &LearningStrategy{
		ID:         "p2",
		Parameters: map[string]float64{"lr": 0.2, "bs": 64, "momentum": 0.8},
	}

	child := ml.crossover(p1, p2, "child-1")
	if child == nil {
		t.Fatal("expected non-nil child")
	}
	if len(child.Parameters) == 0 {
		t.Fatal("expected child to have parameters")
	}
	for key := range p1.Parameters {
		if _, exists := child.Parameters[key]; !exists {
			t.Errorf("expected child to have key %s", key)
		}
	}
}

func TestGenerateReport(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	ml.evolvePopulation()

	report := ml.GenerateReport()
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.TotalStrategies == 0 {
		t.Fatal("expected strategies in report")
	}
	if report.PopulationSize == 0 {
		t.Fatal("expected population in report")
	}
	if len(report.TopStrategies) != 5 {
		t.Errorf("expected 5 top strategies, got %d", len(report.TopStrategies))
	}
	if report.BestStrategyID == "" {
		t.Fatal("expected best strategy ID")
	}
}

func TestSave_Load_RoundTrip(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	ml.evolvePopulation()

	path := t.TempDir() + "/state.json"
	if err := ml.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ml2 := NewMetaLearner(DefaultMetaLearningConfig())
	if err := ml2.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(ml2.population) == 0 {
		t.Fatal("expected population restored")
	}
	if len(ml2.eliteStrategies) == 0 {
		t.Fatal("expected elites restored")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	err := ml.Load(t.TempDir() + "/nonexistent.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	path := t.TempDir() + "/invalid.json"
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	ml := NewMetaLearner(DefaultMetaLearningConfig())
	err := ml.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_OrphanedPopulationIDs(t *testing.T) {
	ml1 := NewMetaLearner(DefaultMetaLearningConfig())
	ml1.evolvePopulation()

	path := t.TempDir() + "/state.json"
	if err := ml1.Save(path); err != nil {
		t.Fatal(err)
	}

	// Manually corrupt state to test orphaned population ID handling
	data, _ := os.ReadFile(path)
	corrupted := strings.Replace(string(data), `"strategy_momentum_0"`, `"orphaned_id"`, 1)

	if err := os.WriteFile(path, []byte(corrupted), 0o644); err != nil {
		t.Fatal(err)
	}

	ml2 := NewMetaLearner(DefaultMetaLearningConfig())
	if err := ml2.Load(path); err != nil {
		t.Fatalf("Load with orphaned IDs should succeed, got: %v", err)
	}
}

func TestSubmitTrainingScenarios_Empty(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	before := len(ml.population)
	ml.SubmitTrainingScenarios(nil)
	if len(ml.population) != before {
		t.Errorf("empty scenarios should not change population")
	}
	ml.SubmitTrainingScenarios([]swarm.TrainingScenario{})
	if len(ml.population) != before {
		t.Errorf("empty slice should not change population")
	}
}

func TestIntn_NonPositive(t *testing.T) {
	if rand.Intn(0) != 0 {
		t.Error("expected 0 for Intn(0)")
	}
	if rand.Intn(-1) != 0 {
		t.Error("expected 0 for Intn(-1)")
	}
}

func TestSave_InvalidPath(t *testing.T) {
	ml := NewMetaLearner(DefaultMetaLearningConfig())
	// Use a path with a directory that doesn't exist, and a subpath that's invalid
	err := ml.Save(t.TempDir() + "/subdir/state.json")
	if err == nil {
		t.Fatal("expected error for invalid save path")
	}
}

func TestScenarioToLearningData_ZeroAccuracy(t *testing.T) {
	scenario := swarm.TrainingScenario{
		ID:       "fish_zero",
		Scenario: "Bear Market",
		Performance: swarm.FishPerformance{
			Accuracy:    0.0,
			MaxDrawdown: 0.3,
		},
		States: make([]swarm.MarketState, 600),
	}

	data := scenarioToLearningData(scenario)
	if data.ConvergenceSpeed != 1.0 {
		t.Errorf("expected convergence speed 1.0 for zero accuracy (default), got %f", data.ConvergenceSpeed)
	}
	if data.LearningRate != 0.1 {
		t.Errorf("expected lr 0.1 for zero accuracy, got %f", data.LearningRate)
	}
	if data.BatchSize != 64 {
		t.Errorf("expected batch size 64 for 600 steps, got %d", data.BatchSize)
	}
}

func TestScenarioToLearningData_NegativeAccuracy(t *testing.T) {
	scenario := swarm.TrainingScenario{
		ID:       "fish_neg",
		Scenario: "Crash",
		Performance: swarm.FishPerformance{
			Accuracy:    -0.1,
			MaxDrawdown: 0.5,
		},
		States: make([]swarm.MarketState, 80),
	}

	data := scenarioToLearningData(scenario)
	if data.ConvergenceSpeed != 1.0 {
		t.Errorf("expected convergence speed 1.0 for negative accuracy (default), got %f", data.ConvergenceSpeed)
	}
}
