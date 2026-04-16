package portfolio

import (
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// FactorEngine calculates multi-factor scores for individual symbols.
// It is extracted from Optimizer so that screening layers can reuse
// the same momentum, value, and quality calculations.
type FactorEngine struct {
	history      *HistoricalPrices
	fundamentals *FundamentalProvider
	mu           sync.RWMutex
}

// NewFactorEngine creates an empty factor engine.
func NewFactorEngine() *FactorEngine {
	return &FactorEngine{}
}

// WithHistoricalPrices attaches a historical price repository for momentum calc.
func (fe *FactorEngine) WithHistoricalPrices(hp *HistoricalPrices) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.history = hp
	return fe
}

// WithFundamentalProvider attaches a fundamental data provider.
func (fe *FactorEngine) WithFundamentalProvider(fp *FundamentalProvider) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.fundamentals = fp
	return fe
}

// CalculateMomentumScore computes momentum based on 20-day price change.
// Falls back to intraday return when no historical data is available.
func (fe *FactorEngine) CalculateMomentumScore(symbol string, quotes map[string]domain.Quote) float64 {
	fe.mu.RLock()
	hp := fe.history
	fe.mu.RUnlock()

	if hp != nil {
		ret20 := hp.MomentumReturn(symbol, 20)
		if ret20 != 0 {
			// Normalize: assume ±30% over 20 days maps to ±1.0
			score := ret20 / 0.30
			if score > 1.0 {
				score = 1.0
			}
			if score < -1.0 {
				score = -1.0
			}
			return score
		}
	}

	// Fallback to intraday momentum proxy
	quote, ok := quotes[symbol]
	if !ok || quote.Open == 0 {
		return 0.0
	}
	intradayReturn := (quote.Last - quote.Open) / quote.Open
	score := intradayReturn / 0.10
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return score
}

// CalculateValueScore computes value based on P/E and P/B from fundamentals.
// Falls back to a mild positive constant when no data is available.
func (fe *FactorEngine) CalculateValueScore(symbol string, quotes map[string]domain.Quote) float64 {
	_ = quotes
	fe.mu.RLock()
	fp := fe.fundamentals
	fe.mu.RUnlock()

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		score := 0.0
		count := 0
		if data.PE > 0 {
			// Lower PE is better. Map PE 5->1.0, PE 50->-1.0 linearly
			peScore := 1.0 - (data.PE-5)/45.0
			if peScore > 1.0 {
				peScore = 1.0
			}
			if peScore < -1.0 {
				peScore = -1.0
			}
			score += peScore
			count++
		}
		if data.PB > 0 {
			pbScore := 1.0 - (data.PB-0.5)/4.5
			if pbScore > 1.0 {
				pbScore = 1.0
			}
			if pbScore < -1.0 {
				pbScore = -1.0
			}
			score += pbScore
			count++
		}
		if count > 0 {
			return score / float64(count)
		}
	}
	return 0.1 // fallback placeholder
}

// CalculateQualityScore computes quality based on dividend yield and price stability.
// Falls back to a mild positive constant when no data is available.
func (fe *FactorEngine) CalculateQualityScore(symbol string, quotes map[string]domain.Quote) float64 {
	fe.mu.RLock()
	fp := fe.fundamentals
	hp := fe.history
	fe.mu.RUnlock()

	score := 0.0
	count := 0

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		if data.DividendYield > 0 {
			// Higher yield suggests stability. Map 0->0, 5%->1.0
			dyScore := data.DividendYield / 5.0
			if dyScore > 1.0 {
				dyScore = 1.0
			}
			score += dyScore
			count++
		}
	}

	if hp != nil {
		vol := hp.Volatility(symbol, 20)
		if vol > 0 {
			// Lower volatility = higher quality. Map 0->1.0, 5%->0 linearly
			volScore := 1.0 - vol/0.05
			if volScore > 1.0 {
				volScore = 1.0
			}
			if volScore < -1.0 {
				volScore = -1.0
			}
			score += volScore
			count++
		}
	}

	if count > 0 {
		return score / float64(count)
	}
	return 0.05 // fallback placeholder
}

// CalculateAllScores returns momentum, value, quality, and agent scores.
// The agent score is computed from the provided recommendations for the symbol.
func (fe *FactorEngine) CalculateAllScores(
	symbol string,
	quotes map[string]domain.Quote,
	agentRecs []domain.Recommendation,
	agentWeights map[string]float64,
	factorWeights map[FactorType]float64,
) map[FactorType]float64 {
	momentumScore := fe.CalculateMomentumScore(symbol, quotes)
	valueScore := fe.CalculateValueScore(symbol, quotes)
	qualityScore := fe.CalculateQualityScore(symbol, quotes)

	var agentScore float64
	var totalWeight float64
	for _, rec := range agentRecs {
		if rec.Symbol != symbol {
			continue
		}
		weight := 1.0
		if w, ok := agentWeights[rec.Agent]; ok {
			weight = w
		}
		agentScore += float64(rec.Conviction) * weight / 100.0
		totalWeight += weight
	}
	if totalWeight > 0 {
		agentScore /= totalWeight
	}

	result := map[FactorType]float64{
		FactorMomentum: momentumScore,
		FactorValue:    valueScore,
		FactorQuality:  qualityScore,
		FactorAgent:    agentScore,
	}

	// Compute weighted total if factorWeights provided
	if len(factorWeights) > 0 {
		total := 0.0
		for ft, score := range result {
			if w, ok := factorWeights[ft]; ok {
				total += score * w
			}
		}
		result["total"] = total
	}

	return result
}
