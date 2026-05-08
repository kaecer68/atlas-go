package sim

import (
	"slices"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// SlippageModel calculates dynamic slippage based on liquidity.
// Tiers:
//   - Top 20% volume: 0.05% (large caps)
//   - Middle 60%: 0.15% (mid caps)
//   - Bottom 20%: 0.50% (small caps)
type SlippageModel struct {
	// TierBPS defines slippage in basis points for each liquidity tier.
	TierBPS []float64

	// precomputed data for fast percentile lookup (nil if Precompute not called)
	sortedVolumes  []int64
	tierBoundaries [2]int // indices into sortedVolumes: [20th percentile index, 80th percentile index]
	precomputedLen int    // len of quotes when precomputed (to detect stale cache)
}

// Precompute caches sorted volume tiers for fast slippage calculation.
// Call this once per trading day/run before calling CalculateSlippageBPS repeatedly.
func (sm *SlippageModel) Precompute(quotes map[string]domain.Quote) {
	if sm == nil {
		return
	}

	sm.sortedVolumes = make([]int64, 0, len(quotes))
	for _, q := range quotes {
		if q.Volume > 0 {
			sm.sortedVolumes = append(sm.sortedVolumes, q.Volume)
		}
	}

	if len(sm.sortedVolumes) == 0 {
		sm.precomputedLen = 0
		return
	}

	slices.Sort(sm.sortedVolumes)

	sm.precomputedLen = len(quotes)

	// Calculate tier boundaries: 20th and 80th percentile indices
	n := len(sm.sortedVolumes)
	sm.tierBoundaries[0] = int(float64(n) * 0.20) // index for bottom 20% / middle 60% boundary
	sm.tierBoundaries[1] = int(float64(n) * 0.80) // index for middle 60% / top 20% boundary
}

// DefaultSlippageModel returns a model with standard TW market tiers.
func DefaultSlippageModel() *SlippageModel {
	return &SlippageModel{
		TierBPS: []float64{5, 15, 50}, // 0.05%, 0.15%, 0.50%
	}
}

// CalculateSlippageBPS returns the slippage in BPS for a given symbol
// based on its volume percentile relative to all available quotes.
// Uses precomputed data if Precompute was called, otherwise falls back to sorting.
func (sm *SlippageModel) CalculateSlippageBPS(symbol string, quotes map[string]domain.Quote) float64 {
	if sm == nil || len(sm.TierBPS) == 0 {
		return 15 // default mid-tier if no model configured
	}

	quote, ok := quotes[symbol]
	if !ok {
		return sm.TierBPS[len(sm.TierBPS)-1] // most conservative
	}

	if quote.Volume <= 0 {
		return sm.TierBPS[len(sm.TierBPS)-1]
	}

	var percentile float64
	if sm.precomputedLen == len(quotes) && sm.precomputedLen > 0 {
		percentile = sm.volumePercentilePrecomputed(quote.Volume)
	} else {
		percentile = calculateVolumePercentile(quote.Volume, quotes)
	}

	switch {
	case percentile >= 0.80:
		return sm.TierBPS[0] // top 20% = most liquid
	case percentile >= 0.20:
		return sm.TierBPS[1] // middle 60%
	default:
		return sm.TierBPS[2] // bottom 20% = least liquid
	}
}

// volumePercentilePrecomputed returns the percentile rank using precomputed sorted volumes.
// Requires Precompute to have been called with matching quote count.
func (sm *SlippageModel) volumePercentilePrecomputed(volume int64) float64 {
	if volume <= 0 || len(sm.sortedVolumes) == 0 {
		return 0
	}

	countBelow := 0
	for _, v := range sm.sortedVolumes {
		if v < volume {
			countBelow++
		}
	}

	return float64(countBelow) / float64(len(sm.sortedVolumes))
}

// calculateVolumePercentile computes the percentile rank of a symbol's volume
// relative to all volumes in the quote map.
func calculateVolumePercentile(volume int64, quotes map[string]domain.Quote) float64 {
	if volume <= 0 {
		return 0
	}

	volumes := make([]int64, 0, len(quotes))
	for _, q := range quotes {
		if q.Volume > 0 {
			volumes = append(volumes, q.Volume)
		}
	}

	if len(volumes) == 0 {
		return 0.5
	}

	slices.Sort(volumes)

	// Find rank
	countBelow := 0
	for _, v := range volumes {
		if v < volume {
			countBelow++
		}
	}

	return float64(countBelow) / float64(len(volumes))
}

// AdjustPriceForSlippage applies slippage-adjusted price for a trade.
// For buys: price goes up (worse fill)
// For sells: price goes down (worse fill)
func AdjustPriceForSlippage(price float64, slippageBPS float64, side domain.Side) float64 {
	if side == domain.SideBuy {
		return price * (1 + slippageBPS/10000.0)
	}
	return price * (1 - slippageBPS/10000.0)
}
