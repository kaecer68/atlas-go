// Package swarm implements MiroFish Swarm - parallel simulated futures training
// Simulates multiple possible market futures to train agents on diverse scenarios
package swarm

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/stress"
)

// MiroFishSwarm manages parallel market simulations.
// Start() is synchronous — it runs all simulations to completion and
// stores a consensus result. No background goroutines or tickers.
type MiroFishSwarm struct {
	config      SwarmConfig
	fish        []*MiroFish
	scenarios   []MarketScenario
	results     []SimulationResult
	generations int
	mu          sync.RWMutex
}

// MiroFish represents a single simulation unit
type MiroFish struct {
	ID           string
	Scenario     MarketScenario
	CurrentState MarketState
	History      []MarketState
	Predictions  []Prediction
	Performance  FishPerformance
	Rule         PredictionRule
	GARCH        *GARCHProcess
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
	Timestamp          time.Time
	Prices             map[string]float64
	Volumes            map[string]float64
	Sentiment          float64
	Volatility         float64
	Correlation        float64
	RealizedVolatility float64 // optional: actual realized vol from market data
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

// InitializeScenarios sets up diverse market scenarios using hardcoded parameters.
// For production use, prefer InitializeScenariosFromStress which derives parameters
// from historical stress test scenarios based on real market data.
func (sw *MiroFishSwarm) InitializeScenarios(baseState MarketState) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	baseTime := baseState.Timestamp

	// Generate diverse scenarios
	sw.scenarios = []MarketScenario{
		{
			ID:         "bull_trend",
			Name:       "Bull Market Trend",
			Regime:     "risk_on",
			Volatility: 0.15,
			Trend:      0.001,
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateBullEvents(baseTime),
		},
		{
			ID:         "bear_trend",
			Name:       "Bear Market Trend",
			Regime:     "risk_off",
			Volatility: 0.25,
			Trend:      -0.002,
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateBearEvents(baseTime),
		},
		{
			ID:         "high_vol",
			Name:       "High Volatility",
			Regime:     "crisis",
			Volatility: 0.40,
			Trend:      0.0,
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateVolatilityEvents(baseTime),
		},
		{
			ID:         "low_vol",
			Name:       "Low Volatility Range",
			Regime:     "complacent",
			Volatility: 0.08,
			Trend:      0.0001,
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateQuietEvents(baseTime),
		},
		{
			ID:         "transition",
			Name:       "Regime Transition",
			Regime:     "transition",
			Volatility: 0.20,
			Trend:      0.0,
			Duration:   sw.config.SimulationHorizon,
			Events:     sw.generateTransitionEvents(baseTime),
		},
	}

	// Spawn fish for each scenario
	fishPerScenario := sw.config.FishCount / len(sw.scenarios)
	for _, scenario := range sw.scenarios {
		omega, alpha, beta := GARCHParamsForRegime(scenario.Regime)
		for i := range fishPerScenario {
			fish := &MiroFish{
				ID:           fmt.Sprintf("fish_%s_%d", scenario.ID, i),
				Scenario:     scenario,
				CurrentState: baseState,
				History:      make([]MarketState, 0),
				Predictions:  make([]Prediction, 0),
				Rule:         RandomPredictionRule(),
				GARCH:        NewGARCHProcess(omega, alpha, beta, scenario.Volatility, baseState.RealizedVolatility/math.Sqrt(252.0)),
				isAlive:      true,
				spawnedAt:    time.Now(),
			}
			sw.fish = append(sw.fish, fish)
		}
	}

	logging.Info("mirofish_swarm", "initialized", "fish_count", len(sw.fish), "scenario_count", len(sw.scenarios))
}

