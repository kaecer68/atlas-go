package orchestrator

import (
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// IntegratedSystem orchestrates all components together
type IntegratedSystem struct {
	mu sync.RWMutex

	// Core components
	darwinianManager  *portfolio.DarwinianWeightManager
	riskManager       *portfolio.RiskManager
	volatilityManager *portfolio.VolatilityManager
	prismManager      *prism.PRISMManager
	reflexivityEngine *reflexivity.ReflexivityEngine
	swarm             *swarm.MiroFishSwarm
	janusEngine       *janus.Engine
	narrativeEngine   *narrative.NarrativeEngine

	// System state
	registry               domain.AgentRegistry
	currentRecommendations []domain.Recommendation
	systemMetrics          SystemMetrics
	lastUpdate             time.Time

	// Configuration
	config SystemConfig
}

// SystemConfig holds configuration for the integrated system
type SystemConfig struct {
	TargetVolatility  float64
	MaxDrawdown       float64
	RebalanceInterval time.Duration
	RiskLimits        RiskLimits
}

// RiskLimits defines risk management limits
type RiskLimits struct {
	MaxPositionSize   float64
	MaxDailyLoss      float64
	MaxConcentration  float64
	StopLossEnabled   bool
	TakeProfitEnabled bool
}

// SystemMetrics tracks overall system performance
type SystemMetrics struct {
	TotalRecommendations int
	ActiveAgents         int
	CurrentDrawdown      float64
	CurrentVolatility    float64
	SystemHealth         float64
	LastUpdate           time.Time
}

// NewIntegratedSystem creates a new integrated system
func NewIntegratedSystem(config SystemConfig) *IntegratedSystem {
	system := &IntegratedSystem{
		config:     config,
		registry:   domain.AgentRegistry{},
		lastUpdate: time.Now(),
	}

	// Initialize components
	system.initializeComponents()

	return system
}

// initializeComponents sets up all system components
func (is *IntegratedSystem) initializeComponents() {
	// Initialize portfolio components
	is.darwinianManager = portfolio.NewDarwinianWeightManager("/tmp/integrated_weights.json")
	is.riskManager = portfolio.NewRiskManager()
	is.volatilityManager = portfolio.NewVolatilityManager(is.config.TargetVolatility, is.config.TargetVolatility*1.5)

	// Initialize other components
	is.prismManager = prism.NewPRISMManager(prism.DefaultPRISMConfig())
	is.reflexivityEngine = reflexivity.NewReflexivityEngine()
	is.swarm = swarm.NewMiroFishSwarm(swarm.DefaultSwarmConfig())
	is.janusEngine = janus.NewEngine()
	is.janusEngine.EnsureAllRegimes()
	is.narrativeEngine = narrative.NewNarrativeEngine()

	// Configure risk management
	is.riskManager.SetRiskParameters(
		is.config.MaxDrawdown,
		is.config.RiskLimits.MaxPositionSize,
		is.config.RiskLimits.MaxDailyLoss,
	)
}

// ProcessMarketData processes new market data and generates recommendations
func (is *IntegratedSystem) ProcessMarketData(marketData MarketData) ([]domain.Recommendation, error) {
	is.mu.Lock()
	defer is.mu.Unlock()

	// 1. Update volatility manager with latest returns
	is.updateVolatilityData(marketData)

	// 1.5 Detect macro narrative events and causal chains
	narrativeData := is.toNarrativeData(marketData)
	narrativeEvents := is.narrativeEngine.DetectEvents(narrativeData)
	narrativeChains := is.narrativeEngine.MatchChains(narrativeEvents)
	eventThemes := make([]string, len(narrativeEvents))
	for i, e := range narrativeEvents {
		eventThemes[i] = e.Theme
	}

	// 2. Get recommendations from all agents (now narrative-aware)
	recommendations := is.generateAgentRecommendations(marketData, narrativeEvents, narrativeChains)

	// 3. Apply Darwinian weights to adjust conviction
	weightedRecommendations := is.darwinianManager.ApplyDarwinianWeights(recommendations)

	// 4. Apply risk management filters
	filteredRecommendations := is.applyRiskFilters(weightedRecommendations)

	// 5. Update reflexivity engine with market biases
	is.updateReflexivityBias(filteredRecommendations, marketData)

	// 6. Get swarm intelligence insights
	swarmInsights := is.getSwarmInsights(marketData)

	// 6.5 Update JANUS from PRISM and apply cohort weight adjustments
	is.updateJANUSFromPRISM()
	currentRegime := is.detectCurrentRegime(marketData)
	janusAdjusted := is.janusEngine.ApplyAdjustment(filteredRecommendations, currentRegime)

	// 7. Final recommendation adjustment based on all inputs
	finalRecommendations := is.finalizeRecommendations(janusAdjusted, swarmInsights)

	is.currentRecommendations = finalRecommendations
	is.lastUpdate = time.Now()
	is.updateSystemMetrics()

	return finalRecommendations, nil
}

// MarketData represents incoming market data
type MarketData struct {
	Timestamp time.Time
	Prices    map[string]float64
	Volumes   map[string]float64
	Returns   map[string]float64
}

// updateVolatilityData updates volatility calculations
func (is *IntegratedSystem) updateVolatilityData(data MarketData) {
	for symbol, ret := range data.Returns {
		// Get historical returns for this symbol
		historicalReturns := is.getHistoricalReturns(symbol, ret)
		is.volatilityManager.UpdateReturns(symbol, historicalReturns)
	}
}

// generateAgentRecommendations gets recommendations from all active agents.
func (is *IntegratedSystem) generateAgentRecommendations(data MarketData, events []narrative.NarrativeEvent, chains []narrative.CausalChain) []domain.Recommendation {
	recommendations := make([]domain.Recommendation, 0)

	for _, agent := range is.registry.Agents {
		if !agent.Enabled {
			continue
		}

		recommendation := is.mockAgentRecommendation(agent, data, events, chains)
		recommendations = append(recommendations, recommendation)
	}

	return recommendations
}

// applyRiskFilters applies risk management filters to recommendations
func (is *IntegratedSystem) applyRiskFilters(recommendations []domain.Recommendation) []domain.Recommendation {
	filtered := make([]domain.Recommendation, 0)

	for _, rec := range recommendations {
		// Check position size limits
		positionSize := is.riskManager.CalculatePositionSize(50.0, 0.02) // Use price instead of symbol
		if positionSize <= 0 {
			continue // Skip if position size is zero or negative
		}

		// Apply conviction scaling based on risk
		scaledConviction := is.scaleConvictionByRisk(rec.Conviction, rec.Symbol)

		filteredRec := rec
		filteredRec.Conviction = scaledConviction
		filtered = append(filtered, filteredRec)
	}

	return filtered
}

// updateReflexivityBias updates market biases based on recommendations
func (is *IntegratedSystem) updateReflexivityBias(recommendations []domain.Recommendation, data MarketData) {
	// Analyze recommendation patterns to detect biases
	biasAnalysis := is.analyzeRecommendationBiases(recommendations)

	// Register detected biases
	for _, bias := range biasAnalysis {
		is.reflexivityEngine.RegisterBias(bias)
	}

	// Update market reality
	reality := &reflexivity.MarketReality{
		ID:         fmt.Sprintf("market_%d", time.Now().Unix()),
		Target:     "MARKET",
		Price:      is.calculateAveragePrice(data.Prices),
		Volatility: is.calculateMarketVolatility(data),
		Volume:     is.calculateTotalVolume(data.Volumes),
		Timestamp:  data.Timestamp,
	}
	is.reflexivityEngine.UpdateReality(reality)
}

// getSwarmInsights gets intelligence from the swarm system
func (is *IntegratedSystem) getSwarmInsights(data MarketData) SwarmInsights {
	// Initialize swarm with current market state
	_ = struct {
		Timestamp time.Time
		Prices    map[string]float64
		Volumes   map[string]float64
	}{
		Timestamp: data.Timestamp,
		Prices:    data.Prices,
		Volumes:   data.Volumes,
	}

	// Get top performing scenarios
	topScenarios := is.swarm.GetTopFish(10)
	trainingData := is.swarm.ExportTrainingData()

	return SwarmInsights{
		TopScenarios: topScenarios,
		TrainingData: trainingData,
		Consensus:    is.calculateSwarmConsensus(topScenarios),
		Confidence:   is.calculateSwarmConfidence(topScenarios),
	}
}

// SwarmInsights represents insights from the swarm system
type SwarmInsights struct {
	TopScenarios []*swarm.MiroFish
	TrainingData []swarm.TrainingScenario
	Consensus    float64
	Confidence   float64
}

// finalizeRecommendations applies final adjustments based on all system inputs
func (is *IntegratedSystem) finalizeRecommendations(recommendations []domain.Recommendation, insights SwarmInsights) []domain.Recommendation {
	final := make([]domain.Recommendation, len(recommendations))

	for i, rec := range recommendations {
		adjustedRec := rec

		// Apply swarm consensus adjustment
		if insights.Consensus > 0.7 {
			adjustedRec.Conviction = int(float64(adjustedRec.Conviction) * 1.1)
		} else if insights.Consensus < 0.3 {
			adjustedRec.Conviction = int(float64(adjustedRec.Conviction) * 0.9)
		}

		// Apply volatility adjustment
		volMetrics := is.volatilityManager.GetVolatilityMetrics()
		if volMetrics.CurrentVolatility > volMetrics.TargetVolatility*1.2 {
			adjustedRec.Conviction = int(float64(adjustedRec.Conviction) * 0.8)
		}

		// Apply reflexivity feedback loop adjustments
		loops := is.reflexivityEngine.GetActiveLoops()
		for _, loop := range loops {
			if loop.Direction == reflexivity.PositiveFeedback && loop.Strength > 0.5 {
				// Reduce conviction during strong positive feedback (bubble risk)
				adjustedRec.Conviction = int(float64(adjustedRec.Conviction) * 0.85)
			}
		}

		final[i] = adjustedRec
	}

	return final
}

// UpdatePortfolio updates portfolio state based on executed trades
func (is *IntegratedSystem) UpdatePortfolio(trades []Trade) error {
	is.mu.Lock()
	defer is.mu.Unlock()

	for _, trade := range trades {
		// Update risk manager
		if trade.Action == "BUY" {
			err := is.riskManager.AddPosition(trade.Symbol, trade.Quantity, trade.Price)
			if err != nil {
				return fmt.Errorf("failed to add position %s: %w", trade.Symbol, err)
			}
		} else {
			is.riskManager.RemovePosition(trade.Symbol, trade.PnL)
		}

		// Update position prices
		is.riskManager.UpdatePosition(trade.Symbol, trade.Price)

		// Record outcomes for Darwinian weights
		is.darwinianManager.RecordOutcome(trade.AgentID, trade.Return, trade.Return > 0)
	}

	// Update portfolio value
	totalValue := is.calculatePortfolioValue()
	is.riskManager.UpdatePortfolioValue(totalValue)

	// Check for daily adjustments
	if time.Since(is.lastUpdate) > 24*time.Hour {
		is.performDailyAdjustments()
	}

	// Update JANUS state periodically (also done in ProcessMarketData, but ensure coherence)
	is.janusEngine.Update()

	return nil
}

// Trade represents an executed trade
type Trade struct {
	Symbol    string
	Action    string // BUY or SELL
	Quantity  float64
	Price     float64
	AgentID   string
	Return    float64
	PnL       float64
	Timestamp time.Time
}

// performDailyAdjustments performs daily system adjustments
// updateJANUSFromPRISM pulls completed PRISM training results and feeds them
// into the JANUS meta-layer for cohort weight recomputation.
func (is *IntegratedSystem) updateJANUSFromPRISM() {
	if is.prismManager == nil || is.janusEngine == nil {
		return
	}

	results := is.prismManager.GetCompletedResults()
	if len(results) == 0 {
		return
	}

	for _, res := range results {
		is.janusEngine.RecordTrainingResult(res.Regime, res.Result)
	}

	is.prismManager.ClearCompletedResults()
	is.janusEngine.Update()
}

// detectCurrentRegime derives a domain.Regime from current market data.
// This is a simplified heuristic; in production it would use the RegimeExecutor.
func (is *IntegratedSystem) detectCurrentRegime(data MarketData) domain.Regime {
	vol := is.calculateMarketVolatility(data)
	avgPrice := is.calculateAveragePrice(data.Prices)

	// Naive heuristic for demonstration:
	// High volatility + declining prices -> RISK_OFF
	// Low volatility + stable prices -> NEUTRAL
	// Otherwise -> RISK_ON
	_ = avgPrice
	if vol > 0.03 {
		return domain.RegimeRiskOff
	}
	if vol < 0.015 {
		return domain.RegimeNeutral
	}
	return domain.RegimeRiskOn
}

func (is *IntegratedSystem) performDailyAdjustments() {
	// Adjust Darwinian weights
	_ = is.darwinianManager.PerformDailyAdjustment()

	// Update JANUS state explicitly on daily boundary
	is.updateJANUSFromPRISM()
	is.janusEngine.Update()

	// Get volatility adjustments
	adjustments := is.volatilityManager.GetVolatilityAdjustments()

	// Apply adjustments
	is.applyVolatilityAdjustments(adjustments)

	// Reset daily risk tracking
	is.riskManager.ResetDaily()
}

// GetSystemStatus returns current system status
func (is *IntegratedSystem) GetSystemStatus() SystemStatus {
	is.mu.RLock()
	defer is.mu.RUnlock()

	return SystemStatus{
		Metrics:           is.systemMetrics,
		ActiveAlerts:      is.riskManager.GetActiveAlerts(),
		VolatilityMetrics: is.volatilityManager.GetVolatilityMetrics(),
		ReflexivityLoops:  is.reflexivityEngine.GetActiveLoops(),
		JANUSStatus:       is.janusEngine.GetStatus(),
		HealthScore:       is.calculateHealthScore(),
		LastUpdate:        is.lastUpdate,
	}
}

// SystemStatus represents overall system status
type SystemStatus struct {
	Metrics           SystemMetrics
	ActiveAlerts      []portfolio.RiskAlert
	VolatilityMetrics portfolio.VolatilityMetrics
	ReflexivityLoops  []reflexivity.FeedbackLoop
	JANUSStatus       janus.Status
	HealthScore       float64
	LastUpdate        time.Time
}

// Helper methods (simplified implementations)

func (is *IntegratedSystem) mockAgentRecommendation(agent domain.AgentSpec, data MarketData, events []narrative.NarrativeEvent, chains []narrative.CausalChain) domain.Recommendation {
	rec := domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     "SAMPLE",
		Side:       domain.SideBuy,
		Conviction: 70,
		Reason:     "Mock recommendation",
	}

	// Layer 1 context agents become narrative-aware.
	if agent.Layer == "context" || agent.Layer == "superinvestor" {
		if len(events) > 0 {
			rec.Reason = fmt.Sprintf("Detected %d narrative event(s): %s", len(events), events[0].Theme)
			rec.ReasoningChain = []string{}
			rec.SupportingEvents = []string{}
			for _, e := range events {
				rec.SupportingEvents = append(rec.SupportingEvents, e.ID)
				rec.ReasoningChain = append(rec.ReasoningChain, fmt.Sprintf("%s (%s, confidence %.2f)", e.Theme, e.Region, e.Confidence))
			}
			for _, c := range chains {
				if len(c.Steps) > 0 {
					rec.ReasoningChain = append(rec.ReasoningChain, fmt.Sprintf("Chain %s: %s", c.TemplateID, c.Steps[0].Description))
				}
			}
		}
	}

	// Sector agents pick up narrative-driven sector bias.
	if agent.Layer == "sector" && len(chains) > 0 {
		for _, c := range chains {
			for _, step := range c.Steps {
				for _, affected := range step.Affected {
					if isSectorMatch(agent.Skill, affected) {
						rec.Reason = fmt.Sprintf("Narrative '%s' affects %s (impact %.2f)", c.TemplateID, affected, step.Impact)
						rec.SupportingEvents = append(rec.SupportingEvents, c.EventID)
						if step.Impact > 0 {
							rec.Conviction = 80
						} else {
							rec.Side = domain.SideSell
							rec.Conviction = 60
						}
						break
					}
				}
			}
		}
	}

	return rec
}

