package orchestrator

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// factorConfig holds all factor-score thresholds and conviction deltas loaded
// from ParametersConfig (SectorExecutor.FactorConviction) with hardcoded fallback
// values. All executors share this single config source.
type factorConfig struct {
	momHigh, momMod, momWeak    float64
	momHighD, momModD, momWeakD int
	valHigh, valMod, valWeak    float64
	valHighD, valModD, valWeakD int
	qualThresh                  float64
	qualDelta                   int
	liqHigh, liqGood, liqLow    float64
	liqHighD, liqGoodD, liqLowD int
}

// loadFactorConfig reads factor thresholds from ParametersConfig with hardcoded
// fallback. Returns a populated factorConfig ready for executors to use.
func loadFactorConfig() factorConfig {
	fc := factorConfig{
		momHigh: 0.4, momHighD: 8, momMod: 0.15, momModD: 4, momWeak: -0.1, momWeakD: -6,
		valHigh: 0.3, valHighD: 8, valMod: 0.1, valModD: 4, valWeak: -0.2, valWeakD: -5,
		qualThresh: 0.2, qualDelta: 4,
		liqHigh: 0.5, liqHighD: 5, liqGood: 0.2, liqGoodD: 3, liqLow: -0.3, liqLowD: -5,
	}
	if cfg := config.GetParametersConfig(); cfg != nil {
		p := cfg.SectorExecutor.FactorConviction
		if p.MomentumHighThreshold.Value != 0 {
			fc.momHigh, fc.momHighD = p.MomentumHighThreshold.Value, p.MomentumHighDelta.Value
			fc.momMod, fc.momModD = p.MomentumModThreshold.Value, p.MomentumModDelta.Value
			fc.momWeak, fc.momWeakD = p.MomentumWeakThreshold.Value, p.MomentumWeakDelta.Value
			fc.valHigh, fc.valHighD = p.ValueHighThreshold.Value, p.ValueHighDelta.Value
			fc.valMod, fc.valModD = p.ValueModThreshold.Value, p.ValueModDelta.Value
			fc.valWeak, fc.valWeakD = p.ValueWeakThreshold.Value, p.ValueWeakDelta.Value
			fc.qualThresh, fc.qualDelta = p.QualityThreshold.Value, p.QualityDelta.Value
			fc.liqHigh, fc.liqHighD = p.LiquidityHighThreshold.Value, p.LiquidityHighDelta.Value
			fc.liqGood, fc.liqGoodD = p.LiquidityGoodThreshold.Value, p.LiquidityGoodDelta.Value
			fc.liqLow, fc.liqLowD = p.LiquidityLowThreshold.Value, p.LiquidityLowDelta.Value
		}
	}
	return fc
}

// addMomentumAdjustment checks factor momentum score and adds conviction delta.
func addMomentumAdjustment(b *convictionBuilder, fq FactorQuery, symbol string, fc factorConfig) {
	if mom, ok := fq.GetScore(symbol, portfolio.FactorMomentum); ok {
		switch {
		case mom > fc.momHigh:
			b.add("factor_momentum_strong", fc.momHighD, fmt.Sprintf("momentum > %.2f", fc.momHigh))
		case mom > fc.momMod:
			b.add("factor_momentum_positive", fc.momModD, fmt.Sprintf("momentum > %.2f", fc.momMod))
		case mom < fc.momWeak:
			b.add("factor_momentum_weak", fc.momWeakD, fmt.Sprintf("momentum < %.2f", fc.momWeak))
		}
	}
}

// addValueAdjustment checks factor value score and adds conviction delta.
func addValueAdjustment(b *convictionBuilder, fq FactorQuery, symbol string, fc factorConfig) {
	if val, ok := fq.GetScore(symbol, portfolio.FactorValue); ok {
		switch {
		case val > fc.valHigh:
			b.add("factor_value_strong", fc.valHighD, fmt.Sprintf("value > %.2f", fc.valHigh))
		case val > fc.valMod:
			b.add("factor_value_positive", fc.valModD, fmt.Sprintf("value > %.2f", fc.valMod))
		case val < fc.valWeak:
			b.add("factor_value_weak", fc.valWeakD, fmt.Sprintf("value < %.2f", fc.valWeak))
		}
	}
}

