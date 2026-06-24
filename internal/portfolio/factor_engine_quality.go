package portfolio

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// CalculateQualityScore computes quality based on dividend yield and price stability.
// Falls back to a mild positive constant when no data is available.
func (fe *FactorEngine) CalculateQualityScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateQualityDetail(context.Background(), symbol).Score
}

// calculateQualityDetail returns the full breakdown for quality calculation.
// Precious metals (gold, silver) have no ROE/profit-margin — returns 0.
//
// Score = average of (DividendYield / QualityDividendYieldCap, capped at 1.0)
//
//	and (1 - VolatilityLookback / QualityVolatilityStd, clamped to [-1, 1])
//
// Only present metrics contribute to the average. Missing dividend yield or
// zero volatility skips that leg (count stays 0 for that leg).
//
// ensureAdjusted is called before volatility computation (same TTL cache as
// momentum — required for back-adjusted historical prices).
func (fe *FactorEngine) calculateQualityDetail(ctx context.Context, symbol string) domain.FactorScoreItem {
	if isPM, _ := isPreciousMetal(symbol); isPM {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "precious_metal: ROE not applicable",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	fe.mu.RLock()
	fp := fe.fundamentals
	hp := fe.history
	fe.mu.RUnlock()

	score := 0.0
	count := 0
	raw := map[string]float64{}

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		if data.DividendYield > 0 {
			dyScore := data.DividendYield / fe.params.Factor.QualityDividendYieldCap
			if dyScore > 1.0 {
				dyScore = 1.0
			}
			score += dyScore
			count++
			raw["dividend_yield"] = data.DividendYield
			raw["dividend_yield_score"] = dyScore
		}
	}

	if hp != nil {
		if err := fe.ensureAdjusted(ctx, symbol); err != nil {
			logging.Warn("factor_engine", "ensure_adjusted_failed", logging.Symbol(symbol), logging.Err(err))
		}
		vol := hp.Volatility(symbol, fe.params.Factor.MomentumLookbackDays)
		if vol > 0 {
			volScore := 1.0 - vol/fe.params.Factor.QualityVolatilityStd
			if volScore > 1.0 {
				volScore = 1.0
			}
			if volScore < -1.0 {
				volScore = -1.0
			}
			score += volScore
			count++
			raw[fmt.Sprintf("volatility_%dd", fe.params.Factor.MomentumLookbackDays)] = vol
			raw["volatility_score"] = volScore
		}
	}

	if count > 0 {
		return domain.FactorScoreItem{
			Score:     score / float64(count),
			Formula:   fmt.Sprintf("avg(DividendYield/%.2f, clamp(1 - Vol%dd/%.2f, -1, 1))", fe.params.Factor.QualityDividendYieldCap, fe.params.Factor.MomentumLookbackDays, fe.params.Factor.QualityVolatilityStd),
			RawInputs: raw,
		}
	}
	return domain.FactorScoreItem{
		Score:      fe.params.Factor.QualityFallbackScore,
		Formula:    fmt.Sprintf("avg(DividendYield/%.2f, clamp(1 - Vol%dd/%.2f, -1, 1))", fe.params.Factor.QualityDividendYieldCap, fe.params.Factor.MomentumLookbackDays, fe.params.Factor.QualityVolatilityStd),
		RawInputs:  map[string]float64{},
		IsFallback: true,
	}
}
