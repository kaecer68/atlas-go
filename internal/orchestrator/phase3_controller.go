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

// Phase3Controller coordinates PRISM, Swarm, Spawning, and Reflexivity.
// The Swarm field holds a pass-through state container (simulation engine
// was demoted in PR #963). No background goroutines or tickers are managed
// here — the caller (e.g., BackgroundTaskManager in main.go) owns the
// scheduling.
type Phase3Controller struct {
	registry        *domain.AgentRegistry
	prismManager    *prism.PRISMManager
	swarm           *swarm.SwarmState
	spawningManager *spawning.SpawningManager
	reflexEngine    *reflexivity.ReflexivityEngine
	ledger          ledger.OutcomeStore
	advRunner       *AdversarialScenarioRunner
	lastAdvResult   *adversarial.StressTestResult
	metaLearner     *metalearning.MetaLearner
	metaLearnPath   string

	mu               sync.RWMutex
	prismWeightCache map[string]float64 // agentID -> weight multiplier
}

// NewPhase3Controller creates a controller for Phase 3 advanced systems.
func NewPhase3Controller(
	registry *domain.AgentRegistry,
	prismMgr *prism.PRISMManager,
	sw *swarm.SwarmState,
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

// SetMetaLearner attaches a MetaLearner for strategy optimization.
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

// GetSwarmConsensus returns empty — swarm simulation engine demoted in PR #963.
func (c *Phase3Controller) GetSwarmConsensus() (domain.SwarmSimulationResult, bool) {
	return domain.SwarmSimulationResult{}, false
}

// GetRecommendedStrategies returns the MetaLearner's top learning strategies,
// which can inform the evolution pipeline's mutation selection.
func (c *Phase3Controller) GetRecommendedStrategies() []*metalearning.LearningStrategy {
	if c.metaLearner == nil {
		return nil
	}
	return c.metaLearner.GetTopStrategies(5)
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
