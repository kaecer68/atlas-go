// Package swarm implements MiroFish Swarm - parallel simulated futures training
// Simulates multiple possible market futures to train agents on diverse scenarios
package swarm

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

// MiroFishSwarm manages parallel market simulations
type MiroFishSwarm struct {
	config    SwarmConfig
	fish      []*MiroFish
	scenarios []MarketScenario
	results   []SimulationResult
	mu        sync.RWMutex
	isRunning bool
	stopCh    chan struct{}
}

// MiroFish represents a single simulation unit
type MiroFish struct {
	ID           string
	Scenario     MarketScenario
	CurrentState MarketState
	History      []MarketState
	Predictions  []Prediction
	Performance  FishPerformance
	isAlive      bool
	spawnedAt    time.Time
}

// MarketScenario defines a possible market future
type MarketScenario struct {
	ID         string
	Name       string
	Regime     string
	Volatility float64
	Trend      float64
	Duration   time.Duration
	Events     []MarketEvent
}

// MarketEvent represents a discrete market-moving event
type MarketEvent struct {
	Time        time.Time
	Type        string
	Magnitude   float64
	Description string
}

// MarketState captures market conditions at a point in time
type MarketState struct {
	Timestamp   time.Time
	Prices      map[string]float64
	Volumes     map[string]float64
	Sentiment   float64
	Volatility  float64
	Correlation float64
}

// Prediction represents a fish's prediction at a moment
type Prediction struct {
	Timestamp  time.Time
	Symbol     string
	Direction  string // "up", "down", "neutral"
	Confidence float64
	Conviction int
	Rationale  string
}

// FishPerformance tracks how well a fish performs
type FishPerformance struct {
	CorrectPredictions int
	TotalPredictions   int
	Accuracy           float64
	SharpeRatio        float64
	MaxDrawdown        float64
	PnL                float64
}

// SimulationResult aggregates results from all fish
type SimulationResult struct {
	ScenarioID  string
	Timestamp   time.Time
	FishResults map[string]FishPerformance
	Consensus   map[string]ConsensusPrediction
	Anomalies   []Anomaly
	Confidence  float64
}

// ConsensusPrediction aggregates predictions across fish
type ConsensusPrediction struct {
	Symbol             string
	BullishCount       int
	BearishCount       int
	NeutralCount       int
	AverageConfidence  float64
	ConsensusDirection string
}

// Anomaly flags unusual patterns in simulation
type Anomaly struct {
	Type        string
	Description string
	Severity    float64
	Symbols     []string
}

// SwarmConfig configures swarm behavior
type SwarmConfig struct {
	FishCount            int
	SimulationHorizon    time.Duration
	TimeStep             time.Duration
	ConvergenceThreshold float64
	Parallelism          int
}

// DefaultSwarmConfig returns recommended defaults
func DefaultSwarmConfig() SwarmConfig {
	return SwarmConfig{
		FishCount:            100,
		SimulationHorizon:    30 * 24 * time.Hour, // 30 days
		TimeStep:             time.Hour,
		ConvergenceThreshold: 0.7,
		Parallelism:          10,
	}
}

// NewMiroFishSwarm creates a new swarm simulator
func NewMiroFishSwarm(config SwarmConfig) *MiroFishSwarm {
	return &MiroFishSwarm{
		config:    config,
		fish:      make([]*MiroFish, 0, config.FishCount),
		scenarios: make([]MarketScenario, 0),
		results:   make([]SimulationResult, 0),
		stopCh:    make(chan struct{}),
	}
}

// UpdateScenario adjusts parameters for a specific scenario by ID.
func (sw *MiroFishSwarm) UpdateScenario(id string, volatilityDelta, trendDelta float64) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	for i := range sw.scenarios {
		if sw.scenarios[i].ID == id {
			sw.scenarios[i].Volatility = math.Max(0.01, sw.scenarios[i].Volatility+volatilityDelta)
			sw.scenarios[i].Trend += trendDelta
			break
		}
	}
}

