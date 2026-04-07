package portfolio

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// VolatilityManager manages portfolio volatility and implements smoothing strategies
type VolatilityManager struct {
	mu sync.RWMutex

	// Volatility parameters
	targetVolatility   float64 // Target annualized volatility (e.g., 0.15 for 15%)
	maxVolatility      float64 // Maximum allowed volatility
	smoothingFactor    float64 // EMA smoothing factor
	rebalanceThreshold float64 // Threshold for rebalancing

	// Current state
	currentVolatility float64
	volatilityHistory []VolatilityPoint
	assetWeights      map[string]float64
	lastRebalance     time.Time

	// Volatility components
	returnsHistory    map[string][]float64
	correlationMatrix map[string]map[string]float64
	betaValues        map[string]float64
}

// VolatilityPoint represents a volatility measurement at a point in time
type VolatilityPoint struct {
	Timestamp  time.Time
	Volatility float64
	Target     float64
	Deviation  float64
}

// VolatilityAdjustment represents a volatility adjustment recommendation
type VolatilityAdjustment struct {
	Asset     string
	Action    AdjustmentAction
	Magnitude float64
	Reason    string
	Priority  int
}

// AdjustmentAction represents the type of volatility adjustment
type AdjustmentAction int

const (
	ActionReduce AdjustmentAction = iota
	ActionIncrease
	ActionMaintain
	ActionRebalance
)

// NewVolatilityManager creates a new volatility manager
func NewVolatilityManager(targetVol, maxVol float64) *VolatilityManager {
	return &VolatilityManager{
		targetVolatility:   targetVol,
		maxVolatility:      maxVol,
		smoothingFactor:    0.3,
		rebalanceThreshold: 0.05, // 5% deviation threshold
		assetWeights:       make(map[string]float64),
		returnsHistory:     make(map[string][]float64),
		correlationMatrix:  make(map[string]map[string]float64),
		betaValues:         make(map[string]float64),
		lastRebalance:      time.Now(),
		volatilityHistory:  make([]VolatilityPoint, 0),
	}
}

// UpdateReturns updates the returns history for volatility calculation
func (vm *VolatilityManager) UpdateReturns(asset string, returns []float64) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	// Keep only recent returns (last 252 trading days)
	maxHistory := 252
	if len(returns) > maxHistory {
		returns = returns[len(returns)-maxHistory:]
	}

	vm.returnsHistory[asset] = returns

	// Update beta and correlation matrices
	vm.updateBetaValues(asset)
	vm.updateCorrelationMatrix(asset)
}

// updateBetaValues calculates beta values for assets
func (vm *VolatilityManager) updateBetaValues(asset string) {
	returns, exists := vm.returnsHistory[asset]
	if !exists || len(returns) < 30 {
		return
	}

	// Calculate beta relative to portfolio (simplified)
	// In practice, this would be calculated against a market benchmark
	beta := vm.calculateBeta(returns, vm.getPortfolioReturns())
	vm.betaValues[asset] = beta
}

// updateCorrelationMatrix updates correlation coefficients
func (vm *VolatilityManager) updateCorrelationMatrix(asset string) {
	if vm.correlationMatrix[asset] == nil {
		vm.correlationMatrix[asset] = make(map[string]float64)
	}

	assetReturns, exists := vm.returnsHistory[asset]
	if !exists || len(assetReturns) < 30 {
		return
	}

	// Calculate correlation with other assets
	for otherAsset, otherReturns := range vm.returnsHistory {
		if otherAsset == asset || len(otherReturns) < 30 {
			continue
		}

		correlation := vm.calculateCorrelation(assetReturns, otherReturns)
		vm.correlationMatrix[asset][otherAsset] = correlation
		vm.correlationMatrix[otherAsset][asset] = correlation
	}
}

// calculateBeta calculates beta coefficient
func (vm *VolatilityManager) calculateBeta(assetReturns, marketReturns []float64) float64 {
	if len(assetReturns) != len(marketReturns) || len(assetReturns) < 30 {
		return 1.0 // Default beta
	}

	// Calculate covariance and variance
	assetMean := vm.mean(assetReturns)
	marketMean := vm.mean(marketReturns)

	covariance := 0.0
	for i := range assetReturns {
		covariance += (assetReturns[i] - assetMean) * (marketReturns[i] - marketMean)
	}
	covariance /= float64(len(assetReturns) - 1)

	marketVariance := vm.variance(marketReturns)
	if marketVariance == 0 {
		return 1.0
	}

	return covariance / marketVariance
}

