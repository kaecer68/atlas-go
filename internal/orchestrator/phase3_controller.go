package orchestrator

import (
	"fmt"
	"maps"
	"math"
	"sync"

	"github.com/kaecer68/atlas-go/internal/adversarial"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/metalearning"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// Phase3Controller coordinates PRISM, Swarm, Spawning, and Reflexivity
// using a swarm-style parallel optimization approach.
// No background goroutines or tickers are managed here — the caller
// (e.g., BackgroundTaskManager in main.go) owns the scheduling.
type Phase3Controller struct {
	registry        *domain.AgentRegistry
	prismManager    *prism.PRISMManager
	swarm           *swarm.MiroFishSwarm
	spawningManager *spawning.SpawningManager
	reflexEngine    *reflexivity.ReflexivityEngine
	ledger          ledger.OutcomeStore
	advRunner       *AdversarialScenarioRunner
	lastAdvResult   *adversarial.StressTestResult
	trainingStore   *swarm.TrainingStore
	snapshotPath    string
	metaLearner     *metalearning.MetaLearner
	metaLearnPath   string

	mu               sync.RWMutex
	prismWeightCache map[string]float64 // agentID -> weight multiplier
}

// NewPhase3Controller creates a controller for Phase 3 advanced systems.
func NewPhase3Controller(
	registry *domain.AgentRegistry,
	prismMgr *prism.PRISMManager,
	sw *swarm.MiroFishSwarm,
	spawningMgr *spawning.SpawningManager,
	reflexEng *reflexivity.ReflexivityEngine,
	ledgerStore ledger.OutcomeStore,
) *Phase3Controller {
	return &Phase3Controller{
		registry:         registry,
		prismManager:     prismMgr,
		swarm:            sw,
		spawningManager:  spawningMgr,
		reflexEngine:     reflexEng,
		ledger:           ledgerStore,
		prismWeightCache: make(map[string]float64),
	}
}

// WithAdversarialRunner attaches a real adversarial scenario runner.
func (c *Phase3Controller) WithAdversarialRunner(r *AdversarialScenarioRunner) *Phase3Controller {
	c.advRunner = r
	return c
}

// SetTrainingStore attaches a training data store for swarm output persistence.
func (c *Phase3Controller) SetTrainingStore(ts *swarm.TrainingStore) {
	c.trainingStore = ts
}

// SetSnapshotPath sets the file path where swarm snapshots are persisted.
func (c *Phase3Controller) SetSnapshotPath(path string) {
	c.snapshotPath = path
}

// SetMetaLearner attaches a MetaLearner for swarm-driven strategy optimization.
// metaLearnPath is the file path for persisting/restoring MetaLearner state.
func (c *Phase3Controller) SetMetaLearner(ml *metalearning.MetaLearner, persistPath string) {
	c.metaLearner = ml
	c.metaLearnPath = persistPath

	// Restore previous state if available
	if persistPath != "" {
		if err := ml.Load(persistPath); err != nil {
			logging.Debug("phase3_controller", "metalearner_load_skipped", "path", persistPath, "err", err)
		} else {
			logging.Info("phase3_controller", "metalearner_restored", "strategies", len(ml.Strategies()), "path", persistPath)
		}
	}
}

// RunSwarmCycle runs one complete swarm simulation cycle synchronously:
//  1. Apply reflexivity mutations to scenarios
//  2. Initialize and run swarm simulation
//  3. Export training data for downstream consumption
//
// No goroutines or tickers. The caller is responsible for scheduling.
func (c *Phase3Controller) RunSwarmCycle(baseState swarm.MarketState) {
	if c.swarm == nil {
		return
	}

	c.mu.Lock()
	c.syncReflexivityToSwarmUnsafe()
	c.mu.Unlock()

	c.swarm.InitializeScenarios(baseState)
	c.swarm.Start()
	c.swarm.EvolveGeneration()

	// Export training data for downstream consumption
	if c.trainingStore != nil {
		trainingData := c.swarm.ExportTrainingData()
		if err := c.trainingStore.Store(trainingData); err != nil {
			logging.Warn("phase3_controller", "training_store_failed", "err", err)
		} else {
			logging.Info("phase3_controller", "training_data_stored", "scenarios", len(trainingData))
		}
	}

	logging.Info("phase3_controller", "swarm_cycle_completed")

	// Save snapshot for dashboard API consumption
	if c.snapshotPath != "" {
		if err := c.swarm.SaveSnapshot(c.snapshotPath); err != nil {
			logging.Warn("phase3_controller", "snapshot_save_failed", "err", err)
		}
	}

	// Feed training data into MetaLearner for strategy evolution
	if c.metaLearner != nil {
		trainingData := c.swarm.ExportTrainingData()
		if len(trainingData) > 0 {
			c.metaLearner.SubmitTrainingScenarios(trainingData)
			logging.Info("phase3_controller", "metalearner_updated", "scenarios", len(trainingData))

			// Persist MetaLearner state
			if c.metaLearnPath != "" {
				if err := c.metaLearner.Save(c.metaLearnPath); err != nil {
					logging.Warn("phase3_controller", "metalearner_save_failed", "err", err)
				}
			}
		}
	}
}

// ApplyPRISMWeights adjusts recommendations based on regime-specific training results.
func (c *Phase3Controller) ApplyPRISMWeights(recs []domain.Recommendation, regime domain.Regime) []domain.Recommendation {
	if c.prismManager == nil || len(recs) == 0 {
		return recs
	}

	var pr prism.RegimeType
	switch regime {
	case domain.RegimeRiskOn:
		pr = prism.RegimeRiskOn
	case domain.RegimeRiskOff:
		pr = prism.RegimeRiskOff
	default:
		pr = prism.RegimeTransition
	}

	results := c.prismManager.GetCompletedResults()
	agentSharpe := make(map[string]float64)
	for _, r := range results {
		if r.Regime != pr {
			continue
		}
		if r.Result.Synthetic {
			continue
		}
		if r.Result.Error != "" {
			continue
		}
		if r.Result.SignalsCount == 0 {
			continue
		}
		if r.Result.SharpeRatio > agentSharpe[r.AgentID] {
			agentSharpe[r.AgentID] = r.Result.SharpeRatio
		}
	}

	c.mu.Lock()
	maps.Copy(c.prismWeightCache, agentSharpe)
	c.mu.Unlock()

	params := config.GetParametersConfig().Orchestrator
	adjusted := make([]domain.Recommendation, len(recs))
	copy(adjusted, recs)
	for i := range adjusted {
		sharpe, ok := agentSharpe[adjusted[i].Agent]
		if !ok || sharpe <= 0 {
			continue
		}
		boost := int(math.Round((sharpe - 0.5) * params.PRISMBoostMultiplier.Value))
		boost = max(params.PRISMBoostMin.Value, min(params.PRISMBoostMax.Value, boost))
		adjusted[i].Conviction = max(0, min(100, adjusted[i].Conviction+boost))
		if boost != 0 {
			adjusted[i].Reason = fmt.Sprintf("%s [PRISM:%.2f]", adjusted[i].Reason, sharpe)
		}
	}
	return adjusted
}

// GetPRISMWeightCache returns a copy of the latest PRISM-derived weights.
func (c *Phase3Controller) GetPRISMWeightCache() map[string]float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]float64, len(c.prismWeightCache))
	maps.Copy(out, c.prismWeightCache)
	return out
}