// InitializeScenariosFromStress creates MarketScenario entries by mapping
// stress.Scenario data. It derives volatility from VIX quotes (when available)
// or from price range, maps stress.Regime to swarm regime strings, and creates
// events from the scenario's description and date.
//
// This avoids duplicating hardcoded scenario definitions between the swarm and
// stress modules. The stress scenarios are based on real historical market data.
func (sw *MiroFishSwarm) InitializeScenariosFromStress(baseState MarketState, stressScenarios []stress.Scenario) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	baseTime := baseState.Timestamp

	sw.scenarios = make([]MarketScenario, 0, len(stressScenarios))
	for _, s := range stressScenarios {
		vol := estimateVolatilityFromStress(s)
		regime := mapStressRegimeToSwarm(s.Regime)
		trend := estimateTrendFromStress(s)
		events := buildEventsFromStress(s, baseTime)

		sw.scenarios = append(sw.scenarios, MarketScenario{
			ID:         s.ID,
			Name:       s.Name,
			Regime:     regime,
			Volatility: vol,
			Trend:      trend,
			Duration:   time.Duration(s.WindowDays) * 24 * time.Hour,
			Events:     events,
		})
	}

	// Spawn fish for each scenario
	if len(sw.scenarios) == 0 {
		logging.Warn("mirofish_swarm", "no_scenarios_from_stress", "fish_count", 0)
		return
	}

	fishPerScenario := sw.config.FishCount / len(sw.scenarios)
	for _, scenario := range sw.scenarios {
		omega, alpha, beta := GARCHParamsForRegime(scenario.Regime)
		for i := range fishPerScenario {
			fish := &MiroFish{
				ID:           fmt.Sprintf("fish_%s_%d", scenario.ID, i),
				Scenario:     scenario,
				CurrentState: baseState,
				History:      make([]MarketState, 0),
				Predictions:  make([]Prediction, 0),
				Rule:         RandomPredictionRule(),
				GARCH:        NewGARCHProcess(omega, alpha, beta, scenario.Volatility, baseState.RealizedVolatility/math.Sqrt(252.0)),
				isAlive:      true,
				spawnedAt:    time.Now(),
			}
			sw.fish = append(sw.fish, fish)
		}
	}

	logging.Info("mirofish_swarm", "initialized_from_stress",
		"fish_count", len(sw.fish),
		"scenario_count", len(sw.scenarios),
		"source", "stress_module")
}

// estimateVolatilityFromStress derives an annualized volatility estimate from
// a stress scenario. It prefers the VIX quote when available; otherwise it
// falls back to the maximum price-range ratio across all quoted assets.
func estimateVolatilityFromStress(s stress.Scenario) float64 {
	for _, q := range s.Quotes {
		if strings.EqualFold(q.Symbol, "VIX") && q.Last > 0 {
			return q.Last / 100.0
		}
	}

	maxRange := 0.0
	for _, q := range s.Quotes {
		if q.Open > 0 && q.Last > 0 {
			r := math.Abs(q.Last-q.Open) / q.Open
			if r > maxRange {
				maxRange = r
			}
		}
	}
	if maxRange > 0 {
		return maxRange * math.Sqrt(252.0)
	}
	return 0.20
}

// mapStressRegimeToSwarm converts a domain.Regime to the string regime
// identifiers used by the swarm module.
func mapStressRegimeToSwarm(r domain.Regime) string {
	switch r {
	case domain.RegimeRiskOn:
		return "risk_on"
	case domain.RegimeRiskOff:
		return "risk_off"
	case domain.RegimeNeutral:
		return "complacent"
	default:
		return "transition"
	}
}

// estimateTrendFromStress infers a daily trend from the scenario's quotes.
// A positive gap between last and open price suggests an upward trend;
// a negative gap suggests a downward trend. The magnitude is scaled to a
// reasonable daily drift range.
func estimateTrendFromStress(s stress.Scenario) float64 {
	var totalReturn, count float64
	for _, q := range s.Quotes {
		if q.Open > 0 && q.Last > 0 {
			totalReturn += (q.Last - q.Open) / q.Open
			count++
		}
	}
	if count == 0 {
		return 0.0
	}
	avgReturn := totalReturn / count
	return math.Max(-0.005, math.Min(0.005, avgReturn/100.0))
}

// buildEventsFromStress creates MarketEvent entries from a stress scenario's
// metadata. The scenario date is used as the event time; the description is
// mapped to an event type based on keywords.
func buildEventsFromStress(s stress.Scenario, baseTime time.Time) []MarketEvent {
	if s.Description == "" {
		return nil
	}

	eventType := "earnings_surprise"
	descLower := strings.ToLower(s.Description)
	switch {
	case strings.Contains(descLower, "crash") || strings.Contains(descLower, "spike") || strings.Contains(descLower, "freeze"):
		eventType = "flash_crash"
	case strings.Contains(descLower, "rally") || strings.Contains(descLower, "surge") || strings.Contains(descLower, "euphoria"):
		eventType = "rally"
	}

	magnitude := 0.02
	if strings.Contains(descLower, "crash") || strings.Contains(descLower, "spike") {
		magnitude = 0.05
	} else if strings.Contains(descLower, "rally") || strings.Contains(descLower, "surge") {
		magnitude = 0.03
	}

	eventTime := baseTime.Add(24 * time.Hour)
	if !s.Date.IsZero() {
		eventTime = s.Date
	}

	return []MarketEvent{
		{
			Time:        eventTime,
			Type:        eventType,
			Magnitude:   magnitude,
			Description: s.Description,
		},
	}
}

