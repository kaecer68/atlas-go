package orchestrator

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// Phase3Controller coordinates PRISM, Swarm, Spawning, and Reflexivity
// using a swarm-style parallel optimization approach.
type Phase3Controller struct {
	registry        *domain.AgentRegistry
	prismManager    *prism.PRISMManager
	swarm           *swarm.MiroFishSwarm
	spawningManager *spawning.SpawningManager
	reflexEngine    *reflexivity.ReflexivityEngine
	ledger          *ledger.Store

	mu               sync.RWMutex
	swarmRunning     bool
	swarmStopCh      chan struct{}
	lastSwarmState   swarm.MarketState
	prismWeightCache map[string]float64 // agentID -> weight multiplier
}

// NewPhase3Controller creates a controller for Phase 3 advanced systems.
func NewPhase3Controller(
	registry *domain.AgentRegistry,
	prismMgr *prism.PRISMManager,
	sw *swarm.MiroFishSwarm,
	spawningMgr *spawning.SpawningManager,
	reflexEng *reflexivity.ReflexivityEngine,
	ledgerStore *ledger.Store,
) *Phase3Controller {
	return &Phase3Controller{
		registry:         registry,
		prismManager:     prismMgr,
		swarm:            sw,
		spawningManager:  spawningMgr,
		reflexEngine:     reflexEng,
		ledger:           ledgerStore,
		swarmStopCh:      make(chan struct{}),
		prismWeightCache: make(map[string]float64),
	}
}

// StartBackgroundSwarm initializes and continuously updates the swarm simulator.
func (c *Phase3Controller) StartBackgroundSwarm(baseState swarm.MarketState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.swarmRunning || c.swarm == nil {
		return
	}

	c.lastSwarmState = baseState
	c.swarm.InitializeScenarios(baseState)
	c.swarm.Start()
	c.swarmRunning = true

	go c.swarmUpdateLoop()
	log.Println("[Phase3Controller] Background swarm started")
}

// StopBackgroundSwarm halts the continuous swarm updates.
func (c *Phase3Controller) StopBackgroundSwarm() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.swarmRunning || c.swarm == nil {
		return
	}

	close(c.swarmStopCh)
	c.swarm.Stop()
	c.swarmRunning = false
	c.swarmStopCh = make(chan struct{})
	log.Println("[Phase3Controller] Background swarm stopped")
}

// UpdateSwarmState feeds the latest market state into the running swarm.
func (c *Phase3Controller) UpdateSwarmState(state swarm.MarketState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSwarmState = state
}

func (c *Phase3Controller) swarmUpdateLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.swarmStopCh:
			return
		case <-ticker.C:
			c.mu.RLock()
			state := c.lastSwarmState
			c.mu.RUnlock()

			if c.swarm != nil && len(state.Prices) > 0 {
				// Re-initialize with fresh state to keep consensus current
				c.swarm.Stop()
				c.syncReflexivityToSwarmUnsafe()
				c.swarm.InitializeScenarios(state)
				c.swarm.Start()
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
		if r.Result.SharpeRatio > agentSharpe[r.AgentID] {
			agentSharpe[r.AgentID] = r.Result.SharpeRatio
		}
	}

	// Update cache for telemetry
	c.mu.Lock()
	for k, v := range agentSharpe {
		c.prismWeightCache[k] = v
	}
	c.mu.Unlock()

	adjusted := make([]domain.Recommendation, len(recs))
	copy(adjusted, recs)
	for i := range adjusted {
		sharpe, ok := agentSharpe[adjusted[i].Agent]
		if !ok || sharpe <= 0 {
			continue
		}
		// Boost conviction up to +15 for top Sharpe performers; penalty up to -10 for poor performers
		boost := int(math.Round((sharpe - 0.5) * 20))
		boost = max(-10, min(15, boost))
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
	for k, v := range c.prismWeightCache {
		out[k] = v
	}
	return out
}

// AutoPromoteSpawnedAgents evaluates candidate agents using ledger scorecards
// and automatically accepts or rejects them based on performance.
func (c *Phase3Controller) AutoPromoteSpawnedAgents() {
	if c.spawningManager == nil || c.ledger == nil {
		return
	}

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
		if !ok || sc.Observations < 10 {
			continue
		}

		// Acceptance criteria
		sharpeLike := sc.SharpeLike
		hitRate := sc.HitRate

		if sharpeLike >= 0.5 && hitRate >= 0.45 {
			if err := c.spawningManager.AcceptAgent(agent.AgentID); err != nil {
				log.Printf("[Phase3Controller] Failed to accept %s: %v", agent.AgentID, err)
			}
		} else if sharpeLike < 0.0 || hitRate < 0.30 {
			if err := c.spawningManager.RejectAgent(agent.AgentID, fmt.Sprintf("poor performance (sharpe %.3f, hit %.2f%%)", sharpeLike, hitRate*100)); err != nil {
				log.Printf("[Phase3Controller] Failed to reject %s: %v", agent.AgentID, err)
			}
		}
	}
}

// SyncReflexivityToSwarmUnsafe reads active reflexivity loops and mutates swarm scenarios.
// Must be called under lock or when swarm is stopped.
func (c *Phase3Controller) syncReflexivityToSwarmUnsafe() {
	if c.reflexEngine == nil || c.swarm == nil {
		return
	}
	// This is a hook for future deep integration.
	// For now, the reflexivity engine inside sim.Engine already adjusts recommendations directly.
}

// GetSwarmConsensus returns the latest swarm consensus if background swarm is running.
func (c *Phase3Controller) GetSwarmConsensus() (swarm.SimulationResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.swarmRunning || c.swarm == nil {
		return swarm.SimulationResult{}, false
	}
	return c.swarm.GetLatestResult()
}

// RunParallelOptimization executes all four Phase 3 optimization tracks.
func (c *Phase3Controller) RunParallelOptimization(baseState swarm.MarketState, regime domain.Regime) {
	var wg sync.WaitGroup
	wg.Add(4)

	go func() {
		defer wg.Done()
		if !c.IsSwarmRunning() {
			c.StartBackgroundSwarm(baseState)
		} else {
			c.UpdateSwarmState(baseState)
		}
	}()

	go func() {
		defer wg.Done()
		_ = c.ApplyPRISMWeights(nil, regime)
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

	wg.Wait()
}

// IsSwarmRunning reports whether the background swarm is active.
func (c *Phase3Controller) IsSwarmRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.swarmRunning
}