// AutoPromoteSpawnedAgents evaluates candidate agents using ledger scorecards
// and automatically accepts or rejects them based on performance.
func (c *Phase3Controller) AutoPromoteSpawnedAgents() {
	if c.spawningManager == nil || c.ledger == nil {
		return
	}

	params := config.GetParametersConfig().Orchestrator

	scorecards, _, err := c.ledger.LoadAllSessionScorecards()
	if err != nil {
		return
	}

	scorecardByAgent := make(map[string]*domain.Scorecard, len(scorecards))
	for i := range scorecards {
		scorecardByAgent[scorecards[i].AgentID] = &scorecards[i]
	}

	agents := c.spawningManager.GetSpawnedAgents()
	for _, agent := range agents {
		if agent.Status != spawning.SpawnStatusCandidate && agent.Status != spawning.SpawnStatusValidating {
			continue
		}

		sc, ok := scorecardByAgent[agent.AgentID]
		if !ok || sc.Observations < params.PromotionMinObservations.Value {
			continue
		}

		sharpeLike := sc.SharpeLike
		hitRate := sc.HitRate

		if sharpeLike >= params.PromotionSharpeThreshold.Value && hitRate >= params.PromotionHitRateThreshold.Value {
			if err := c.spawningManager.AcceptAgent(agent.AgentID); err != nil {
				logging.Error("phase3_controller", "accept_failed", logging.AgentID(agent.AgentID), logging.Err(err))
			}
		} else if sharpeLike < params.RejectionSharpeThreshold.Value || hitRate < params.RejectionHitRateThreshold.Value {
			if err := c.spawningManager.RejectAgent(agent.AgentID, fmt.Sprintf("poor performance (sharpe %.3f, hit %.2f%%)", sharpeLike, hitRate*100)); err != nil {
				logging.Error("phase3_controller", "reject_failed", logging.AgentID(agent.AgentID), logging.Err(err))
			}
		}
	}
}