// calculateCorrelation calculates Pearson correlation coefficient
func (vm *VolatilityManager) calculateCorrelation(returns1, returns2 []float64) float64 {
	if len(returns1) != len(returns2) || len(returns1) < 30 {
		return 0.0
	}

	mean1 := vm.mean(returns1)
	mean2 := vm.mean(returns2)

	covariance := 0.0
	variance1 := 0.0
	variance2 := 0.0

	for i := range returns1 {
		diff1 := returns1[i] - mean1
		diff2 := returns2[i] - mean2

		covariance += diff1 * diff2
		variance1 += diff1 * diff1
		variance2 += diff2 * diff2
	}

	covariance /= float64(len(returns1) - 1)
	variance1 /= float64(len(returns1) - 1)
	variance2 /= float64(len(returns1) - 1)

	if variance1 == 0 || variance2 == 0 {
		return 0.0
	}

	return covariance / (math.Sqrt(variance1) * math.Sqrt(variance2))
}

// CalculateCurrentVolatility calculates current portfolio volatility
func (vm *VolatilityManager) CalculateCurrentVolatility(weights map[string]float64) float64 {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vm.assetWeights = weights

	// Calculate portfolio variance using the formula:
	// σ²p = Σ(wi² * σi²) + ΣΣ(wi * wj * σi * σj * ρij)

	portfolioVariance := 0.0

	// Individual asset contributions
	for asset := range weights {
		returns, exists := vm.returnsHistory[asset]
		if !exists || len(returns) < 30 {
			continue
		}

		assetVol := vm.standardDeviation(returns)
		weight := weights[asset]
		portfolioVariance += weight * weight * assetVol * assetVol
	}

	// Correlation contributions
	for asset1, weight1 := range weights {
		for asset2, weight2 := range weights {
			if asset1 >= asset2 {
				continue
			}

			correlation := vm.correlationMatrix[asset1][asset2]
			returns1, exists1 := vm.returnsHistory[asset1]
			returns2, exists2 := vm.returnsHistory[asset2]

			if !exists1 || !exists2 || len(returns1) < 30 || len(returns2) < 30 {
				continue
			}

			vol1 := vm.standardDeviation(returns1)
			vol2 := vm.standardDeviation(returns2)

			portfolioVariance += 2 * weight1 * weight2 * vol1 * vol2 * correlation
		}
	}

	currentVol := math.Sqrt(portfolioVariance)
	vm.currentVolatility = currentVol

	// Add to history
	point := VolatilityPoint{
		Timestamp:  time.Now(),
		Volatility: currentVol,
		Target:     vm.targetVolatility,
		Deviation:  (currentVol - vm.targetVolatility) / vm.targetVolatility,
	}

	vm.volatilityHistory = append(vm.volatilityHistory, point)
	if len(vm.volatilityHistory) > 1000 {
		vm.volatilityHistory = vm.volatilityHistory[1:]
	}

	return currentVol
}

// GetVolatilityAdjustments recommends adjustments to meet target volatility
func (vm *VolatilityManager) GetVolatilityAdjustments() []VolatilityAdjustment {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	adjustments := make([]VolatilityAdjustment, 0)

	// Check if volatility is too high
	if vm.currentVolatility > vm.maxVolatility {
		// Reduce exposure to high-volatility assets
		for asset := range vm.assetWeights {
			returns, exists := vm.returnsHistory[asset]
			if !exists || len(returns) < 30 {
				continue
			}

			assetVol := vm.standardDeviation(returns)
			if assetVol > vm.targetVolatility*1.5 {
				adjustment := VolatilityAdjustment{
					Asset:     asset,
					Action:    ActionReduce,
					Magnitude: 0.5, // Reduce by 50%
					Reason:    fmt.Sprintf("Asset volatility %.2f%% exceeds target", assetVol*100),
					Priority:  1,
				}
				adjustments = append(adjustments, adjustment)
			}
		}
	}

	// Check if volatility is too low
	if vm.currentVolatility < vm.targetVolatility*0.7 {
		// Increase exposure to low-volatility, high-return assets
		for asset := range vm.assetWeights {
			returns, exists := vm.returnsHistory[asset]
			if !exists || len(returns) < 30 {
				continue
			}

			assetVol := vm.standardDeviation(returns)
			assetReturn := vm.mean(returns)

			// Look for assets with low volatility but positive returns
			if assetVol < vm.targetVolatility && assetReturn > 0 {
				adjustment := VolatilityAdjustment{
					Asset:     asset,
					Action:    ActionIncrease,
					Magnitude: 0.2, // Increase by 20%
					Reason:    fmt.Sprintf("Low volatility asset with positive return: %.2f%%", assetReturn*100),
					Priority:  2,
				}
				adjustments = append(adjustments, adjustment)
			}
		}
	}

	// Check for rebalancing needs
	if time.Since(vm.lastRebalance) > 7*24*time.Hour { // Weekly rebalancing
		_ = struct{}{} // Use the rebalancing logic
		adjustment := VolatilityAdjustment{
			Asset:     "portfolio",
			Action:    ActionRebalance,
			Magnitude: 1.0,
			Reason:    "Scheduled weekly rebalancing",
			Priority:  3,
		}
		adjustments = append(adjustments, adjustment)
	}

	// Sort by priority
	sort.Slice(adjustments, func(i, j int) bool {
		return adjustments[i].Priority < adjustments[j].Priority
	})

	return adjustments
}

