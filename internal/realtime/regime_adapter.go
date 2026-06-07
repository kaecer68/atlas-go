// Package realtime implements sub-second regime detection and adaptation
// Monitors market conditions in real-time and triggers rapid agent adjustments
package realtime

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// RegimeType represents current market regime
type RegimeType string

const (
	RegimeCalm         RegimeType = "calm"
	RegimeVolatile     RegimeType = "volatile"
	RegimeTrendingUp   RegimeType = "trending_up"
	RegimeTrendingDown RegimeType = "trending_down"
	RegimeReversing    RegimeType = "reversing"
	RegimeBreakout     RegimeType = "breakout"
	RegimeBreakdown    RegimeType = "breakdown"
)

// MarketDataPoint represents a single market observation
type MarketDataPoint struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Volume    float64   `json:"volume"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	Spread    float64   `json:"spread"`
	Timestamp time.Time `json:"timestamp"`
}

// RegimeDetector analyzes market conditions for regime classification
type RegimeDetector struct {
	windowSize           int
	volatilityThreshold  float64
	volumeSpikeThreshold float64
	priceChangeThreshold float64
}

func defaultRealtimeConfig() *config.RealtimeParameters {
	return &config.RealtimeParameters{
		VolatilityThreshold:  config.ParameterMetadata[float64]{Value: 0.02},
		VolumeSpikeThreshold: config.ParameterMetadata[float64]{Value: 2.0},
		PriceChangeThreshold: config.ParameterMetadata[float64]{Value: 0.01},
		MinConfidence:        config.ParameterMetadata[float64]{Value: 0.7},
		WeightAdjustmentRate: config.ParameterMetadata[float64]{Value: 0.1},
		MaxWeightChange:      config.ParameterMetadata[float64]{Value: 0.5},
		MinWeight:            config.ParameterMetadata[float64]{Value: 0.1},
		UpdateIntervalMs:     config.ParameterMetadata[int]{Value: 100},
	}
}

// NewRegimeDetector creates a detector with configurable thresholds
func NewRegimeDetector(params *config.RealtimeParameters) *RegimeDetector {
	if params == nil {
		params = defaultRealtimeConfig()
	}
	return &RegimeDetector{
		windowSize:           60,
		volatilityThreshold:  params.VolatilityThreshold.Value,
		volumeSpikeThreshold: params.VolumeSpikeThreshold.Value,
		priceChangeThreshold: params.PriceChangeThreshold.Value,
	}
}

// DetectRegime determines current market regime from data window
func (rd *RegimeDetector) DetectRegime(data []MarketDataPoint) RegimeType {
	if len(data) < rd.windowSize/2 {
		return RegimeCalm
	}

	// Calculate metrics
	volatility := rd.calculateVolatility(data)
	volumeSpike := rd.detectVolumeSpike(data)
	priceTrend := rd.calculatePriceTrend(data)
	reversalPattern := rd.detectReversal(data)

	// Determine regime
	if reversalPattern {
		return RegimeReversing
	}

	if volumeSpike && math.Abs(priceTrend) > rd.priceChangeThreshold*2 {
		if priceTrend > 0 {
			return RegimeBreakout
		}
		return RegimeBreakdown
	}

	if volatility > rd.volatilityThreshold {
		return RegimeVolatile
	}

	if math.Abs(priceTrend) > rd.priceChangeThreshold {
		if priceTrend > 0 {
			return RegimeTrendingUp
		}
		return RegimeTrendingDown
	}

	return RegimeCalm
}

// calculateVolatility computes standard deviation of returns
func (rd *RegimeDetector) calculateVolatility(data []MarketDataPoint) float64 {
	if len(data) < 2 {
		return 0
	}

	returns := make([]float64, len(data)-1)
	for i := 1; i < len(data); i++ {
		returns[i-1] = (data[i].Price - data[i-1].Price) / data[i-1].Price
	}

	return standardDeviation(returns)
}

// detectVolumeSpike checks if volume is significantly above average
func (rd *RegimeDetector) detectVolumeSpike(data []MarketDataPoint) bool {
	if len(data) < 10 {
		return false
	}

	// Calculate average of previous period (excluding last point)
	avgVolume := 0.0
	for i := 0; i < len(data)-1; i++ {
		avgVolume += data[i].Volume
	}
	avgVolume /= float64(len(data) - 1)

	if avgVolume == 0 {
		return false
	}

	currentVolume := data[len(data)-1].Volume
	spikeRatio := currentVolume / avgVolume

	return spikeRatio > rd.volumeSpikeThreshold
}

// calculatePriceTrend computes price momentum
func (rd *RegimeDetector) calculatePriceTrend(data []MarketDataPoint) float64 {
	if len(data) < 2 {
		return 0
	}

	startPrice := data[0].Price
	endPrice := data[len(data)-1].Price

	return (endPrice - startPrice) / startPrice
}

// detectReversal identifies potential reversal patterns
func (rd *RegimeDetector) detectReversal(data []MarketDataPoint) bool {
	if len(data) < 20 {
		return false
	}

	// Simple reversal detection: strong move followed by opposing move
	firstHalf := data[:len(data)/2]
	secondHalf := data[len(data)/2:]

	firstTrend := rd.calculatePriceTrend(firstHalf)
	secondTrend := rd.calculatePriceTrend(secondHalf)

	// Reversal: opposite directions with significant magnitude
	return firstTrend*secondTrend < 0 &&
		math.Abs(firstTrend) > rd.priceChangeThreshold &&
		math.Abs(secondTrend) > rd.priceChangeThreshold*0.5
}

// standardDeviation calculates std dev
func standardDeviation(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}

	mean := 0.0
	for _, v := range data {
		mean += v
	}
	mean /= float64(len(data))

	variance := 0.0
	for _, v := range data {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(data))

	return math.Sqrt(variance)
}

// RealTimeAdapter manages sub-second adaptation
type RealTimeAdapter struct {
	detector       *RegimeDetector
	dataWindows    map[string][]MarketDataPoint
	currentRegime  map[string]RegimeType
	agentWeights   map[string]map[string]float64 // agent -> symbol -> weight adjustment
	lastUpdate     map[string]time.Time
	config         *RealTimeConfig
	params         *config.RealtimeParameters
	stopChan       chan struct{}
	mu             sync.RWMutex
	stopOnce       sync.Once
	onRegimeChange func(symbol string, oldRegime, newRegime RegimeType)
}

// RealTimeConfig configures the adapter (legacy)
type RealTimeConfig struct {
	UpdateInterval       time.Duration `json:"update_interval"`
	DataWindowSize       int           `json:"data_window_size"`
	MinConfidence        float64       `json:"min_confidence"`
	WeightAdjustmentRate float64       `json:"weight_adjustment_rate"`
	MaxWeightChange      float64       `json:"max_weight_change"`
	MinWeight            float64       `json:"min_weight"`
}

// DefaultRealTimeConfig returns standard configuration
func DefaultRealTimeConfig() *RealTimeConfig {
	return &RealTimeConfig{
		UpdateInterval:       100 * time.Millisecond,
		DataWindowSize:       60,
		MinConfidence:        0.7,
		WeightAdjustmentRate: 0.1,
		MaxWeightChange:      0.5,
		MinWeight:            0.1,
	}
}

// NewRealTimeAdapter creates an adapter with optional RealtimeParameters
func NewRealTimeAdapter(params *config.RealtimeParameters) *RealTimeAdapter {
	if params == nil {
		params = defaultRealtimeConfig()
	}

	cfg := &RealTimeConfig{
		UpdateInterval:       time.Duration(params.UpdateIntervalMs.Value) * time.Millisecond,
		DataWindowSize:       60,
		MinConfidence:        params.MinConfidence.Value,
		WeightAdjustmentRate: params.WeightAdjustmentRate.Value,
		MaxWeightChange:      params.MaxWeightChange.Value,
		MinWeight:            params.MinWeight.Value,
	}

	return &RealTimeAdapter{
		detector:      NewRegimeDetector(params),
		dataWindows:   make(map[string][]MarketDataPoint),
		currentRegime: make(map[string]RegimeType),
		agentWeights:  make(map[string]map[string]float64),
		lastUpdate:    make(map[string]time.Time),
		config:        cfg,
		params:        params,
		stopChan:      make(chan struct{}),
	}
}

// Start begins real-time monitoring
func (rta *RealTimeAdapter) Start(ctx context.Context) {
	ticker := time.NewTicker(rta.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rta.processUpdate()
		case <-ctx.Done():
			return
		case <-rta.stopChan:
			return
		}
	}
}

// Stop halts real-time monitoring. Safe to call multiple times (idempotent via sync.Once).
func (rta *RealTimeAdapter) Stop() {
	rta.stopOnce.Do(func() {
		close(rta.stopChan)
	})
}

// IngestData adds new market data point
func (rta *RealTimeAdapter) IngestData(point MarketDataPoint) {
	rta.mu.Lock()
	defer rta.mu.Unlock()

	window := rta.dataWindows[point.Symbol]

	// Add new point
	window = append(window, point)

	// Maintain window size
	if len(window) > rta.config.DataWindowSize {
		window = window[len(window)-rta.config.DataWindowSize:]
	}

	rta.dataWindows[point.Symbol] = window
	rta.lastUpdate[point.Symbol] = time.Now()
}

// processUpdate performs regime detection and adaptation
func (rta *RealTimeAdapter) processUpdate() {
	rta.mu.Lock()
	defer rta.mu.Unlock()

	for symbol, window := range rta.dataWindows {
		// Skip if not enough data
		if len(window) < rta.config.DataWindowSize/2 {
			continue
		}

		// Detect regime
		newRegime := rta.detector.DetectRegime(window)
		oldRegime := rta.currentRegime[symbol]

		// Check if regime changed
		if newRegime != oldRegime {
			// Notify callback
			if rta.onRegimeChange != nil {
				rta.onRegimeChange(symbol, oldRegime, newRegime)
			}

			// Adapt agent weights
			rta.adaptToRegime(symbol, newRegime)

			// Update current regime
			rta.currentRegime[symbol] = newRegime
		}
	}
}

// adaptToRegime adjusts agent weights based on regime
func (rta *RealTimeAdapter) adaptToRegime(symbol string, regime RegimeType) {
	// Get base adjustments for regime
	adjustments := rta.getRegimeAdjustments(regime)

	// Apply to all agents tracking this symbol
	for agentID, weights := range rta.agentWeights {
		if _, tracking := weights[symbol]; tracking {
			// Calculate new weight
			currentWeight := weights[symbol]
			adjustment := adjustments[agentID]

			newWeight := currentWeight + adjustment*rta.config.WeightAdjustmentRate

			// Clamp to max change
			change := newWeight - currentWeight
			if math.Abs(change) > rta.config.MaxWeightChange {
				if change > 0 {
					newWeight = currentWeight + rta.config.MaxWeightChange
				} else {
					newWeight = currentWeight - rta.config.MaxWeightChange
				}
			}

			if newWeight < rta.config.MinWeight {
				newWeight = rta.config.MinWeight
			}

			rta.agentWeights[agentID][symbol] = newWeight
		}
	}
}

// getRegimeAdjustments returns weight adjustments for each agent type in regime
func (rta *RealTimeAdapter) getRegimeAdjustments(regime RegimeType) map[string]float64 {
	adjustments := make(map[string]float64)

	switch regime {
	case RegimeCalm:
		// Favor value and fundamental agents
		adjustments["value_agent"] = 0.2
		adjustments["fundamental_agent"] = 0.2
		adjustments["momentum_agent"] = -0.1

	case RegimeVolatile:
		// Reduce all position sizes, favor risk management
		adjustments["risk_manager"] = 0.3
		adjustments["momentum_agent"] = 0.1
		adjustments["value_agent"] = -0.2

	case RegimeTrendingUp:
		// Favor momentum and trend agents
		adjustments["momentum_agent"] = 0.3
		adjustments["trend_following"] = 0.2
		adjustments["contrarian"] = -0.2

	case RegimeTrendingDown:
		// Favor defensive and hedging agents
		adjustments["defensive_agent"] = 0.3
		adjustments["hedge_agent"] = 0.2
		adjustments["growth_agent"] = -0.3

	case RegimeReversing:
		// Favor contrarian and mean-reversion agents
		adjustments["contrarian"] = 0.3
		adjustments["mean_reversion"] = 0.2
		adjustments["trend_following"] = -0.2

	case RegimeBreakout:
		// Favor momentum and volume-based agents
		adjustments["momentum_agent"] = 0.3
		adjustments["volume_analyst"] = 0.2

	case RegimeBreakdown:
		// Favor risk-off and defensive agents
		adjustments["risk_manager"] = 0.3
		adjustments["defensive_agent"] = 0.3
		adjustments["growth_agent"] = -0.3
	}

	return adjustments
}

// RegisterAgent adds an agent to real-time monitoring
func (rta *RealTimeAdapter) RegisterAgent(agentID string, symbols []string, baseWeight float64) {
	rta.mu.Lock()
	defer rta.mu.Unlock()

	if rta.agentWeights[agentID] == nil {
		rta.agentWeights[agentID] = make(map[string]float64)
	}

	for _, symbol := range symbols {
		rta.agentWeights[agentID][symbol] = baseWeight
	}
}

// UnregisterAgent removes an agent from monitoring
func (rta *RealTimeAdapter) UnregisterAgent(agentID string) {
	rta.mu.Lock()
	defer rta.mu.Unlock()

	delete(rta.agentWeights, agentID)
}

// GetAgentWeight returns current weight for agent-symbol pair
func (rta *RealTimeAdapter) GetAgentWeight(agentID, symbol string) float64 {
	rta.mu.RLock()
	defer rta.mu.RUnlock()

	if weights, exists := rta.agentWeights[agentID]; exists {
		if weight, exists := weights[symbol]; exists {
			return weight
		}
	}

	return 1.0 // Default
}

// GetCurrentRegime returns detected regime for symbol
func (rta *RealTimeAdapter) GetCurrentRegime(symbol string) RegimeType {
	rta.mu.RLock()
	defer rta.mu.RUnlock()

	if regime, exists := rta.currentRegime[symbol]; exists {
		return regime
	}

	return RegimeCalm
}

// GetRegimeConfidence returns confidence level for current regime
func (rta *RealTimeAdapter) GetRegimeConfidence(symbol string) float64 {
	rta.mu.RLock()
	defer rta.mu.RUnlock()

	window := rta.dataWindows[symbol]
	if len(window) < 10 {
		return 0.0
	}

	// Confidence based on data quality and consistency
	volatility := rta.detector.calculateVolatility(window)

	// Higher volatility = lower confidence (harder to predict)
	confidence := 1.0 - math.Min(1.0, volatility/rta.detector.volatilityThreshold)

	return confidence
}

// SetRegimeChangeCallback sets function to call on regime change
func (rta *RealTimeAdapter) SetRegimeChangeCallback(fn func(symbol string, oldRegime, newRegime RegimeType)) {
	rta.onRegimeChange = fn
}

// GetActiveSymbols returns all symbols being monitored
func (rta *RealTimeAdapter) GetActiveSymbols() []string {
	rta.mu.RLock()
	defer rta.mu.RUnlock()

	symbols := make([]string, 0, len(rta.dataWindows))
	for symbol := range rta.dataWindows {
		symbols = append(symbols, symbol)
	}

	return symbols
}

// GetStatistics returns real-time monitoring statistics
func (rta *RealTimeAdapter) GetStatistics() RealTimeStats {
	rta.mu.RLock()
	defer rta.mu.RUnlock()

	stats := RealTimeStats{
		MonitoredSymbols: len(rta.dataWindows),
		ActiveAgents:     len(rta.agentWeights),
		RegimeCounts:     make(map[RegimeType]int),
	}

	// Count regimes
	for _, regime := range rta.currentRegime {
		stats.RegimeCounts[regime]++
	}

	// Calculate average confidence
	totalConfidence := 0.0
	count := 0
	for symbol := range rta.dataWindows {
		confidence := rta.GetRegimeConfidence(symbol)
		totalConfidence += confidence
		count++
	}

	if count > 0 {
		stats.AverageConfidence = totalConfidence / float64(count)
	}

	return stats
}

// RealTimeStats captures monitoring statistics
type RealTimeStats struct {
	MonitoredSymbols  int                `json:"monitored_symbols"`
	ActiveAgents      int                `json:"active_agents"`
	RegimeCounts      map[RegimeType]int `json:"regime_counts"`
	AverageConfidence float64            `json:"average_confidence"`
	LastUpdate        time.Time          `json:"last_update"`
}

// GenerateReport creates comprehensive real-time analysis
func (rta *RealTimeAdapter) GenerateReport() *RealTimeReport {
	rta.mu.RLock()
	defer rta.mu.RUnlock()

	report := &RealTimeReport{
		GeneratedAt:    time.Now(),
		UpdateInterval: rta.config.UpdateInterval,
		SymbolReports:  make([]SymbolRealTimeReport, 0),
	}

	stats := rta.GetStatistics()
	report.Stats = stats

	// Generate per-symbol reports
	for symbol, window := range rta.dataWindows {
		if len(window) == 0 {
			continue
		}

		latest := window[len(window)-1]
		regime := rta.currentRegime[symbol]
		confidence := rta.GetRegimeConfidence(symbol)

		symbolReport := SymbolRealTimeReport{
			Symbol:        symbol,
			CurrentRegime: regime,
			Confidence:    confidence,
			LastPrice:     latest.Price,
			LastVolume:    latest.Volume,
			Spread:        latest.Spread,
			DataPoints:    len(window),
		}

		// Calculate additional metrics
		if len(window) >= 2 {
			symbolReport.PriceChange = (latest.Price - window[0].Price) / window[0].Price
			symbolReport.Volatility = rta.detector.calculateVolatility(window)
		}

		report.SymbolReports = append(report.SymbolReports, symbolReport)
	}

	return report
}

// RealTimeReport summarizes real-time monitoring
type RealTimeReport struct {
	GeneratedAt    time.Time              `json:"generated_at"`
	UpdateInterval time.Duration          `json:"update_interval"`
	Stats          RealTimeStats          `json:"stats"`
	SymbolReports  []SymbolRealTimeReport `json:"symbol_reports"`
}

// SymbolRealTimeReport details per-symbol status
type SymbolRealTimeReport struct {
	Symbol        string     `json:"symbol"`
	CurrentRegime RegimeType `json:"current_regime"`
	Confidence    float64    `json:"confidence"`
	LastPrice     float64    `json:"last_price"`
	LastVolume    float64    `json:"last_volume"`
	Spread        float64    `json:"spread"`
	PriceChange   float64    `json:"price_change"`
	Volatility    float64    `json:"volatility"`
	DataPoints    int        `json:"data_points"`
}

// ApplyToRecommendation modifies a recommendation with real-time adjustments
func (rta *RealTimeAdapter) ApplyToRecommendation(rec domain.Recommendation) domain.Recommendation {
	weight := rta.GetAgentWeight(rec.Agent, rec.Symbol)

	// Adjust conviction based on real-time weight
	adjustedConviction := max(min(int(float64(rec.Conviction)*weight), 100), 1)

	return domain.Recommendation{
		Agent:      rec.Agent,
		Skill:      rec.Skill,
		Symbol:     rec.Symbol,
		Side:       rec.Side,
		Conviction: adjustedConviction,
		Reason:     rec.Reason + fmt.Sprintf(" (RT weight: %.2f)", weight),
	}
}