// Start runs the swarm simulation synchronously.
// All fish are simulated to completion, then a consensus result is computed
// and stored. No background goroutines or tickers.
func (sw *MiroFishSwarm) Start() {
	sw.mu.Lock()

	var wg sync.WaitGroup
	batchSize := max(1, len(sw.fish)/sw.config.Parallelism)
	for i := 0; i < sw.config.Parallelism && i*batchSize < len(sw.fish); i++ {
		start := i * batchSize
		end := start + batchSize
		if i == sw.config.Parallelism-1 || end > len(sw.fish) {
			end = len(sw.fish)
		}
		wg.Add(1)
		go func(batch []*MiroFish) {
			defer wg.Done()
			sw.runBatch(batch)
		}(sw.fish[start:end])
	}
	sw.mu.Unlock()

	wg.Wait()

	sw.mu.Lock()
	result := sw.computeConsensus()
	sw.results = append(sw.results, result)
	sw.mu.Unlock()

	logging.Info("mirofish_swarm", "swarm_completed", "fish_count", len(sw.fish))
}

// Stop is a no-op kept for backward compatibility. Start() is now synchronous.
func (sw *MiroFishSwarm) Stop() {}

// runBatch simulates a batch of fish
func (sw *MiroFishSwarm) runBatch(fish []*MiroFish) {
	for _, f := range fish {
		sw.simulateFish(f)
	}
}

// simulateFish runs full simulation for a single fish
func (sw *MiroFishSwarm) simulateFish(fish *MiroFish) {
	steps := int(fish.Scenario.Duration / sw.config.TimeStep)

	var prevState MarketState
	var prevPred Prediction
	for i := 0; i < steps && fish.isAlive; i++ {
		newState := sw.evolveState(fish, fish.CurrentState, fish.Scenario, i)
		fish.History = append(fish.History, newState)

		if i > 0 {
			fish.Performance.TotalPredictions++
			if prevState.Prices != nil {
				actualReturn := newState.Prices[prevPred.Symbol] - prevState.Prices[prevPred.Symbol]
				var actualDir string
				switch {
				case actualReturn > 0:
					actualDir = "up"
				case actualReturn < 0:
					actualDir = "down"
				default:
					actualDir = "neutral"
				}
				if prevPred.Direction == actualDir {
					fish.Performance.CorrectPredictions++
				}
				if actualDir == "up" || actualDir == "down" {
					fish.Performance.PnL += actualReturn * float64(prevPred.Conviction) / 100.0
				}
			}
			if fish.Performance.TotalPredictions > 0 {
				fish.Performance.Accuracy = float64(fish.Performance.CorrectPredictions) / float64(fish.Performance.TotalPredictions)
			}
		}

		fish.CurrentState = newState

		pred := sw.generatePrediction(fish, newState)
		fish.Predictions = append(fish.Predictions, pred)

		prevState = newState
		prevPred = pred
	}
}

