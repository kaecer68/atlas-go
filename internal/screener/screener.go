package screener

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// TraceWriter is an optional interface for recording screening events.
// Implementations should be nil-safe and non-blocking.
type TraceWriter interface {
	Record(step int, layer, status string, meta map[string]any)
}

// Screener evaluates whether a symbol passes an agent's ScreeningCriteria.
type Screener interface {
	Screen(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (bool, error)
	ScreenDetailed(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (ScreenResult, error)
	ScreenUniverse(ctx context.Context, symbols []string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) ([]string, error)
}

// Engine implements Screener using FactorEngine and FundamentalProvider.
type Engine struct {
	factorEngine *portfolio.FactorEngine
	fundamentals *portfolio.FundamentalProvider
	traceWriter  TraceWriter
}

// NewEngine creates a screener with the required data sources.
func NewEngine(fe *portfolio.FactorEngine, fp *portfolio.FundamentalProvider) *Engine {
	return &Engine{
		factorEngine: fe,
		fundamentals: fp,
	}
}

// WithTraceWriter sets an optional trace writer for recording screening events.
func (e *Engine) WithTraceWriter(tw TraceWriter) *Engine {
	e.traceWriter = tw
	return e
}

// ScreenResult carries the outcome of a single screening evaluation.
type ScreenResult struct {
	Passed    bool
	Reason    string
	Criterion string
	Label     string
	Threshold string
	Actual    string
}

// Screen evaluates a single symbol against criteria. Returns true if it passes all present filters.
func (e *Engine) Screen(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (bool, error) {
	res, err := e.ScreenDetailed(ctx, symbol, criteria, quotes)
	return res.Passed, err
}

// ScreenDetailed evaluates a single symbol and returns detailed pass/fail metadata.
// ScreenDetailed never returns a non-nil error; the error return is reserved for future use.
func (e *Engine) ScreenDetailed(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (ScreenResult, error) {
	pass := func() ScreenResult {
		return ScreenResult{Passed: true}
	}
	fail := func(criterion, label, threshold, actual string) ScreenResult {
		return ScreenResult{
			Passed:    false,
			Reason:    fmt.Sprintf("%s: %s (threshold %s, actual %s)", criterion, label, threshold, actual),
			Criterion: criterion,
			Label:     label,
			Threshold: threshold,
			Actual:    actual,
		}
	}

	if !criteria.HasFilters() {
		return pass(), nil
	}

	quote, hasQuote := quotes[symbol]

	if criteria.VolumeIntraday != nil && criteria.VolumeIntraday.Min != nil {
		if !hasQuote {
			return fail("volume_intraday_min", "Volume intraday", fmt.Sprintf("%d", *criteria.VolumeIntraday.Min), "missing quote"), nil
		}
		minVol := *criteria.VolumeIntraday.Min
		if quote.Volume < minVol {
			return fail("volume_intraday_min", "Volume intraday", fmt.Sprintf("%d", minVol), fmt.Sprintf("%d", quote.Volume)), nil
		}
	}

	if e.fundamentals != nil && e.fundamentals.HasData() {
		data := e.fundamentals.Get(symbol)

		if criteria.PE != nil {
			if data.PE > 0 {
				if criteria.PE.Min != nil && data.PE < *criteria.PE.Min {
					return fail("pe_min", "P/E", fmt.Sprintf("%.2f", *criteria.PE.Min), fmt.Sprintf("%.2f", data.PE)), nil
				}
				if criteria.PE.Max != nil && data.PE > *criteria.PE.Max {
					return fail("pe_max", "P/E", fmt.Sprintf("%.2f", *criteria.PE.Max), fmt.Sprintf("%.2f", data.PE)), nil
				}
			} else {
				return fail("pe_missing", "P/E", "required", "missing data"), nil
			}
		}

		if criteria.PB != nil {
			if data.PB > 0 {
				if criteria.PB.Min != nil && data.PB < *criteria.PB.Min {
					return fail("pb_min", "P/B", fmt.Sprintf("%.2f", *criteria.PB.Min), fmt.Sprintf("%.2f", data.PB)), nil
				}
				if criteria.PB.Max != nil && data.PB > *criteria.PB.Max {
					return fail("pb_max", "P/B", fmt.Sprintf("%.2f", *criteria.PB.Max), fmt.Sprintf("%.2f", data.PB)), nil
				}
			} else {
				return fail("pb_missing", "P/B", "required", "missing data"), nil
			}
		}

		if criteria.DividendYield != nil {
			if data.DividendYield > 0 {
				if criteria.DividendYield.Min != nil && data.DividendYield < *criteria.DividendYield.Min {
					return fail("dividend_yield_min", "Dividend yield", fmt.Sprintf("%.2f", *criteria.DividendYield.Min), fmt.Sprintf("%.2f", data.DividendYield)), nil
				}
				if criteria.DividendYield.Max != nil && data.DividendYield > *criteria.DividendYield.Max {
					return fail("dividend_yield_max", "Dividend yield", fmt.Sprintf("%.2f", *criteria.DividendYield.Max), fmt.Sprintf("%.2f", data.DividendYield)), nil
				}
			}
		}
	}

	if e.factorEngine != nil {
		if criteria.Momentum20Day != nil {
			momentum := e.factorEngine.CalculateMomentumScore(symbol, quotes)
			if criteria.Momentum20Day.Min != nil && momentum < *criteria.Momentum20Day.Min {
				return fail("momentum_20d_min", "20-day momentum", fmt.Sprintf("%.2f", *criteria.Momentum20Day.Min), fmt.Sprintf("%.2f", momentum)), nil
			}
			if criteria.Momentum20Day.Max != nil && momentum > *criteria.Momentum20Day.Max {
				return fail("momentum_20d_max", "20-day momentum", fmt.Sprintf("%.2f", *criteria.Momentum20Day.Max), fmt.Sprintf("%.2f", momentum)), nil
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
				actual := "missing"
				if ok {
					actual = fmt.Sprintf("%.3f", total)
				}
				return fail("min_total_factor_score", "Total factor score", fmt.Sprintf("%.3f", *criteria.MinTotalFactorScore), actual), nil
			}
		}
	}

	return pass(), nil
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

	// Emit WARN trace if all symbols were rejected.
	if len(passed) == 0 && len(symbols) > 0 && e.traceWriter != nil {
		criteriaDesc := "no filters"
		if criteria.HasFilters() {
			criteriaDesc = "active filters"
		}
		e.traceWriter.Record(0, "screener", "WARN", map[string]any{
			"candidates": len(symbols),
			"rejected":   len(symbols),
			"criteria":   criteriaDesc,
		})
	}

	return passed, nil
}