// syncReflexivityToSwarmUnsafe reads active reflexivity loops and mutates swarm scenarios.
// Must be called under lock or when swarm is stopped.
func (c *Phase3Controller) syncReflexivityToSwarmUnsafe() {
	if c.reflexEngine == nil || c.swarm == nil {
		return
	}

	loops := c.reflexEngine.GetActiveLoops()
	if len(loops) == 0 {
		// No active loops: drift toward complacency (lower vol on high_vol, raise low_vol calm)
		c.swarm.UpdateScenario("high_vol", -0.03, 0)
		c.swarm.UpdateScenario("low_vol", -0.01, 0)
		return
	}

	bullStrength := 0.0
	bearStrength := 0.0
	meanRevStrength := 0.0
	crashStrength := 0.0
	bubbleStrength := 0.0

	for _, loop := range loops {
		switch loop.Direction {
		case reflexivity.PositiveFeedback:
			if loop.Bias != nil && loop.Bias.Magnitude > 0 {
				bubbleStrength += loop.Strength
				bullStrength += loop.Strength
			} else {
				crashStrength += loop.Strength
				bearStrength += loop.Strength
			}
		case reflexivity.NegativeFeedback:
			meanRevStrength += loop.Strength
		}
	}

	if bubbleStrength > 0 {
		c.swarm.UpdateScenario("bull_trend", bubbleStrength*0.05, bubbleStrength*0.001)
	}
	if crashStrength > 0 {
		c.swarm.UpdateScenario("bear_trend", crashStrength*0.08, -crashStrength*0.002)
	}
	if meanRevStrength > 0 {
		c.swarm.UpdateScenario("transition", meanRevStrength*0.04, 0)
	}
	if bullStrength+bearStrength > 0.7 {
		c.swarm.UpdateScenario("high_vol", 0.05, 0)
	}
	if bubbleStrength > 0.8 || crashStrength > 0.8 {
		c.swarm.UpdateScenario("low_vol", 0.02, 0)
	}
}

// GetSwarmConsensus returns the latest swarm consensus.
func (c *Phase3Controller) GetSwarmConsensus() (swarm.SimulationResult, bool) {
	if c.swarm == nil {
		return swarm.SimulationResult{}, false
	}
	return c.swarm.GetLatestResult()
}

// GetRecommendedStrategies returns the MetaLearner's top learning strategies,
// which can inform the evolution pipeline's mutation selection.
func (c *Phase3Controller) GetRecommendedStrategies() []*metalearning.LearningStrategy {
	if c.metaLearner == nil {
		return nil
	}
	return c.metaLearner.GetTopStrategies(5)
}

// RunParallelOptimization executes all five Phase 3 optimization tracks.
func (c *Phase3Controller) RunParallelOptimization(baseState swarm.MarketState, regime domain.Regime) {
	var wg sync.WaitGroup
	wg.Add(5)

	go func() {
		defer wg.Done()
		c.RunSwarmCycle(baseState)
	}()

	go func() {
		defer wg.Done()
		if err := c.ApplyPRISMWeights(nil, regime); err != nil {
			logging.Warn("Phase3Controller", "ApplyPRISMWeights failed", "err", err)
		}
	}()

	go func() {
		defer wg.Done()
		c.AutoPromoteSpawnedAgents()
	}()

	go func() {
		defer wg.Done()
		c.mu.Lock()
		c.syncReflexivityToSwarmUnsafe()
		c.mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		c.runAdversarialStressTests()
	}()

	wg.Wait()
	if err := c.SaveMetrics(""); err != nil {
		logging.Warn("Phase3Controller", "SaveMetrics failed", "err", err)
	}
}

// runAdversarialStressTests finds the weakest agent and runs real stress scenarios.
func (c *Phase3Controller) runAdversarialStressTests() {
	if c.advRunner == nil || c.registry == nil {
		return
	}
	weakestAgent := ""
	worstSharpe := 999.0
	c.mu.RLock()
	for agentID, sharpe := range c.prismWeightCache {
		if sharpe < worstSharpe {
			worstSharpe = sharpe
			weakestAgent = agentID
		}
	}
	c.mu.RUnlock()
	if weakestAgent == "" {
		return
	}
	var target domain.AgentSpec
	for _, a := range c.registry.Agents {
		if a.ID == weakestAgent {
			target = a
			break
		}
	}
	if target.ID == "" {
		return
	}
	result := c.advRunner.RunStressTest(target.ID, target)
	c.mu.Lock()
	c.lastAdvResult = result
	c.mu.Unlock()
	if !result.Passed {
		logging.Warn("phase3_controller", "adversarial_test_failed", logging.AgentID(target.ID), "score", result.OverallScore)
	}
}

// GetLastAdversarialResult returns the most recent stress test result.
func (c *Phase3Controller) GetLastAdversarialResult() *adversarial.StressTestResult {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastAdvResult
}
