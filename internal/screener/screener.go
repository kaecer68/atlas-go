package screener

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// Screener evaluates whether a symbol passes an agent's ScreeningCriteria.
type Screener interface {
	Screen(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (bool, error)
	ScreenUniverse(ctx context.Context, symbols []string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) ([]string, error)
}

// Engine implements Screener using FactorEngine and FundamentalProvider.
type Engine struct {
	factorEngine *portfolio.FactorEngine
	fundamentals *portfolio.FundamentalProvider
}

// NewEngine creates a screener with the required data sources.
func NewEngine(fe *portfolio.FactorEngine, fp *portfolio.FundamentalProvider) *Engine {
	return &Engine{
		factorEngine: fe,
		fundamentals: fp,
	}
}

// Screen evaluates a single symbol against criteria. Returns true if it passes all present filters.
func (e *Engine) Screen(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (bool, error) {
	if !criteria.HasFilters() {
		return true, nil
	}

	quote, hasQuote := quotes[symbol]

	if criteria.VolumeIntraday != nil && criteria.VolumeIntraday.Min != nil {
		if !hasQuote {
			return false, nil
		}
		minVol := *criteria.VolumeIntraday.Min
		if quote.Volume < minVol {
			return false, nil
		}
	}

	if e.fundamentals != nil && e.fundamentals.HasData() {
		data := e.fundamentals.Get(symbol)

		if criteria.PE != nil {
			if data.PE > 0 {
				if criteria.PE.Min != nil && data.PE < *criteria.PE.Min {
					return false, nil
				}
				if criteria.PE.Max != nil && data.PE > *criteria.PE.Max {
					return false, nil
				}
			} else {
				return false, nil
			}
		}

		if criteria.PB != nil {
			if data.PB > 0 {
				if criteria.PB.Min != nil && data.PB < *criteria.PB.Min {
					return false, nil
				}
				if criteria.PB.Max != nil && data.PB > *criteria.PB.Max {
					return false, nil
				}
			} else {
				return false, nil
			}
		}

		if criteria.DividendYield != nil {
			if data.DividendYield > 0 {
				if criteria.DividendYield.Min != nil && data.DividendYield < *criteria.DividendYield.Min {
					return false, nil
				}
				if criteria.DividendYield.Max != nil && data.DividendYield > *criteria.DividendYield.Max {
					return false, nil
				}
			}
		}
	}

	if e.factorEngine != nil {
		if criteria.Momentum20Day != nil {
			momentum := e.factorEngine.CalculateMomentumScore(symbol, quotes)
			if criteria.Momentum20Day.Min != nil && momentum < *criteria.Momentum20Day.Min {
				return false, nil
			}
			if criteria.Momentum20Day.Max != nil && momentum > *criteria.Momentum20Day.Max {
				return false, nil
			}
		}

		if criteria.MinTotalFactorScore != nil {
			defaultWeights := map[portfolio.FactorType]float64{
				portfolio.FactorMomentum: 0.30,
				portfolio.FactorValue:    0.25,
				portfolio.FactorQuality:  0.25,
				portfolio.FactorAgent:    0.20,
			}
			scores := e.factorEngine.CalculateAllScores(symbol, quotes, nil, nil, defaultWeights)
			total, ok := scores["total"]
			if !ok || total < *criteria.MinTotalFactorScore {
				return false, nil
			}
		}
	}

	_ = ctx
	return true, nil
}

// ScreenUniverse filters a list of symbols, returning only those that pass.
func (e *Engine) ScreenUniverse(ctx context.Context, symbols []string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) ([]string, error) {
	var passed []string
	for _, symbol := range symbols {
		ok, err := e.Screen(ctx, symbol, criteria, quotes)
		if err != nil {
			return nil, fmt.Errorf("screen %s: %w", symbol, err)
		}
		if ok {
			passed = append(passed, symbol)
		}
	}
	return passed, nil
}
