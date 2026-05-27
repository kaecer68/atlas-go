package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

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
