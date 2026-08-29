package strategy

import (
	"math"
	"sort"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// StrategyMix maps strategy IDs to target portfolio weights (Σw = 1).
type StrategyMix map[string]float64

// StrategyAllocator computes risk-parity capital allocation across strategies.
//
// Model (risk parity): wₖ ∝ 1/σₖ — each strategy contributes equal marginal risk.
// Weight caps: w_max = 0.50, w_min = 0.05 (configurable).
// Regime-filtered: only strategies matching the current regime participate.
// Uses shadow return tracking to estimate strategy-level volatility.
type StrategyAllocator struct {
	mu            sync.RWMutex
	registry      *Registry
	shadowReturns map[string][]float64
	maxWeight     float64
	minWeight     float64
	windowDays    int
}

// NewStrategyAllocator creates a risk-parity strategy allocator.
func NewStrategyAllocator(registry *Registry) *StrategyAllocator {
	return &StrategyAllocator{
		registry:      registry,
		shadowReturns: make(map[string][]float64),
		maxWeight:     0.50,
		minWeight:     0.05,
		windowDays:    60,
	}
}

// SetCaps overrides the default weight caps.
func (sa *StrategyAllocator) SetCaps(maxW, minW float64) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.maxWeight = maxW
	sa.minWeight = minW
}

// Allocate computes risk-parity strategy weights for the given regime.
//
// Steps:
//  1. Filter strategies by regime compatibility
//  2. Estimate volatility σₖ from shadow returns (or default 0.20)
//  3. Compute raw weights wₖ ∝ 1/σₖ
//  4. Apply caps (w_max, w_min) and re-normalize
func (sa *StrategyAllocator) Allocate(regime domain.Regime, vix float64) StrategyMix {
	_ = vix

	sa.mu.RLock()
	defer sa.mu.RUnlock()

	candidates := sa.registry.ListByRegime(regime)
	if len(candidates) == 0 {
		all := sa.registry.List()
		if len(all) == 0 {
			return StrategyMix{}
		}
		candidates = all
	}

	vols := make(map[string]float64, len(candidates))
	for _, s := range candidates {
		vols[s.ID] = sa.estimateVolatility(s.ID)
	}

	mix := sa.computeRiskParity(candidates, vols)
	return mix
}

func (sa *StrategyAllocator) estimateVolatility(strategyID string) float64 {
	returns := sa.shadowReturns[strategyID]
	if len(returns) < 5 {
		return 0.20 // default annualized volatility
	}

	n := len(returns)
	if n > sa.windowDays {
		returns = returns[n-sa.windowDays:]
		n = sa.windowDays
	}

	var sum, ssq float64
	for _, r := range returns {
		sum += r
		ssq += r * r
	}
	mean := sum / float64(n)
	variance := ssq/float64(n) - mean*mean
	if variance < 1e-15 {
		return 0.20
	}
	dailyVol := math.Sqrt(variance)
	annualVol := dailyVol * math.Sqrt(252)
	if annualVol < 0.05 {
		annualVol = 0.05
	}
	return annualVol
}

func (sa *StrategyAllocator) computeRiskParity(candidates []*Strategy, vols map[string]float64) StrategyMix {
	if len(candidates) == 0 {
		return StrategyMix{}
	}

	raw := make(map[string]float64, len(candidates))
	var totalRaw float64
	for _, s := range candidates {
		vol := vols[s.ID]
		w := 1.0 / vol
		raw[s.ID] = w
		totalRaw += w
	}

	if totalRaw < 1e-15 {
		return equalMix(candidates)
	}

	mix := make(StrategyMix, len(candidates))
	for id, rw := range raw {
		mix[id] = rw / totalRaw
	}

	mix = sa.applyCaps(mix, candidates)
	return mix
}

func (sa *StrategyAllocator) applyCaps(mix StrategyMix, candidates []*Strategy) StrategyMix {
	maxIter := 10
	for range maxIter {
		var excess float64
		capped := 0

		for _, s := range candidates {
			w := mix[s.ID]
			if w > sa.maxWeight {
				excess += w - sa.maxWeight
				mix[s.ID] = sa.maxWeight
				capped++
			}
			if w < sa.minWeight {
				excess += w - sa.minWeight
				mix[s.ID] = sa.minWeight
				capped++
			}
		}

		if capped == 0 {
			break
		}

		var uncappedTotal float64
		for _, s := range candidates {
			if mix[s.ID] > sa.minWeight && mix[s.ID] < sa.maxWeight {
				uncappedTotal += mix[s.ID]
			}
		}

		if uncappedTotal < 1e-15 {
			break
		}
		scale := (uncappedTotal + excess) / uncappedTotal
		for _, s := range candidates {
			if mix[s.ID] > sa.minWeight && mix[s.ID] < sa.maxWeight {
				mix[s.ID] *= scale
			}
		}
	}

	var total float64
	for _, w := range mix {
		total += w
	}
	if total > 0 && math.Abs(total-1.0) > 0.001 {
		for id := range mix {
			mix[id] /= total
		}
	}

	return mix
}

// UpdateShadowReturns records daily returns for each strategy.
// This feeds the volatility estimation used by Allocate.
func (sa *StrategyAllocator) UpdateShadowReturns(returns map[string]float64) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	for id, r := range returns {
		sa.shadowReturns[id] = append(sa.shadowReturns[id], r)

		if len(sa.shadowReturns[id]) > sa.windowDays*2 {
			sa.shadowReturns[id] = sa.shadowReturns[id][len(sa.shadowReturns[id])-sa.windowDays*2:]
		}
	}
}

// Volatilities returns the current estimated volatility for each strategy.
func (sa *StrategyAllocator) Volatilities() map[string]float64 {
	sa.mu.RLock()
	defer sa.mu.RUnlock()

	result := make(map[string]float64, len(sa.shadowReturns))
	for id := range sa.shadowReturns {
		result[id] = sa.estimateVolatility(id)
	}
	return result
}

func equalMix(candidates []*Strategy) StrategyMix {
	mix := make(StrategyMix, len(candidates))
	w := 1.0 / float64(len(candidates))
	for _, s := range candidates {
		mix[s.ID] = w
	}
	return mix
}

// Validate returns true if all weights are in [0,1] and sum to ~1.0.
func (mix StrategyMix) Validate() bool {
	if len(mix) == 0 {
		return true
	}
	var total float64
	ids := make([]string, 0, len(mix))
	for id, w := range mix {
		if w < 0 || w > 1 || math.IsNaN(w) || math.IsInf(w, 0) {
			return false
		}
		total += w
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return math.Abs(total-1.0) < 0.001
}
