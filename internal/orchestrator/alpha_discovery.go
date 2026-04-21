package orchestrator

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// AlphaDiscoveryEngine identifies symbols with high multi-factor scores
// but low agent coverage, generating exploratory recommendations.
type AlphaDiscoveryEngine struct {
	factorEngine    *portfolio.FactorEngine
	factorThreshold float64
}

// NewAlphaDiscoveryEngine creates a new alpha discovery engine.
func NewAlphaDiscoveryEngine(factorEngine *portfolio.FactorEngine) *AlphaDiscoveryEngine {
	return &AlphaDiscoveryEngine{
		factorEngine:    factorEngine,
		factorThreshold: 0.2,
	}
}

// SetFactorThreshold sets the minimum multi-factor score to trigger discovery.
func (e *AlphaDiscoveryEngine) SetFactorThreshold(threshold float64) {
	e.factorThreshold = threshold
}

// Discover scans symbols for high-factor / low-coverage opportunities.
func (e *AlphaDiscoveryEngine) Discover(
	ctx context.Context,
	symbols []string,
	quotes map[string]domain.Quote,
	recs []domain.Recommendation,
) []domain.Recommendation {
	// Count agent coverage per symbol
	coverage := make(map[string]int)
	for _, rec := range recs {
		coverage[rec.Symbol]++
	}

	// Use the optimizer's factor scoring logic directly.
	// We create synthetic recommendations per symbol to feed the optimizer.
	var discovered []domain.Recommendation
	for _, symbol := range symbols {
		if coverage[symbol] > 1 {
			continue // Only discover for 0 or 1 agent coverage
		}

		quote, ok := quotes[symbol]
		if !ok || !quote.IsTradable || quote.Last == 0 {
			continue
		}

		// We use a package-local helper to compute scores without exposing private methods.
		// Since we can't call private methods, we approximate by running a mini optimize.
		score := e.estimateFactorScore(ctx, symbol, quotes)
		if score < e.factorThreshold {
			continue
		}

		tp, slp := priceTargets(quote, 1.06, 0.95)
		discovered = append(discovered, domain.Recommendation{
			Agent:         "alpha_discovery",
			Skill:         "alpha_discovery",
			Layer:         domain.LayerStyle,
			Symbol:        symbol,
			Side:          domain.SideBuy,
			Conviction:    40,
			Reason:        fmt.Sprintf("alpha_discovery|factor_score:%.2f|coverage:%d", score, coverage[symbol]),
			TargetPrice:   tp,
			StopLossPrice: slp,
		})
	}

	return discovered
}

// estimateFactorScore computes a rough multi-factor score for a symbol using
// the factor engine directly.
func (e *AlphaDiscoveryEngine) estimateFactorScore(_ context.Context, symbol string, quotes map[string]domain.Quote) float64 {
	if e.factorEngine == nil {
		return 0.0
	}
	defaultWeights := map[portfolio.FactorType]float64{
		portfolio.FactorMomentum: 0.30,
		portfolio.FactorValue:    0.25,
		portfolio.FactorQuality:  0.25,
		portfolio.FactorAgent:    0.20,
	}
	scores := e.factorEngine.CalculateAllScores(symbol, quotes, nil, nil, defaultWeights)
	total, ok := scores["total"]
	if !ok {
		return 0.0
	}
	return total
}
