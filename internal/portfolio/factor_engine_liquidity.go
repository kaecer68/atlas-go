package portfolio

import (
	"math"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CalculateLiquidityScore computes the Amihud ILLIQ proxy:
//
//	illiq = |return| / volume
//	score = -log(illiq + 1e-10)
//
// clamped to [-1, 1].
//
// Fallback (IsFallback=true, score=0): missing quote, Open==0, or Volume==0.
// Edge case (return=0): score=0, formula='-log(abs(0) / volume) = 0' (NOT a
// fallback — the formula reflects a known degenerate case).
func (fe *FactorEngine) CalculateLiquidityScore(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	quote, ok := quotes[symbol]
	if !ok || quote.Open == 0 || quote.Volume == 0 {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "-log(abs(return) / volume)",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	ret := (quote.Last - quote.Open) / quote.Open
	if ret == 0 {
		return domain.FactorScoreItem{
			Score:     0.0,
			Formula:   "-log(abs(0) / volume) = 0",
			RawInputs: map[string]float64{"return": 0, "volume": float64(quote.Volume)},
		}
	}
	illiq := math.Abs(ret) / float64(quote.Volume)
	score := -math.Log(illiq + 1e-10)
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:   score,
		Formula: "clamp(-log(abs(return)/volume), -1, 1)",
		RawInputs: map[string]float64{
			"return": ret,
			"volume": float64(quote.Volume),
			"illiq":  illiq,
		},
	}
}