func isSectorMatch(skill, affected string) bool {
	mappings := map[string][]string{
		"semiconductor_desk":     {"semiconductor", "foundry"},
		"ai_supply_chain_desk":   {"ai_supply_chain", "pcb", "thermal"},
		"financials_desk":        {"financials"},
		"shipping_desk":          {"shipping"},
		"etf_rotation_desk":      {"high_dividend", "etf_rotation"},
	}
	for _, s := range mappings[skill] {
		if s == affected {
			return true
		}
	}
	return false
}

func (is *IntegratedSystem) toNarrativeData(data MarketData) narrative.MarketNarrativeData {
	nd := narrative.MarketNarrativeData{}
	// Simple heuristics derived from market data.
	if len(data.Returns) > 0 {
		var avgRet float64
		for _, r := range data.Returns {
			avgRet += r
		}
		avgRet /= float64(len(data.Returns))
		// Proxy VIX level via cross-sectional return dispersion.
		nd.VIXLevel = avgRet * 100
	}
	if v, ok := data.Prices["DXY"]; ok && v > 0 {
		nd.DXYChangePct = v / 100.0
	}
	if v, ok := data.Prices["US10Y"]; ok && v > 0 {
		nd.US10YChangeBps = v
	}
	if v, ok := data.Prices["OIL"]; ok && v > 0 {
		nd.OilChangePct = v / 10.0
	}
	if v, ok := data.Prices["GOLD"]; ok && v > 0 {
		nd.GoldChangePct = v / 10.0
	}
	return nd
}