// addQualityAdjustment checks factor quality score and adds conviction delta.
func addQualityAdjustment(b *convictionBuilder, fq FactorQuery, symbol string, fc factorConfig) {
	if qly, ok := fq.GetScore(symbol, portfolio.FactorQuality); ok && qly > fc.qualThresh {
		b.add("factor_quality_boost", fc.qualDelta, fmt.Sprintf("quality > %.2f", fc.qualThresh))
	}
}

// addLiquidityAdjustment checks factor liquidity score and adds conviction delta.
func addLiquidityAdjustment(b *convictionBuilder, fq FactorQuery, symbol string, fc factorConfig) {
	if liq, ok := fq.GetScore(symbol, portfolio.FactorLiquidity); ok {
		switch {
		case liq > fc.liqHigh:
			b.add("factor_liquidity_high", fc.liqHighD, fmt.Sprintf("liquidity > %.2f", fc.liqHigh))
		case liq > fc.liqGood:
			b.add("factor_liquidity_good", fc.liqGoodD, fmt.Sprintf("liquidity > %.2f", fc.liqGood))
		case liq < fc.liqLow:
			b.add("factor_liquidity_low", fc.liqLowD, fmt.Sprintf("liquidity < %.2f", fc.liqLow))
		}
	}
}

// FactorQuery provides read-only access to pre-computed factor scores for a symbol.
// Executors use this in their Recommend() methods to access factor scores without
// re-calculating them per symbol.
type FactorQuery interface {
	// GetScore returns the pre-computed score for the given symbol and factor.
	// Returns (0, false) when the symbol or factor is not available.
	GetScore(symbol string, factor portfolio.FactorType) (float64, bool)
}

// FactorSnapshot holds pre-computed factor scores for all symbols in a session.
// Scores are calculated once per collectRecommendations() call and made available
// to executors via ExecutionContext.FactorSnapshot.
type FactorSnapshot struct {
	scores map[string]map[portfolio.FactorType]float64
}

// NewFactorSnapshot pre-computes Momentum, Value, Quality, and Liquidity scores
// for every symbol in the quotes map. Returns an empty snapshot (no panic) when
// fe is nil. Agent and InstitutionalSentiment factors require per-executor data
// and are not pre-computed here.
func NewFactorSnapshot(quotes map[string]domain.Quote, fe *portfolio.FactorEngine) *FactorSnapshot {
	if fe == nil {
		return &FactorSnapshot{
			scores: make(map[string]map[portfolio.FactorType]float64),
		}
	}

	scores := make(map[string]map[portfolio.FactorType]float64, len(quotes))
	for symbol := range quotes {
		entry := make(map[portfolio.FactorType]float64, 4)
		entry[portfolio.FactorMomentum] = fe.CalculateMomentumScore(symbol, quotes)
		entry[portfolio.FactorValue] = fe.CalculateValueScore(symbol, quotes)
		entry[portfolio.FactorQuality] = fe.CalculateQualityScore(symbol, quotes)
		entry[portfolio.FactorLiquidity] = fe.CalculateLiquidityScore(symbol, quotes).Score
		scores[symbol] = entry
	}

	return &FactorSnapshot{scores: scores}
}

// GetScore returns the pre-computed score for the given symbol and factor.
// Returns (0, false) when the receiver is nil, the symbol is not found,
// or the factor is not available in the snapshot.
func (fs *FactorSnapshot) GetScore(symbol string, factor portfolio.FactorType) (float64, bool) {
	if fs == nil || fs.scores == nil {
		return 0, false
	}
	entry, ok := fs.scores[symbol]
	if !ok {
		return 0, false
	}
	score, ok := entry[factor]
	if !ok {
		return 0, false
	}
	return score, true
}

// compile-time interface check
var _ FactorQuery = (*FactorSnapshot)(nil)
