package portfolio

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// CalculateMomentumScore computes momentum based on price change over the configured lookback period.
// Falls back to intraday return when no historical data is available.
func (fe *FactorEngine) CalculateMomentumScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateMomentumDetail(context.Background(), symbol, quotes).Score
}

// calculateMomentumDetail returns the full breakdown for momentum calculation.
// Path priority:
//  1. HistoricalPrices available → call ensureAdjusted (idempotency TTL),
//     use hp.MomentumReturn(lookbackDays). Score = ret / StdDevDivisor,
//     clamped to [-1, 1]. Non-fallback.
//  2. Historical data absent or zero return → fall back to intraday return
//     using quotes[symbol] Open→Last. IsFallback=true.
//  3. Quote missing or Open==0 → fallback score 0.0. IsFallback=true.
func (fe *FactorEngine) calculateMomentumDetail(ctx context.Context, symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	fe.mu.RLock()
	hp := fe.history
	fe.mu.RUnlock()

	if hp != nil {
		if err := fe.ensureAdjusted(ctx, symbol); err != nil {
			logging.Warn("factor_engine", "ensure_adjusted_failed", logging.Symbol(symbol), logging.Err(err))
		}
		ret := hp.MomentumReturn(symbol, fe.params.Factor.MomentumLookbackDays)
		if ret != 0 {
			score := ret / fe.params.Factor.MomentumStdDevDivisor
			if score > 1.0 {
				score = 1.0
			}
			if score < -1.0 {
				score = -1.0
			}
			return domain.FactorScoreItem{
				Score:     score,
				Formula:   fmt.Sprintf("clamp(ret%d / %.2f, -1, 1)", fe.params.Factor.MomentumLookbackDays, fe.params.Factor.MomentumStdDevDivisor),
				RawInputs: map[string]float64{fmt.Sprintf("ret%d", fe.params.Factor.MomentumLookbackDays): ret},
			}
		}
	}

	quote, ok := quotes[symbol]
	if !ok || quote.Open == 0 {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    fmt.Sprintf("clamp(intraday / %.2f * %.1f, -1, 1)", fe.params.Factor.MomentumIntradayThreshold, fe.params.Factor.MomentumIntradayDiscount),
			RawInputs:  map[string]float64{"open": 0, "last": 0},
			IsFallback: true,
		}
	}
	intradayReturn := (quote.Last - quote.Open) / quote.Open
	score := intradayReturn / fe.params.Factor.MomentumIntradayThreshold * fe.params.Factor.MomentumIntradayDiscount
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:      score,
		Formula:    fmt.Sprintf("clamp(intraday / %.2f * %.1f, -1, 1)", fe.params.Factor.MomentumIntradayThreshold, fe.params.Factor.MomentumIntradayDiscount),
		RawInputs:  map[string]float64{"open": quote.Open, "last": quote.Last, "intraday_return": intradayReturn},
		IsFallback: true,
	}
}