func (is *IntegratedSystem) getHistoricalReturns(symbol string, newReturn float64) []float64 {
	// In practice, this would fetch from database
	return []float64{newReturn - 0.01, newReturn + 0.02, newReturn - 0.005}
}

func (is *IntegratedSystem) scaleConvictionByRisk(conviction int, symbol string) int {
	// Scale conviction based on risk metrics
	return conviction
}

func (is *IntegratedSystem) analyzeRecommendationBiases(recommendations []domain.Recommendation) []*reflexivity.MarketBias {
	// Analyze patterns to detect biases
	return []*reflexivity.MarketBias{}
}

func (is *IntegratedSystem) calculateAveragePrice(prices map[string]float64) float64 {
	if len(prices) == 0 {
		return 0
	}
	sum := 0.0
	for _, price := range prices {
		sum += price
	}
	return sum / float64(len(prices))
}

func (is *IntegratedSystem) calculateMarketVolatility(data MarketData) float64 {
	// Simplified volatility calculation
	return 0.02
}

func (is *IntegratedSystem) calculateTotalVolume(volumes map[string]float64) float64 {
	sum := 0.0
	for _, volume := range volumes {
		sum += volume
	}
	return sum
}

func (is *IntegratedSystem) calculateSwarmConsensus(topFish []*swarm.MiroFish) float64 {
	return 0.6 // Mock consensus
}