// ApplySmoothing applies volatility smoothing to returns
func (vm *VolatilityManager) ApplySmoothing(returns []float64) []float64 {
	if len(returns) < 2 {
		return returns
	}

	smoothed := make([]float64, len(returns))
	smoothed[0] = returns[0]

	// Apply EMA smoothing
	for i := 1; i < len(returns); i++ {
		smoothed[i] = vm.smoothingFactor*returns[i] + (1-vm.smoothingFactor)*smoothed[i-1]
	}

	return smoothed
}

// GetVolatilityForecast forecasts future volatility using GARCH(1,1) model
func (vm *VolatilityManager) GetVolatilityForecast(asset string, periods int) []float64 {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	returns, exists := vm.returnsHistory[asset]
	if !exists || len(returns) < 100 {
		// Return simple historical volatility forecast
		baseVol := vm.currentVolatility
		forecast := make([]float64, periods)
		for i := range forecast {
			forecast[i] = baseVol
		}
		return forecast
	}

	// Simplified GARCH(1,1) parameters
	omega := 0.000001
	alpha := 0.1
	beta := 0.85

	// Calculate initial variance
	variance := vm.variance(returns)
	forecast := make([]float64, periods)

	for i := 0; i < periods; i++ {
		// GARCH(1,1) formula: σ²t = ω + α * ε²t-1 + β * σ²t-1
		if i == 0 {
			forecast[i] = math.Sqrt(variance)
		} else {
			prevVariance := forecast[i-1] * forecast[i-1]
			newVariance := omega + alpha*prevVariance + beta*prevVariance
			forecast[i] = math.Sqrt(newVariance)
		}
	}

	return forecast
}

// Helper functions
func (vm *VolatilityManager) mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func (vm *VolatilityManager) variance(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := vm.mean(values)
	sum := 0.0
	for _, v := range values {
		sum += (v - mean) * (v - mean)
	}
	return sum / float64(len(values)-1)
}

func (vm *VolatilityManager) standardDeviation(values []float64) float64 {
	return math.Sqrt(vm.variance(values))
}

func (vm *VolatilityManager) getPortfolioReturns() []float64 {
	// Simplified portfolio returns calculation
	// In practice, this would be weighted average of all asset returns
	allReturns := make([]float64, 0)
	for _, returns := range vm.returnsHistory {
		allReturns = append(allReturns, returns...)
	}
	return allReturns
}

// GetVolatilityMetrics returns current volatility metrics
func (vm *VolatilityManager) GetVolatilityMetrics() VolatilityMetrics {
	vm.mu.RLock()
	defer vm.mu.RUnlock()

	return VolatilityMetrics{
		CurrentVolatility: vm.currentVolatility,
		TargetVolatility:  vm.targetVolatility,
		MaxVolatility:     vm.maxVolatility,
		Deviation:         (vm.currentVolatility - vm.targetVolatility) / vm.targetVolatility,
		LastRebalance:     vm.lastRebalance,
		AssetCount:        len(vm.assetWeights),
		HistoryLength:     len(vm.volatilityHistory),
	}
}

// VolatilityMetrics represents current volatility metrics
type VolatilityMetrics struct {
	CurrentVolatility float64
	TargetVolatility  float64
	MaxVolatility     float64
	Deviation         float64
	LastRebalance     time.Time
	AssetCount        int
	HistoryLength     int
}