// InitializeScenarios sets up diverse market scenarios
func (sw *MiroFishSwarm) InitializeScenarios(baseState MarketState) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	// Generate diverse scenarios
	sw.scenarios = []MarketScenario{
		{
			ID:         "bull_trend",
			Name:       "Bull Market Trend",
			Regime:     "risk_on",
			Volatility: 0.15,
			Trend:      0.001, // Positive drift
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateBullEvents(),
		},
		{
			ID:         "bear_trend",
			Name:       "Bear Market Trend",
			Regime:     "risk_off",
			Volatility: 0.25,
			Trend:      -0.002, // Negative drift
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateBearEvents(),
		},
		{
			ID:         "high_vol",
			Name:       "High Volatility",
			Regime:     "crisis",
			Volatility: 0.40,
			Trend:      0.0,
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateVolatilityEvents(),
		},
		{
			ID:         "low_vol",
			Name:       "Low Volatility Range",
			Regime:     "complacent",
			Volatility: 0.08,
			Trend:      0.0001,
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateQuietEvents(),
		},
		{
			ID:         "transition",
			Name:       "Regime Transition",
			Regime:     "transition",
			Volatility: 0.20,
			Trend:      0.0,
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateTransitionEvents(),
		},
	}

	// Spawn fish for each scenario
	fishPerScenario := sw.config.FishCount / len(sw.scenarios)
	for _, scenario := range sw.scenarios {
		for i := 0; i < fishPerScenario; i++ {
			fish := &MiroFish{
				ID:           fmt.Sprintf("fish_%s_%d", scenario.ID, i),
				Scenario:     scenario,
				CurrentState: baseState,
				History:      make([]MarketState, 0),
				Predictions:  make([]Prediction, 0),
				isAlive:      true,
				spawnedAt:    time.Now(),
			}
			sw.fish = append(sw.fish, fish)
		}
	}

	log.Printf("[MiroFish] Initialized %d fish across %d scenarios", len(sw.fish), len(sw.scenarios))
}

// Start begins the swarm simulation
func (sw *MiroFishSwarm) Start() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if sw.isRunning {
		return
	}

	sw.isRunning = true
	sw.stopCh = make(chan struct{})

	// Run simulation in parallel batches
	batchSize := len(sw.fish) / sw.config.Parallelism
	for i := 0; i < sw.config.Parallelism; i++ {
		start := i * batchSize
		end := start + batchSize
		if i == sw.config.Parallelism-1 {
			end = len(sw.fish)
		}
		go sw.runBatch(sw.fish[start:end], sw.stopCh)
	}

	// Start result aggregator
	go sw.aggregateResults(sw.stopCh)

	log.Println("[MiroFish] Swarm simulation started")
}

// IsRunning reports whether the swarm simulation is active.
func (sw *MiroFishSwarm) IsRunning() bool {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.isRunning
}

// Stop halts the simulation
func (sw *MiroFishSwarm) Stop() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if !sw.isRunning {
		return
	}

	sw.isRunning = false
	close(sw.stopCh)

	log.Println("[MiroFish] Swarm simulation stopped")
}

// runBatch simulates a batch of fish
func (sw *MiroFishSwarm) runBatch(fish []*MiroFish, stopCh <-chan struct{}) {
	for _, f := range fish {
		select {
		case <-stopCh:
			return
		default:
		}

		sw.simulateFish(f)
	}
}

// simulateFish runs full simulation for a single fish
func (sw *MiroFishSwarm) simulateFish(fish *MiroFish) {
	steps := int(fish.Scenario.Duration / sw.config.TimeStep)

	for i := 0; i < steps && fish.isAlive; i++ {
		// Update market state based on scenario
		newState := sw.evolveState(fish.CurrentState, fish.Scenario, i)
		fish.History = append(fish.History, newState)
		fish.CurrentState = newState

		// Generate predictions
		pred := sw.generatePrediction(fish, newState)
		fish.Predictions = append(fish.Predictions, pred)

		// Update performance
		sw.updatePerformance(fish, pred, newState)
	}
}