// evolveState advances market state according to scenario dynamics,
// fish's GARCH process, cross-asset correlation, and jump-diffusion.
func (sw *MiroFishSwarm) evolveState(fish *MiroFish, current MarketState, scenario MarketScenario, step int) MarketState {
	newState := MarketState{
		Timestamp: current.Timestamp.Add(sw.config.TimeStep),
		Prices:    make(map[string]float64),
		Volumes:   make(map[string]float64),
	}

	// Collect symbols for correlated shock generation
	symbols := make([]string, 0, len(current.Prices))
	for sym := range current.Prices {
		symbols = append(symbols, sym)
	}

	// Generate correlated shocks across all symbols
	corrShocks := CorrelatedShocks(symbols, scenario.Regime)

	// Jump process (systemic, applied once per step)
	jumpProcess := JumpParamsForRegime(scenario.Regime)
	var jumpMagnitude float64
	if jumpProcess.ShouldJump() {
		jumpMagnitude = jumpProcess.Magnitude()
	}

	for _, symbol := range symbols {
		price := current.Prices[symbol]
		drift := scenario.Trend * float64(sw.config.TimeStep.Hours()) / 24.0
		sigma := fish.GARCH.CurrentSigma()
		corrShock := corrShocks[symbol]
		shock := corrShock*sigma + jumpMagnitude

		newPrice := price * (1 + drift + shock)
		newState.Prices[symbol] = newPrice
		newState.Volumes[symbol] = current.Volumes[symbol] * (1 + rand.Float64()*0.2)

		fish.GARCH.Advance(shock)
	}

	for _, event := range scenario.Events {
		eventStep := int(event.Time.Sub(current.Timestamp) / sw.config.TimeStep)
		if eventStep == step {
			sw.applyEvent(&newState, event)
		}
	}

	newState.Volatility = scenario.Volatility * (1 + rand.Float64()*0.3)
	newState.Sentiment = calculateSentiment(newState, scenario)
	newState.Correlation = correlationForRegime(scenario.Regime)

	return newState
}

// generatePrediction creates prediction based on fish's view of market.
// Uses the fish's PredictionRule for strategy parameters.
func (sw *MiroFishSwarm) generatePrediction(fish *MiroFish, state MarketState) Prediction {
	var targetSymbol string
	var targetPrice float64

	symbols := make([]string, 0, len(state.Prices))
	for sym := range state.Prices {
		symbols = append(symbols, sym)
	}
	sort.Strings(symbols)
	for _, sym := range symbols {
		targetSymbol = sym
		targetPrice = state.Prices[sym]
		break
	}

	rule := fish.Rule
	direction := "neutral"
	if len(fish.History) > rule.LookbackWindow {
		oldPrice := fish.History[len(fish.History)-rule.LookbackWindow].Prices[targetSymbol]
		ratio := targetPrice / oldPrice
		if ratio > rule.TrendUpThreshold {
			direction = "up"
		} else if ratio < rule.TrendDownThreshold {
			direction = "down"
		}
	}

	// Apply contrarian bias
	if rule.ContrarianBias < 0 && direction == "up" {
		if rand.Float64() < -rule.ContrarianBias {
			direction = "down"
		}
	} else if rule.ContrarianBias < 0 && direction == "down" {
		if rand.Float64() < -rule.ContrarianBias {
			direction = "up"
		}
	}

	// Apply sentiment if enabled
	if rule.UseSentiment && state.Sentiment > 0.3 && direction != "up" {
		direction = "up"
	} else if rule.UseSentiment && state.Sentiment < -0.3 && direction != "down" {
		direction = "down"
	}

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
		Rationale:  fmt.Sprintf("Rule:%s regime=%s vol=%.1f%%", rule.RuleSummary(), fish.Scenario.Regime, state.Volatility*100),
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
		// Affects all symbols with individual random magnitudes
		for sym := range state.Prices {
			state.Prices[sym] *= (1 + event.Magnitude*(rand.Float64()-0.5))
		}
	default:
	}
}

