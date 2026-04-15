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
	optimizer       *portfolio.Optimizer
	factorThreshold float64
}

// NewAlphaDiscoveryEngine creates a new alpha discovery engine.
func NewAlphaDiscoveryEngine(optimizer *portfolio.Optimizer) *AlphaDiscoveryEngine {
	return &AlphaDiscoveryEngine{
		optimizer:       optimizer,
		factorThreshold: 0.6,
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
// the optimizer's public Optimize entry point with a single dummy recommendation.
func (e *AlphaDiscoveryEngine) estimateFactorScore(ctx context.Context, symbol string, quotes map[string]domain.Quote) float64 {
	quote, ok := quotes[symbol]
	if !ok || quote.Last == 0 {
		return 0.0
	}

	dummyRec := domain.Recommendation{
		Agent:      "alpha_discovery",
		Symbol:     symbol,
		Side:       domain.SideBuy,
		Conviction: 50,
	}

	// Run a minimal optimization to extract the factor scores from the result.
	positions, err := e.optimizer.Optimize(ctx, []domain.Recommendation{dummyRec}, quotes, 1_000_000)
	if err != nil || len(positions) == 0 {
		return 0.0
	}

	pos := positions[0]
	total := 0.0
	for _, score := range pos.Factors {
		total += score
	}
	if len(pos.Factors) == 0 {
		return 0.0
	}
	return total / float64(len(pos.Factors))
}
