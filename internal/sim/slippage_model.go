package sim

import (
	"math"
	"slices"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
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
// If fallbackEvents is non-nil, descriptive fallback strings are appended when
// the model takes a non-standard code path.
func (sm *SlippageModel) CalculateSlippageBPS(symbol string, quotes map[string]domain.Quote, fallbackEvents *[]string) float64 {
	if sm == nil || len(sm.TierBPS) == 0 {
		if fallbackEvents != nil {
			*fallbackEvents = append(*fallbackEvents, "slippage: nil model, using 15 BPS default")
		}
		logging.Warn("sim", "slippage_fallback", "reason", "nil_model", "default_bps", 15)
		return 15 // default mid-tier if no model configured
	}

	quote, ok := quotes[symbol]
	if !ok {
		if fallbackEvents != nil {
			*fallbackEvents = append(*fallbackEvents, "slippage: missing quote for symbol, using conservative tier")
		}
		logging.Warn("sim", "slippage_fallback", "reason", "missing_quote", "symbol", symbol, "fallback_bps", sm.TierBPS[len(sm.TierBPS)-1])
		return sm.TierBPS[len(sm.TierBPS)-1] // most conservative
	}

	if quote.Volume <= 0 {
		if fallbackEvents != nil {
			*fallbackEvents = append(*fallbackEvents, "slippage: zero/negative volume for symbol, using conservative tier")
		}
		logging.Warn("sim", "slippage_fallback", "reason", "zero_volume", "symbol", symbol, "fallback_bps", sm.TierBPS[len(sm.TierBPS)-1])
		return sm.TierBPS[len(sm.TierBPS)-1]
	}

	var percentile float64
	if sm.precomputedLen == len(quotes) && sm.precomputedLen > 0 {
		percentile = sm.volumePercentilePrecomputed(quote.Volume, fallbackEvents)
	} else {
		percentile = calculateVolumePercentile(quote.Volume, quotes, fallbackEvents)
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
func (sm *SlippageModel) volumePercentilePrecomputed(volume int64, fallbackEvents *[]string) float64 {
	if volume <= 0 || len(sm.sortedVolumes) == 0 {
		if fallbackEvents != nil {
			*fallbackEvents = append(*fallbackEvents, "slippage: invalid volume or empty precomputed data, using 0 percentile")
		}
		logging.Warn("sim", "volume_percentile_fallback", "reason", "zero_volume", "fallback", 0)
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
func calculateVolumePercentile(volume int64, quotes map[string]domain.Quote, fallbackEvents *[]string) float64 {
	if volume <= 0 {
		if fallbackEvents != nil {
			*fallbackEvents = append(*fallbackEvents, "slippage: invalid volume, using 0 percentile")
		}
		logging.Warn("sim", "volume_percentile_fallback", "reason", "zero_volume", "fallback", 0)
		return 0
	}

	volumes := make([]int64, 0, len(quotes))
	for _, q := range quotes {
		if q.Volume > 0 {
			volumes = append(volumes, q.Volume)
		}
	}

	if len(volumes) == 0 {
		if fallbackEvents != nil {
			*fallbackEvents = append(*fallbackEvents, "slippage: no positive volumes in market, using 0.5 percentile")
		}
		logging.Warn("sim", "volume_percentile_fallback", "reason", "no_volumes", "fallback", 0.5)
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

// --------------------------------------------------------------------------
// MarketImpactModel — Almgren-Chriss style market impact estimation
// --------------------------------------------------------------------------

// MarketImpactModel estimates temporary and permanent price impact
// based on order size relative to average daily volume (ADV).
// Uses a simplified square-root model: impact ∝ σ * √(size/ADV)
type MarketImpactModel struct {
	// TemporaryImpactCoef scales temporary impact (default: 0.5)
	TemporaryImpactCoef float64
	// PermanentImpactCoef scales permanent impact (default: 0.1)
	PermanentImpactCoef float64
	// DefaultADV used when volume data is unavailable (default: 1,000,000)
	DefaultADV int64
}

// ImpactResult holds the estimated market impact for a trade.
type ImpactResult struct {
	TemporaryImpactBPS float64 `json:"temporary_impact_bps"`
	PermanentImpactBPS float64 `json:"permanent_impact_bps"`
	TotalImpactBPS     float64 `json:"total_impact_bps"`
	AdvRatio           float64 `json:"adv_ratio"`
}

// DefaultMarketImpactModel returns a model calibrated for TWSE stocks.
func DefaultMarketImpactModel() *MarketImpactModel {
	return &MarketImpactModel{
		TemporaryImpactCoef: 0.5,
		PermanentImpactCoef: 0.1,
		DefaultADV:          1_000_000,
	}
}

// Estimate calculates market impact for an order of given notional size.
// adv: the stock's average daily volume in shares
// price: current stock price
// vol_estimate: estimated daily volatility as decimal (e.g. 0.02 for 2%)
func (m *MarketImpactModel) Estimate(orderNotional float64, adv int64, price float64, volEstimate float64) ImpactResult {
	if m == nil {
		return ImpactResult{}
	}
	if adv <= 0 {
		adv = m.DefaultADV
	}
	if price <= 0 {
		price = 100
	}
	if volEstimate <= 0 {
		volEstimate = 0.02
	}

	shares := orderNotional / price
	if shares <= 0 {
		shares = 1
	}

	advRatio := float64(shares) / float64(adv)
	sqrtRatio := math.Sqrt(advRatio)

	tempBPS := m.TemporaryImpactCoef * volEstimate * 10000 * sqrtRatio
	permBPS := m.PermanentImpactCoef * volEstimate * 10000 * sqrtRatio

	return ImpactResult{
		TemporaryImpactBPS: tempBPS,
		PermanentImpactBPS: permBPS,
		TotalImpactBPS:     tempBPS + permBPS,
		AdvRatio:           advRatio,
	}
}

// --------------------------------------------------------------------------
// CostBreakdown — all-in transaction cost breakdown
// --------------------------------------------------------------------------

// CostBreakdown decomposes total transaction cost into its components.
type CostBreakdown struct {
	SlippageBPS   float64 `json:"slippage_bps"`
	ImpactBPS     float64 `json:"impact_bps"`
	CommissionBPS float64 `json:"commission_bps"`
	TotalBPS      float64 `json:"total_bps"`
	TotalPct      float64 `json:"total_pct"`
	TotalCost     float64 `json:"total_cost"`
}

// DefaultCommissionBPS is the standard TWSE broker commission rate (0.1425%).
const DefaultCommissionBPS = 14.25

// CalculateCosts computes the all-in transaction cost for an order.
func CalculateCosts(orderNotional float64, slippageBPS float64, impact ImpactResult, commissionBPS float64) CostBreakdown {
	if commissionBPS <= 0 {
		commissionBPS = DefaultCommissionBPS
	}
	totalBPS := slippageBPS + impact.TotalImpactBPS + commissionBPS
	return CostBreakdown{
		SlippageBPS:   slippageBPS,
		ImpactBPS:     impact.TotalImpactBPS,
		CommissionBPS: commissionBPS,
		TotalBPS:      totalBPS,
		TotalPct:      totalBPS / 100.0,
		TotalCost:     orderNotional * totalBPS / 10000.0,
	}
}