// evolveState advances market state according to scenario dynamics
func (sw *MiroFishSwarm) evolveState(current MarketState, scenario MarketScenario, step int) MarketState {
	newState := MarketState{
		Timestamp: current.Timestamp.Add(sw.config.TimeStep),
		Prices:    make(map[string]float64),
		Volumes:   make(map[string]float64),
	}

	// Apply random walk with drift
	for symbol, price := range current.Prices {
		drift := scenario.Trend * float64(sw.config.TimeStep.Hours()) / 24.0
		shock := rand.NormFloat64() * scenario.Volatility / math.Sqrt(252.0) // Daily vol
		newPrice := price * (1 + drift + shock)
		newState.Prices[symbol] = newPrice
		newState.Volumes[symbol] = current.Volumes[symbol] * (1 + rand.Float64()*0.2)
	}

	// Apply scenario events
	for _, event := range scenario.Events {
		eventStep := int(event.Time.Sub(current.Timestamp) / sw.config.TimeStep)
		if eventStep == step {
			sw.applyEvent(&newState, event)
		}
	}

	// Update aggregated metrics
	newState.Volatility = scenario.Volatility * (1 + rand.Float64()*0.3)
	newState.Sentiment = calculateSentiment(newState, scenario)
	newState.Correlation = 0.5 + rand.Float64()*0.3

	return newState
}

// generatePrediction creates prediction based on fish's view of market
func (sw *MiroFishSwarm) generatePrediction(fish *MiroFish, state MarketState) Prediction {
	// Simple trend-following prediction for demonstration
	var targetSymbol string
	var targetPrice float64

	for sym, price := range state.Prices {
		targetSymbol = sym
		targetPrice = price
		break
	}

	// Determine direction based on recent trend
	direction := "neutral"
	if len(fish.History) > 5 {
		oldPrice := fish.History[len(fish.History)-5].Prices[targetSymbol]
		if targetPrice > oldPrice*1.02 {
			direction = "up"
		} else if targetPrice < oldPrice*0.98 {
			direction = "down"
		}
	}

	// Confidence based on volatility
	confidence := 1.0 - state.Volatility
	if confidence < 0.3 {
		confidence = 0.3
	}

	return Prediction{
		Timestamp:  state.Timestamp,
		Symbol:     targetSymbol,
		Direction:  direction,
		Confidence: confidence,
		Conviction: int(confidence * 100),
		Rationale:  fmt.Sprintf("Based on %s regime with %.1f%% vol", fish.Scenario.Regime, state.Volatility*100),
	}
}

// updatePerformance tracks fish prediction accuracy
func (sw *MiroFishSwarm) updatePerformance(fish *MiroFish, pred Prediction, state MarketState) {
	// Check if prediction was correct (would need future state in real implementation)
	// For now, increment counters
	fish.Performance.TotalPredictions++

	// Simulate accuracy based on confidence
	if rand.Float64() < pred.Confidence {
		fish.Performance.CorrectPredictions++
	}

	// Update accuracy
	if fish.Performance.TotalPredictions > 0 {
		fish.Performance.Accuracy = float64(fish.Performance.CorrectPredictions) / float64(fish.Performance.TotalPredictions)
	}
}

// applyEvent modifies state based on market event
func (sw *MiroFishSwarm) applyEvent(state *MarketState, event MarketEvent) {
	switch event.Type {
	case "flash_crash":
		for sym := range state.Prices {
			state.Prices[sym] *= (1 - event.Magnitude)
		}
		state.Volatility *= 2.0
		state.Sentiment = -0.8
	case "rally":
		for sym := range state.Prices {
			state.Prices[sym] *= (1 + event.Magnitude)
		}
		state.Sentiment = 0.8
	case "earnings_surprise":
		// Random symbol gets earnings surprise
		for sym := range state.Prices {
			state.Prices[sym] *= (1 + event.Magnitude*(rand.Float64()-0.5))
			break // Only affect one symbol
		}
	}
}