func (is *IntegratedSystem) calculateSwarmConfidence(topFish []*swarm.MiroFish) float64 {
	return 0.7 // Mock confidence
}

func (is *IntegratedSystem) calculatePortfolioValue() float64 {
	return 100000.0 // Mock portfolio value
}

func (is *IntegratedSystem) applyVolatilityAdjustments(adjustments []portfolio.VolatilityAdjustment) {
	_ = adjustments // Apply volatility adjustments (placeholder)
}

func (is *IntegratedSystem) updateSystemMetrics() {
	is.systemMetrics = SystemMetrics{
		TotalRecommendations: len(is.currentRecommendations),
		ActiveAgents:         len(is.registry.Agents),
		CurrentDrawdown:      is.riskManager.GetRiskMetrics().CurrentDrawdown,
		CurrentVolatility:    is.volatilityManager.GetVolatilityMetrics().CurrentVolatility,
		SystemHealth:         is.calculateHealthScore(),
		LastUpdate:           time.Now(),
	}
}

func (is *IntegratedSystem) calculateHealthScore() float64 {
	// Calculate overall system health score
	riskMetrics := is.riskManager.GetRiskMetrics()
	volMetrics := is.volatilityManager.GetVolatilityMetrics()

	score := 100.0

	// Deduct for risk alerts
	alerts := is.riskManager.GetActiveAlerts()
	score -= float64(len(alerts)) * 5

	// Deduct for volatility deviation
	if volMetrics.Deviation > 0.2 {
		score -= 20
	}

	// Deduct for drawdown
	if riskMetrics.CurrentDrawdown > 0.05 {
		score -= 30
	}

	if score < 0 {
		score = 0
	}

	return score
}

// ... (rest of the code remains the same)