// computeConsensus aggregates fish predictions into consensus view
func (sw *MiroFishSwarm) computeConsensus() SimulationResult {
	consensus := make(map[string]ConsensusPrediction)
	anomalies := make([]Anomaly, 0)

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

	for sym, cp := range consensus {
		total := cp.BullishCount + cp.BearishCount + cp.NeutralCount
		if total > 0 {
			cp.AverageConfidence /= float64(total)
		}

		if cp.BullishCount > cp.BearishCount && cp.BullishCount > cp.NeutralCount {
			cp.ConsensusDirection = "bullish"
		} else if cp.BearishCount > cp.BullishCount && cp.BearishCount > cp.NeutralCount {
			cp.ConsensusDirection = "bearish"
		} else {
			cp.ConsensusDirection = "neutral"
		}

		consensus[sym] = cp

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

	results := make([]SimulationResult, len(sw.results))
	copy(results, sw.results)
	return results
}

// GetTopFish returns best performing fish
func (sw *MiroFishSwarm) GetTopFish(n int) []*MiroFish {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.getTopFishUnsafe(n)
}

// getTopFishUnsafe is like GetTopFish but without locking (caller must hold lock).
func (sw *MiroFishSwarm) getTopFishUnsafe(n int) []*MiroFish {
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

// EvolveGeneration selects top-performing fish and uses their rules to
// create replacement fish via crossover and mutation. Bottom 40% of fish
// are replaced. This implements the selection mechanism for strategy diversity.
// Call after Start() completes for meaningful accuracy-based selection.
func (sw *MiroFishSwarm) EvolveGeneration() {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	totalFish := len(sw.fish)
	if totalFish == 0 {
		return
	}

	eliteCount := max(1, totalFish*3/10)
	elite := sw.getTopFishUnsafe(eliteCount)
	if len(elite) < 2 {
		return
	}

	replaceCount := totalFish * 4 / 10
	for i := range replaceCount {
		bottomIdx := totalFish - 1 - i
		if bottomIdx < eliteCount {
			break
		}

		p1 := elite[rand.Intn(len(elite))]
		p2 := elite[rand.Intn(len(elite))]

		childRule := CrossoverRules(p1.Rule, p2.Rule)
		childRule = MutateRule(childRule, 0.15)

		oldFish := sw.fish[bottomIdx]
		oldFish.Rule = childRule
		omega, alpha, beta := GARCHParamsForRegime(oldFish.Scenario.Regime)
		oldFish.GARCH = NewGARCHProcess(omega, alpha, beta, oldFish.Scenario.Volatility)
		oldFish.Performance = FishPerformance{}
		oldFish.Predictions = oldFish.Predictions[:0]
		oldFish.History = oldFish.History[:0]
	}

	sw.generations++

	logging.Info("mirofish_swarm", "generation_evolved",
		"elite", eliteCount, "replaced", replaceCount, "total", totalFish, "gen", sw.generations)
}

// Generations returns the number of evolution cycles completed.
func (sw *MiroFishSwarm) Generations() int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.generations
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
			Rule:        fish.Rule,
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
	Rule        PredictionRule
}

// Scenario generators

func (sw *MiroFishSwarm) generateBullEvents(baseTime time.Time) []MarketEvent {
	return []MarketEvent{
		{Time: baseTime.Add(24 * time.Hour), Type: "rally", Magnitude: 0.03, Description: "Earnings beat"},
		{Time: baseTime.Add(72 * time.Hour), Type: "rally", Magnitude: 0.02, Description: "Fed dovish"},
	}
}

func (sw *MiroFishSwarm) generateBearEvents(baseTime time.Time) []MarketEvent {
	return []MarketEvent{
		{Time: baseTime.Add(48 * time.Hour), Type: "flash_crash", Magnitude: 0.05, Description: "Margin call cascade"},
		{Time: baseTime.Add(120 * time.Hour), Type: "flash_crash", Magnitude: 0.03, Description: "Recession fears"},
	}
}

func (sw *MiroFishSwarm) generateVolatilityEvents(baseTime time.Time) []MarketEvent {
	return []MarketEvent{
		{Time: baseTime.Add(12 * time.Hour), Type: "flash_crash", Magnitude: 0.04, Description: "VIX spike"},
		{Time: baseTime.Add(36 * time.Hour), Type: "rally", Magnitude: 0.04, Description: "Short squeeze"},
		{Time: baseTime.Add(60 * time.Hour), Type: "flash_crash", Magnitude: 0.03, Description: "Profit taking"},
	}
}

func (sw *MiroFishSwarm) generateQuietEvents(baseTime time.Time) []MarketEvent {
	if rand.Float64() < 0.3 {
		return []MarketEvent{
			{Time: baseTime.Add(168 * time.Hour), Type: "earnings_surprise", Magnitude: 0.02, Description: "Unexpected earnings"},
		}
	}
	return []MarketEvent{}
}

func (sw *MiroFishSwarm) generateTransitionEvents(baseTime time.Time) []MarketEvent {
	return []MarketEvent{
		{Time: baseTime.Add(24 * time.Hour), Type: "flash_crash", Magnitude: 0.02, Description: "Regime shift start"},
		{Time: baseTime.Add(96 * time.Hour), Type: "rally", Magnitude: 0.03, Description: "New trend emerges"},
	}
}

// Utility functions

func calculateSentiment(state MarketState, scenario MarketScenario) float64 {
	base := 0.0
	if scenario.Trend > 0 {
		base = 0.3
	} else if scenario.Trend < 0 {
		base = -0.3
	}

	if state.Volatility > 0.3 {
		base *= 0.5
	}

	return base + rand.Float64()*0.2 - 0.1
}
