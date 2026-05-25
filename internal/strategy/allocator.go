package strategy

import (
	"math"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// StrategyAllocation represents capital allocation across multiple strategies.
type StrategyAllocation struct {
	Allocations map[string]float64 // strategy ID → weight (sum ≈ 1.0)
	Regime      domain.Regime
	Source      string // "config", "equal", "covariance"
}

// Allocator computes regime-based multi-strategy capital allocation.
//
// Unlike Selector (which picks ONE best strategy), Allocator splits
// capital across all eligible strategies using regime-specific base weights
// and optionally incorporates inter-strategy covariance optimization.
type Allocator struct {
	mu          sync.RWMutex
	registry    *Registry
	baseWeights map[domain.Regime]map[string]float64
}

// NewAllocator creates a strategy allocator with sensible regime-based defaults.
func NewAllocator(registry *Registry) *Allocator {
	return &Allocator{
		registry:    registry,
		baseWeights: defaultAllocationWeights(),
	}
}

func defaultAllocationWeights() map[domain.Regime]map[string]float64 {
	return map[domain.Regime]map[string]float64{
		domain.RegimeRiskOn: {
			"growth":      0.40,
			"momentum":    0.30,
			"all_weather": 0.30,
		},
		domain.RegimeRiskOff: {
			"defensive":   0.40,
			"all_weather": 0.35,
			"value":       0.25,
		},
		domain.RegimeNeutral: {
			"all_weather": 0.40,
			"growth":      0.25,
			"value":       0.20,
			"defensive":   0.15,
		},
	}
}

// SetBaseWeights overrides the default regime-based allocation weights.
func (a *Allocator) SetBaseWeights(weights map[domain.Regime]map[string]float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.baseWeights = weights
}

// Compute returns allocation weights for the given regime.
// Strategies not in the registry are excluded from the result.
// Weights are re-normalized to sum to 1.0 after filtering.
func (a *Allocator) Compute(regime domain.Regime) StrategyAllocation {
	a.mu.RLock()
	defer a.mu.RUnlock()

	bw := a.baseWeights[regime]
	if bw == nil {
		// Fallback to equal-weight across all enabled strategies.
		return a.equalWeight(regime)
	}

	result := make(map[string]float64)
	var total float64

	for id, w := range bw {
		if _, ok := a.registry.Get(id); !ok {
			continue
		}
		result[id] = w
		total += w
	}

	if total < 1e-15 {
		return a.equalWeight(regime)
	}

	// Re-normalize after filtering unregistered strategies.
	for id := range result {
		result[id] /= total
	}

	return StrategyAllocation{
		Allocations: result,
		Regime:      regime,
		Source:      "config",
	}
}

func (a *Allocator) equalWeight(regime domain.Regime) StrategyAllocation {
	strategies := a.registry.ListByRegime(regime)
	if len(strategies) == 0 {
		return StrategyAllocation{Regime: regime, Source: "equal"}
	}

	n := float64(len(strategies))
	result := make(map[string]float64, len(strategies))
	for _, s := range strategies {
		result[s.ID] = 1.0 / n
	}

	return StrategyAllocation{
		Allocations: result,
		Regime:      regime,
		Source:      "equal",
	}
}

// Allocations returns the ordered list of (strategy, weight) pairs for the given regime.
func (a *Allocator) Allocations(regime domain.Regime) []StrategyWeight {
	alloc := a.Compute(regime)
	result := make([]StrategyWeight, 0, len(alloc.Allocations))
	for id, w := range alloc.Allocations {
		result = append(result, StrategyWeight{ID: id, Weight: w})
	}
	return result
}

// StrategyWeight pairs a strategy ID with its allocation weight.
type StrategyWeight struct {
	ID     string
	Weight float64
}

// Validate checks that allocation weights are within [0,1] and sum to ~1.0.
func (sa StrategyAllocation) Validate() bool {
	if len(sa.Allocations) == 0 {
		return true
	}
	var total float64
	for _, w := range sa.Allocations {
		if w < 0 || w > 1 || math.IsNaN(w) || math.IsInf(w, 0) {
			return false
		}
		total += w
	}
	return math.Abs(total-1.0) < 0.001
}
