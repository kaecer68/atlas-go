// Package metalearning implements learning-to-learn optimization for agent strategies
// Based on MiroFish swarm results and training data to optimize learning strategies
//
// Deprecated: This package was built and tested but never integrated into the
// production pipeline. The genetic algorithm and strategy optimization features
// require swarm data producer and training pipeline infrastructure that was
// never built. The current evolution system (internal/evolution/) uses a simpler
// prompt-mutation approach. See DEPRECATED.md for re-enablement conditions
// (Phase 5 of the system health remediation plan).
package metalearning

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
	"time"
)

// LearningStrategy represents a configurable learning approach
type LearningStrategy struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Type        StrategyType         `json:"type"`
	Parameters  map[string]float64   `json:"parameters"`
	Performance *StrategyPerformance `json:"performance,omitempty"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// StrategyType defines different learning strategy categories
type StrategyType string

const (
	StrategyMomentum     StrategyType = "momentum"     // Momentum-based learning
	StrategyAdaptive     StrategyType = "adaptive"     // Adaptive learning rate
	StrategyCurriculum   StrategyType = "curriculum"   // Curriculum learning
	StrategyEnsemble     StrategyType = "ensemble"     // Ensemble of strategies
	StrategyEvolutionary StrategyType = "evolutionary" // Evolutionary optimization
)

// StrategyPerformance tracks how well a strategy performs
type StrategyPerformance struct {
	StrategyID        string    `json:"strategy_id"`
	TotalApplications int       `json:"total_applications"`
	SuccessCount      int       `json:"success_count"`
	FailureCount      int       `json:"failure_count"`
	AvgImprovement    float64   `json:"avg_improvement"`
	BestImprovement   float64   `json:"best_improvement"`
	WorstImprovement  float64   `json:"worst_improvement"`
	AvgTrainingTime   float64   `json:"avg_training_time"`
	ConvergenceRate   float64   `json:"convergence_rate"`
	StabilityScore    float64   `json:"stability_score"`
	LastEvaluated     time.Time `json:"last_evaluated"`
}

// MetaLearningConfig holds configuration for the meta-learning system
type MetaLearningConfig struct {
	PopulationSize     int           `json:"population_size"`     // Number of strategies to maintain
	EliteRatio         float64       `json:"elite_ratio"`         // Top % to keep as elite
	MutationRate       float64       `json:"mutation_rate"`       // Probability of mutation
	CrossoverRate      float64       `json:"crossover_rate"`      // Probability of crossover
	EvaluationWindow   time.Duration `json:"evaluation_window"`   // Time to evaluate strategies
	AdaptationInterval time.Duration `json:"adaptation_interval"` // How often to adapt
	MinImprovement     float64       `json:"min_improvement"`     // Minimum improvement to count as success
}

// DefaultMetaLearningConfig returns sensible defaults
func DefaultMetaLearningConfig() *MetaLearningConfig {
	return &MetaLearningConfig{
		PopulationSize:     20,
		EliteRatio:         0.2,
		MutationRate:       0.15,
		CrossoverRate:      0.3,
		EvaluationWindow:   7 * 24 * time.Hour,
		AdaptationInterval: 24 * time.Hour,
		MinImprovement:     0.05,
	}
}

// MetaLearner is the central meta-learning engine
type MetaLearner struct {
	strategies      map[string]*LearningStrategy
	population      []*LearningStrategy
	eliteStrategies []*LearningStrategy
	config          *MetaLearningConfig
	swarmData       chan SwarmLearningData
	trainingResults chan TrainingResult
	stopChan        chan struct{}
	mu              sync.RWMutex
	wg              sync.WaitGroup
}

// SwarmLearningData represents feedback from MiroFish swarm
type SwarmLearningData struct {
	FishID           string             `json:"fish_id"`
	Scenario         string             `json:"scenario"`
	LearningRate     float64            `json:"learning_rate"`
	BatchSize        int                `json:"batch_size"`
	Epochs           int                `json:"epochs"`
	FinalAccuracy    float64            `json:"final_accuracy"`
	ConvergenceSpeed float64            `json:"convergence_speed"`
	Stability        float64            `json:"stability"`
	Timestamp        time.Time          `json:"timestamp"`
	StrategyParams   map[string]float64 `json:"strategy_params"`
}

// TrainingResult captures outcome of applying a learning strategy
type TrainingResult struct {
	AgentID      string    `json:"agent_id"`
	StrategyID   string    `json:"strategy_id"`
	InitialScore float64   `json:"initial_score"`
	FinalScore   float64   `json:"final_score"`
	Improvement  float64   `json:"improvement"`
	TrainingTime float64   `json:"training_time"`
	Converged    bool      `json:"converged"`
	Timestamp    time.Time `json:"timestamp"`
}

// NewMetaLearner creates a new meta-learning engine
func NewMetaLearner(config *MetaLearningConfig) *MetaLearner {
	if config == nil {
		config = DefaultMetaLearningConfig()
	}

	ml := &MetaLearner{
		strategies:      make(map[string]*LearningStrategy),
		population:      make([]*LearningStrategy, 0, config.PopulationSize),
		eliteStrategies: make([]*LearningStrategy, 0),
		config:          config,
		swarmData:       make(chan SwarmLearningData, 100),
		trainingResults: make(chan TrainingResult, 100),
		stopChan:        make(chan struct{}),
	}

	// Initialize with diverse strategies
	ml.initializePopulation()

	return ml
}

// initializePopulation creates initial diverse strategy population
func (ml *MetaLearner) initializePopulation() {
	baseStrategies := []struct {
		name   string
		typ    StrategyType
		params map[string]float64
	}{
		{
			name: "Conservative Momentum",
			typ:  StrategyMomentum,
			params: map[string]float64{
				"learning_rate": 0.001,
				"momentum":      0.9,
				"batch_size":    32,
				"warmup_epochs": 5,
			},
		},
		{
			name: "Aggressive Adaptive",
			typ:  StrategyAdaptive,
			params: map[string]float64{
				"base_lr":      0.01,
				"max_lr":       0.1,
				"decay_factor": 0.95,
				"patience":     3,
			},
		},
		{
			name: "Progressive Curriculum",
			typ:  StrategyCurriculum,
			params: map[string]float64{
				"difficulty_levels": 5,
				"mastery_threshold": 0.85,
				"stage_epochs":      10,
			},
		},
		{
			name: "Diverse Ensemble",
			typ:  StrategyEnsemble,
			params: map[string]float64{
				"ensemble_size":       7,
				"diversity_weight":    0.3,
				"agreement_threshold": 0.6,
			},
		},
		{
			name: "Evolutionary Search",
			typ:  StrategyEvolutionary,
			params: map[string]float64{
				"population_size":    50,
				"mutation_rate":      0.1,
				"selection_pressure": 2.0,
			},
		},
	}

	// Create variations of base strategies
	for i, base := range baseStrategies {
		// Original
		strategy := &LearningStrategy{
			ID:         fmt.Sprintf("strategy_%s_%d", base.typ, i),
			Name:       base.name,
			Type:       base.typ,
			Parameters: base.params,
			Performance: &StrategyPerformance{
				StrategyID: base.name,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		ml.strategies[strategy.ID] = strategy
		ml.population = append(ml.population, strategy)

		// Create mutated variations
		for j := range 3 {
			mutated := ml.mutateStrategy(strategy, fmt.Sprintf("%s_v%d", strategy.ID, j+1))
			ml.strategies[mutated.ID] = mutated
			ml.population = append(ml.population, mutated)
		}
	}
}

// mutateStrategy creates a variation of a strategy with mutated parameters
func (ml *MetaLearner) mutateStrategy(parent *LearningStrategy, newID string) *LearningStrategy {
	mutated := &LearningStrategy{
		ID:         newID,
		Name:       parent.Name + " (Mutated)",
		Type:       parent.Type,
		Parameters: make(map[string]float64),
		Performance: &StrategyPerformance{
			StrategyID: newID,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Copy and mutate parameters
	for key, val := range parent.Parameters {
		mutated.Parameters[key] = val

		// Apply mutation with probability
		if math.Abs(ml.config.MutationRate) > 1e-9 {
			mutation := (rand.Float64() - 0.5) * 2 * ml.config.MutationRate * val
			mutated.Parameters[key] = val + mutation

			// Ensure positive for certain parameters
			if mutated.Parameters[key] < 0 && (key == "learning_rate" || key == "batch_size") {
				mutated.Parameters[key] = math.Abs(mutated.Parameters[key])
			}
		}
	}

	return mutated
}

// crossover creates offspring from two parent strategies
func (ml *MetaLearner) crossover(parent1, parent2 *LearningStrategy, newID string) *LearningStrategy {
	child := &LearningStrategy{
		ID:         newID,
		Name:       fmt.Sprintf("Crossover_%s_%s", parent1.ID, parent2.ID),
		Type:       parent1.Type,
		Parameters: make(map[string]float64),
		Performance: &StrategyPerformance{
			StrategyID: newID,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Random crossover of parameters
	for key, val1 := range parent1.Parameters {
		if val2, exists := parent2.Parameters[key]; exists {
			if rand.Float64() < 0.5 {
				child.Parameters[key] = val1
			} else {
				child.Parameters[key] = val2
			}

			// Optional: blend values
			if rand.Float64() < 0.3 {
				alpha := rand.Float64()
				child.Parameters[key] = alpha*val1 + (1-alpha)*val2
			}
		} else {
			child.Parameters[key] = val1
		}
	}

	return child
}

// Start begins the meta-learning adaptation loop
func (ml *MetaLearner) Start() {
	ml.wg.Add(1)
	go ml.adaptationLoop()

	ml.wg.Add(1)
	go ml.swarmDataProcessor()

	ml.wg.Add(1)
	go ml.trainingResultProcessor()
}

// Stop gracefully shuts down the meta-learner
func (ml *MetaLearner) Stop() {
	close(ml.stopChan)
	ml.wg.Wait()
}

// adaptationLoop periodically evolves the strategy population
func (ml *MetaLearner) adaptationLoop() {
	defer ml.wg.Done()

	ticker := time.NewTicker(ml.config.AdaptationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ml.evolvePopulation()
		case <-ml.stopChan:
			return
		}
	}
}

// evolvePopulation performs one generation of evolution
func (ml *MetaLearner) evolvePopulation() {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	// 1. Evaluate and rank strategies
	ml.evaluateStrategies()

	// 2. Select elite
	eliteCount := max(int(float64(len(ml.population))*ml.config.EliteRatio), 2)

	sort.Slice(ml.population, func(i, j int) bool {
		scoreI := ml.calculateStrategyScore(ml.population[i])
		scoreJ := ml.calculateStrategyScore(ml.population[j])
		return scoreI > scoreJ
	})

	ml.eliteStrategies = make([]*LearningStrategy, eliteCount)
	copy(ml.eliteStrategies, ml.population[:eliteCount])

	// 3. Generate offspring
	newPopulation := make([]*LearningStrategy, 0, ml.config.PopulationSize)
	newPopulation = append(newPopulation, ml.eliteStrategies...)

	// Crossover among elite
	for i := 0; i < eliteCount && len(newPopulation) < ml.config.PopulationSize; i++ {
		for j := i + 1; j < eliteCount && len(newPopulation) < ml.config.PopulationSize; j++ {
			if rand.Float64() < ml.config.CrossoverRate {
				childID := fmt.Sprintf("cross_%d_%d_%d", i, j, time.Now().Unix())
				child := ml.crossover(ml.eliteStrategies[i], ml.eliteStrategies[j], childID)
				newPopulation = append(newPopulation, child)
				ml.strategies[child.ID] = child
			}
		}
	}

	// Mutation of elite
	for _, elite := range ml.eliteStrategies {
		if len(newPopulation) >= ml.config.PopulationSize {
			break
		}
		if rand.Float64() < ml.config.MutationRate {
			mutatedID := fmt.Sprintf("mut_%s_%d", elite.ID, time.Now().Unix())
			mutated := ml.mutateStrategy(elite, mutatedID)
			newPopulation = append(newPopulation, mutated)
			ml.strategies[mutated.ID] = mutated
		}
	}

	// Fill remaining with random mutations
	for len(newPopulation) < ml.config.PopulationSize {
		parent := ml.eliteStrategies[rand.Intn(len(ml.eliteStrategies))]
		mutatedID := fmt.Sprintf("fill_%s_%d", parent.ID, time.Now().Unix())
		mutated := ml.mutateStrategy(parent, mutatedID)
		newPopulation = append(newPopulation, mutated)
		ml.strategies[mutated.ID] = mutated
	}

	ml.population = newPopulation
}

// calculateStrategyScore computes fitness score for a strategy
func (ml *MetaLearner) calculateStrategyScore(s *LearningStrategy) float64 {
	if s.Performance == nil || s.Performance.TotalApplications == 0 {
		return 0.0
	}

	perf := s.Performance

	// Success rate
	successRate := float64(perf.SuccessCount) / float64(perf.TotalApplications)

	// Average improvement (normalized)
	improvementScore := perf.AvgImprovement / (1 + math.Abs(perf.AvgImprovement))

	// Convergence rate bonus
	convergenceBonus := perf.ConvergenceRate * 0.5

	// Stability bonus
	stabilityBonus := perf.StabilityScore * 0.3

	// Penalize for high training time (efficiency)
	efficiencyScore := 1.0 / (1.0 + perf.AvgTrainingTime/3600) // Normalize to hours

	// Combined score
	score := successRate*0.3 + improvementScore*0.3 + convergenceBonus*0.2 + stabilityBonus*0.1 + efficiencyScore*0.1

	return score
}

// evaluateStrategies updates all strategy evaluations
func (ml *MetaLearner) evaluateStrategies() {
	// In real implementation, would aggregate all historical data
	// For now, strategies are evaluated on-demand via their performance tracking
}

// swarmDataProcessor handles feedback from MiroFish swarm
func (ml *MetaLearner) swarmDataProcessor() {
	defer ml.wg.Done()

	for {
		select {
		case data := <-ml.swarmData:
			ml.processSwarmData(data)
		case <-ml.stopChan:
			return
		}
	}
}

// processSwarmData updates strategies based on swarm results
func (ml *MetaLearner) processSwarmData(data SwarmLearningData) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	// Find matching strategies by similarity to swarm parameters
	for _, strategy := range ml.population {
		similarity := ml.calculateParameterSimilarity(strategy.Parameters, data.StrategyParams)

		if similarity > 0.8 {
			// Update strategy performance based on swarm outcome
			if strategy.Performance == nil {
				strategy.Performance = &StrategyPerformance{StrategyID: strategy.ID}
			}

			perf := strategy.Performance
			perf.TotalApplications++

			// Success if accuracy is good and converged quickly
			improvement := data.FinalAccuracy - 0.5 // Assuming 0.5 baseline
			if data.FinalAccuracy > 0.7 && data.ConvergenceSpeed < 100 {
				perf.SuccessCount++
				perf.AvgImprovement = (perf.AvgImprovement*float64(perf.SuccessCount-1) + improvement) / float64(perf.SuccessCount)
			} else {
				perf.FailureCount++
			}

			perf.LastEvaluated = time.Now()
			strategy.UpdatedAt = time.Now()
		}
	}
}

// calculateParameterSimilarity computes similarity between two parameter sets
func (ml *MetaLearner) calculateParameterSimilarity(p1, p2 map[string]float64) float64 {
	if len(p1) == 0 || len(p2) == 0 {
		return 0.0
	}

	var totalDiff, count float64

	for key, val1 := range p1 {
		if val2, exists := p2[key]; exists {
			diff := math.Abs(val1 - val2)
			avg := (math.Abs(val1) + math.Abs(val2)) / 2
			if avg > 1e-9 {
				totalDiff += diff / avg
			}
			count++
		}
	}

	if count == 0 {
		return 0.0
	}

	avgDiff := totalDiff / count
	similarity := math.Max(0, 1-avgDiff)

	return similarity
}

// trainingResultProcessor handles training outcomes
func (ml *MetaLearner) trainingResultProcessor() {
	defer ml.wg.Done()

	for {
		select {
		case result := <-ml.trainingResults:
			ml.processTrainingResult(result)
		case <-ml.stopChan:
			return
		}
	}
}

// processTrainingResult updates strategy based on training outcome
func (ml *MetaLearner) processTrainingResult(result TrainingResult) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	strategy, exists := ml.strategies[result.StrategyID]
	if !exists {
		return
	}

	if strategy.Performance == nil {
		strategy.Performance = &StrategyPerformance{StrategyID: result.StrategyID}
	}

	perf := strategy.Performance
	perf.TotalApplications++

	if result.Improvement > ml.config.MinImprovement && result.Converged {
		perf.SuccessCount++
		if result.Improvement > perf.BestImprovement {
			perf.BestImprovement = result.Improvement
		}
	} else {
		perf.FailureCount++
		if result.Improvement < perf.WorstImprovement {
			perf.WorstImprovement = result.Improvement
		}
	}

	// Update rolling averages
	n := float64(perf.TotalApplications)
	perf.AvgImprovement = (perf.AvgImprovement*(n-1) + result.Improvement) / n
	perf.AvgTrainingTime = (perf.AvgTrainingTime*(n-1) + result.TrainingTime) / n

	if result.Converged {
		perf.ConvergenceRate = (perf.ConvergenceRate*(n-1) + 1) / n
	} else {
		perf.ConvergenceRate = (perf.ConvergenceRate*(n-1) + 0) / n
	}

	perf.LastEvaluated = time.Now()
	strategy.UpdatedAt = time.Now()
}

// GetBestStrategy returns the current best learning strategy
func (ml *MetaLearner) GetBestStrategy() *LearningStrategy {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	if len(ml.eliteStrategies) == 0 {
		return nil
	}

	return ml.eliteStrategies[0]
}

// GetTopStrategies returns the top N strategies
func (ml *MetaLearner) GetTopStrategies(n int) []*LearningStrategy {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	// Sort by score
	sorted := make([]*LearningStrategy, len(ml.population))
	copy(sorted, ml.population)

	sort.Slice(sorted, func(i, j int) bool {
		scoreI := ml.calculateStrategyScore(sorted[i])
		scoreJ := ml.calculateStrategyScore(sorted[j])
		return scoreI > scoreJ
	})

	if n > len(sorted) {
		n = len(sorted)
	}

	return sorted[:n]
}

// RecommendStrategyForAgent suggests best strategy for a given agent
func (ml *MetaLearner) RecommendStrategyForAgent(agentID string, agentType StrategyType) *LearningStrategy {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	// Filter by type and score
	var candidates []*LearningStrategy
	for _, s := range ml.population {
		if s.Type == agentType || agentType == "" {
			candidates = append(candidates, s)
		}
	}

	if len(candidates) == 0 {
		return ml.GetBestStrategy()
	}

	// Return highest scoring of matching type
	sort.Slice(candidates, func(i, j int) bool {
		scoreI := ml.calculateStrategyScore(candidates[i])
		scoreJ := ml.calculateStrategyScore(candidates[j])
		return scoreI > scoreJ
	})

	return candidates[0]
}

// SubmitSwarmData sends swarm results to meta-learner
func (ml *MetaLearner) SubmitSwarmData(data SwarmLearningData) {
	select {
	case ml.swarmData <- data:
	default:
		// Channel full, drop data
	}
}

// SubmitTrainingResult sends training outcome to meta-learner
func (ml *MetaLearner) SubmitTrainingResult(result TrainingResult) {
	select {
	case ml.trainingResults <- result:
	default:
		// Channel full, drop result
	}
}

// Save persists meta-learner state
func (ml *MetaLearner) Save(filepath string) error {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	state := struct {
		Strategies      map[string]*LearningStrategy `json:"strategies"`
		Population      []string                     `json:"population"`
		EliteStrategies []string                     `json:"elite_strategies"`
		Config          *MetaLearningConfig          `json:"config"`
		SavedAt         time.Time                    `json:"saved_at"`
	}{
		Strategies: ml.strategies,
		Config:     ml.config,
		SavedAt:    time.Now(),
	}

	// Save IDs for population references
	for _, s := range ml.population {
		state.Population = append(state.Population, s.ID)
	}
	for _, s := range ml.eliteStrategies {
		state.EliteStrategies = append(state.EliteStrategies, s.ID)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, data, 0644)
}

// Load restores meta-learner state
func (ml *MetaLearner) Load(filepath string) error {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	data, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	var state struct {
		Strategies      map[string]*LearningStrategy `json:"strategies"`
		Population      []string                     `json:"population"`
		EliteStrategies []string                     `json:"elite_strategies"`
		Config          *MetaLearningConfig          `json:"config"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	ml.strategies = state.Strategies
	ml.config = state.Config

	// Restore population references
	ml.population = make([]*LearningStrategy, 0, len(state.Population))
	for _, id := range state.Population {
		if s, exists := ml.strategies[id]; exists {
			ml.population = append(ml.population, s)
		}
	}

	// Restore elite references
	ml.eliteStrategies = make([]*LearningStrategy, 0, len(state.EliteStrategies))
	for _, id := range state.EliteStrategies {
		if s, exists := ml.strategies[id]; exists {
			ml.eliteStrategies = append(ml.eliteStrategies, s)
		}
	}

	return nil
}