// aggregateResults combines predictions from all fish
func (sw *MiroFishSwarm) aggregateResults(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			sw.mu.Lock()
			result := sw.computeConsensus()
			sw.results = append(sw.results, result)
			sw.mu.Unlock()
		}
	}
}

// computeConsensus aggregates fish predictions into consensus view
func (sw *MiroFishSwarm) computeConsensus() SimulationResult {
	consensus := make(map[string]ConsensusPrediction)
	anomalies := make([]Anomaly, 0)

	// Aggregate predictions by symbol
	for _, fish := range sw.fish {
		if !fish.isAlive {
			continue
		}

		for _, pred := range fish.Predictions {
			cp := consensus[pred.Symbol]
			cp.Symbol = pred.Symbol

			switch pred.Direction {
			case "up":
				cp.BullishCount++
			case "down":
				cp.BearishCount++
			default:
				cp.NeutralCount++
			}

			cp.AverageConfidence += pred.Confidence
			consensus[pred.Symbol] = cp
		}
	}

	// Calculate consensus direction and average confidence
	for sym, cp := range consensus {
		total := cp.BullishCount + cp.BearishCount + cp.NeutralCount
		if total > 0 {
			cp.AverageConfidence /= float64(total)
		}

		// Determine consensus direction
		if cp.BullishCount > cp.BearishCount && cp.BullishCount > cp.NeutralCount {
			cp.ConsensusDirection = "bullish"
		} else if cp.BearishCount > cp.BullishCount && cp.BearishCount > cp.NeutralCount {
			cp.ConsensusDirection = "bearish"
		} else {
			cp.ConsensusDirection = "neutral"
		}

		consensus[sym] = cp

		// Check for anomalies (high disagreement)
		if cp.BullishCount > 0 && cp.BearishCount > 0 {
			disagreement := float64(min(cp.BullishCount, cp.BearishCount)) / float64(max(cp.BullishCount, cp.BearishCount))
			if disagreement > 0.3 {
				anomalies = append(anomalies, Anomaly{
					Type:        "high_disagreement",
					Description: fmt.Sprintf("Significant disagreement on %s: %d vs %d", sym, cp.BullishCount, cp.BearishCount),
					Severity:    disagreement,
					Symbols:     []string{sym},
				})
			}
		}
	}

	return SimulationResult{
		Timestamp:   time.Now(),
		FishResults: sw.collectPerformance(),
		Consensus:   consensus,
		Anomalies:   anomalies,
		Confidence:  sw.calculateOverallConfidence(consensus),
	}
}

// collectPerformance aggregates performance metrics
func (sw *MiroFishSwarm) collectPerformance() map[string]FishPerformance {
	results := make(map[string]FishPerformance)
	for _, fish := range sw.fish {
		results[fish.ID] = fish.Performance
	}
	return results
}

// calculateOverallConfidence computes system-wide confidence
func (sw *MiroFishSwarm) calculateOverallConfidence(consensus map[string]ConsensusPrediction) float64 {
	if len(consensus) == 0 {
		return 0.0
	}

	total := 0.0
	for _, cp := range consensus {
		total += cp.AverageConfidence
	}
	return total / float64(len(consensus))
}

// GetLatestResult returns most recent simulation result
func (sw *MiroFishSwarm) GetLatestResult() (SimulationResult, bool) {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	if len(sw.results) == 0 {
		return SimulationResult{}, false
	}
	return sw.results[len(sw.results)-1], true
}