// GenerateReport creates comprehensive meta-learning analysis
func (ml *MetaLearner) GenerateReport() *MetaLearningReport {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	report := &MetaLearningReport{
		GeneratedAt:     time.Now(),
		TotalStrategies: len(ml.strategies),
		PopulationSize:  len(ml.population),
		EliteCount:      len(ml.eliteStrategies),
		StrategyTypes:   make(map[StrategyType]int),
		TopStrategies:   make([]*StrategySummary, 0, 5),
	}

	// Count by type
	for _, s := range ml.strategies {
		report.StrategyTypes[s.Type]++
	}

	// Get top 5
	top := ml.GetTopStrategies(5)
	for i, s := range top {
		score := ml.calculateStrategyScore(s)
		report.TopStrategies = append(report.TopStrategies, &StrategySummary{
			Rank:        i + 1,
			ID:          s.ID,
			Name:        s.Name,
			Type:        s.Type,
			Score:       score,
			Performance: s.Performance,
		})
	}

	// Best strategy parameters
	if best := ml.GetBestStrategy(); best != nil {
		report.BestStrategyID = best.ID
		report.BestStrategyParams = best.Parameters
	}

	return report
}

// MetaLearningReport summarizes meta-learning system state
type MetaLearningReport struct {
	GeneratedAt        time.Time            `json:"generated_at"`
	TotalStrategies    int                  `json:"total_strategies"`
	PopulationSize     int                  `json:"population_size"`
	EliteCount         int                  `json:"elite_count"`
	StrategyTypes      map[StrategyType]int `json:"strategy_types"`
	TopStrategies      []*StrategySummary   `json:"top_strategies"`
	BestStrategyID     string               `json:"best_strategy_id"`
	BestStrategyParams map[string]float64   `json:"best_strategy_params"`
}

// StrategySummary provides quick overview of a strategy
type StrategySummary struct {
	Rank        int                  `json:"rank"`
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Type        StrategyType         `json:"type"`
	Score       float64              `json:"score"`
	Performance *StrategyPerformance `json:"performance"`
}

// Helper: pseudo-random for deterministic testing
var rand = &deterministicRand{}

type deterministicRand struct {
	seed uint64
}

func (r *deterministicRand) Float64() float64 {
	r.seed = r.seed*6364136223846793005 + 1
	return float64(r.seed>>33) / (1 << 31)
}

func (r *deterministicRand) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.Float64() * float64(n))
}

func init() {
	rand.seed = uint64(time.Now().UnixNano())
}