// GetAllResults returns complete simulation history
func (sw *MiroFishSwarm) GetAllResults() []SimulationResult {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	// Return copy
	results := make([]SimulationResult, len(sw.results))
	copy(results, sw.results)
	return results
}

// GetTopFish returns best performing fish
func (sw *MiroFishSwarm) GetTopFish(n int) []*MiroFish {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	// Sort by accuracy
	sorted := make([]*MiroFish, len(sw.fish))
	copy(sorted, sw.fish)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Performance.Accuracy > sorted[i].Performance.Accuracy {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

// ExportTrainingData exports simulation results for agent training
func (sw *MiroFishSwarm) ExportTrainingData() []TrainingScenario {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	scenarios := make([]TrainingScenario, 0)

	for _, fish := range sw.fish {
		if len(fish.History) < 10 {
			continue
		}

		scenario := TrainingScenario{
			ID:          fish.ID,
			Scenario:    fish.Scenario.Name,
			States:      fish.History,
			Predictions: fish.Predictions,
			Performance: fish.Performance,
		}
		scenarios = append(scenarios, scenario)
	}

	return scenarios
}

// TrainingScenario packages data for training
type TrainingScenario struct {
	ID          string
	Scenario    string
	States      []MarketState
	Predictions []Prediction
	Performance FishPerformance
}

// Scenario generators

func (sw *MiroFishSwarm) generateBullEvents() []MarketEvent {
	return []MarketEvent{
		{Time: time.Now().Add(24 * time.Hour), Type: "rally", Magnitude: 0.03, Description: "Earnings beat"},
		{Time: time.Now().Add(72 * time.Hour), Type: "rally", Magnitude: 0.02, Description: "Fed dovish"},
	}
}

func (sw *MiroFishSwarm) generateBearEvents() []MarketEvent {
	return []MarketEvent{
		{Time: time.Now().Add(48 * time.Hour), Type: "flash_crash", Magnitude: 0.05, Description: "Margin call cascade"},
		{Time: time.Now().Add(120 * time.Hour), Type: "flash_crash", Magnitude: 0.03, Description: "Recession fears"},
	}
}

func (sw *MiroFishSwarm) generateVolatilityEvents() []MarketEvent {
	return []MarketEvent{
		{Time: time.Now().Add(12 * time.Hour), Type: "flash_crash", Magnitude: 0.04, Description: "VIX spike"},
		{Time: time.Now().Add(36 * time.Hour), Type: "rally", Magnitude: 0.04, Description: "Short squeeze"},
		{Time: time.Now().Add(60 * time.Hour), Type: "flash_crash", Magnitude: 0.03, Description: "Profit taking"},
	}
}

func (sw *MiroFishSwarm) generateQuietEvents() []MarketEvent {
	// Fewer events in quiet periods
	if rand.Float64() < 0.3 {
		return []MarketEvent{
			{Time: time.Now().Add(168 * time.Hour), Type: "earnings_surprise", Magnitude: 0.02, Description: "Unexpected earnings"},
		}
	}
	return []MarketEvent{}
}

func (sw *MiroFishSwarm) generateTransitionEvents() []MarketEvent {
	return []MarketEvent{
		{Time: time.Now().Add(24 * time.Hour), Type: "flash_crash", Magnitude: 0.02, Description: "Regime shift start"},
		{Time: time.Now().Add(96 * time.Hour), Type: "rally", Magnitude: 0.03, Description: "New trend emerges"},
	}
}

// Utility functions

func calculateSentiment(state MarketState, scenario MarketScenario) float64 {
	// Simplified sentiment calculation
	base := 0.0
	if scenario.Trend > 0 {
		base = 0.3
	} else if scenario.Trend < 0 {
		base = -0.3
	}

	// Adjust for volatility
	if state.Volatility > 0.3 {
		base *= 0.5 // Fear reduces sentiment impact
	}

	return base + rand.Float64()*0.2 - 0.1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
