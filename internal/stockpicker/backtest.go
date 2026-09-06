// Package backtest.go — PR 1c stockpicker panel backtest engine.
//
// The engine replays a hardcoded demo condition set over a historical panel
// (price bars + per-symbol T86 institutional flows) using only point-in-time
// (PIT) data, then produces SignalOutcome rows whose hit is determined by
// NetHit (net of round-trip cost, P0-3).
//
// PIT guarantees (P0-1):
//   - Condition inputs at trigger date t are computed strictly from data with
//     date <= t (the engine passes bars[:i+1] and flows dated <= t only).
//   - The engine rejects any panel whose latest bar is after the as-of date:
//     that is exactly the lookahead the design forbids
//     (TestBacktest_NoFutureData).
//   - Fundamentals-based conditions (value / all_weather PE/PB/div-yield) are
//     deliberately absent: data/fundamentals.json is a single dateless
//     snapshot, so any historical use would be lookahead. They stay
//     live_observe_only until a dated fundamentals-history pipeline exists.
package stockpicker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// HistoricalBar is the minimal point-in-time daily bar a condition may read.
type HistoricalBar struct {
	Date   time.Time
	Close  float64
	Volume int64
}

// FlowPoint is a per-symbol T86 institutional flow reading for one date
// (units: shares; the provider divides TWSE 股數 by 1e3). JSON tags align
// with the data/state/stock_flows/<symbol>.json file format.
type FlowPoint struct {
	Date       string  `json:"date"` // YYYY-MM-DD
	ForeignNet float64 `json:"foreign_net"`
}

// PanelSource supplies point-in-time historical data to the backtest.
// Implementations MUST NOT return data dated after the run's as-of date;
// the engine enforces this and fails the run otherwise.
type PanelSource interface {
	Bars(ctx context.Context, symbol string) ([]HistoricalBar, error)
	Flows(ctx context.Context, symbol string) ([]FlowPoint, error)
}

// BacktestConfig configures one panel backtest run.
type BacktestConfig struct {
	Universe    []string  // symbols to scan
	Start       time.Time // first trigger date (inclusive)
	End         time.Time // last trigger date (inclusive)
	AsOf        time.Time // PIT data cutoff; any bar after this fails the run
	ForwardDays int       // holding period in trading days (default 5)
	CostRate    float64   // round-trip cost rate fed to NetHit (P0-3)
	Source      string    // outcome source prefix (default "stockpicker")
}

// DemoConditionID identifies one of the PR 1c hardcoded demo conditions.
type DemoConditionID string

// The PR 1c demo condition set, extended 2026-09-07 with the price/volume
// divergence pair (量價背離). All conditions consume only PIT data: price
// bars (momentum, divergence) and per-symbol T86 flows (foreign net buy).
// The configurable condition engine is PR 2a.
const (
	ConditionForeign3DNetBuy DemoConditionID = "foreign-3d-net-buy"
	ConditionMomentum20D     DemoConditionID = "momentum-20d-positive"
)

// DemoConditions returns the default backtest-eligible condition IDs.
// Fundamentals conditions (value / all_weather) are intentionally absent
// (P0-1). The condition definitions themselves come from
// DefaultConditions() (PR 2a); the divergence pair was added 2026-09-07
// (both PIT-safe ConditionTypePrice over Date/Close/Volume bars).
func DemoConditions() []DemoConditionID {
	return []DemoConditionID{
		ConditionForeign3DNetBuy,
		ConditionMomentum20D,
		ConditionPriceVolumeTopDivergence,
		ConditionPriceVolumeBottomDivergence,
	}
}

// RunBacktest replays conditions over the panel and returns the
// SignalOutcome rows for every (symbol, trigger_date, condition) that fired.
// conditions is variadic for backward compatibility: when empty, the default
// backtest-eligible conditions from the loaded parameters config are used
// (DefaultConditions, PR 2a). Forward return is measured from the trigger
// close to the close ForwardDays trading sessions later;
// hit = NetHit(forwardReturn, costRate) (P0-3).
func RunBacktest(ctx context.Context, cfg BacktestConfig, panel PanelSource, conditions ...Condition) ([]SignalOutcome, error) {
	if cfg.AsOf.IsZero() {
		return nil, fmt.Errorf("stockpicker: backtest requires AsOf (point-in-time cutoff)")
	}
	if cfg.Start.IsZero() || cfg.End.IsZero() {
		return nil, fmt.Errorf("stockpicker: backtest requires Start and End")
	}
	if cfg.End.Before(cfg.Start) {
		return nil, fmt.Errorf("stockpicker: End %s before Start %s", cfg.End.Format("2006-01-02"), cfg.Start.Format("2006-01-02"))
	}
	if cfg.ForwardDays <= 0 {
		cfg.ForwardDays = 5
	}
	if cfg.CostRate < 0 {
		return nil, fmt.Errorf("stockpicker: CostRate must be non-negative, got %v", cfg.CostRate)
	}
	if cfg.Source == "" {
		cfg.Source = "stockpicker"
	}
	if len(cfg.Universe) == 0 {
		return nil, fmt.Errorf("stockpicker: empty universe")
	}

	conds := conditions
	if len(conds) == 0 {
		conds = DefaultConditions() // PR 2a: params from configs/parameters.json
	}
	if len(conds) == 0 {
		return nil, fmt.Errorf("stockpicker: no conditions to evaluate")
	}

	var out []SignalOutcome
	for _, symbol := range cfg.Universe {
		bars, err := panel.Bars(ctx, symbol)
		if err != nil {
			return nil, fmt.Errorf("stockpicker: backtest bars %s: %w", symbol, err)
		}
		flows, err := panel.Flows(ctx, symbol)
		if err != nil {
			return nil, fmt.Errorf("stockpicker: backtest flows %s: %w", symbol, err)
		}

		// P0-1 PIT red line: any bar after the as-of date is lookahead.
		if n := len(bars); n > 0 && bars[n-1].Date.After(cfg.AsOf) {
			return nil, fmt.Errorf("stockpicker: lookahead: %s has bar %s after as-of %s (point-in-time violation)",
				symbol, bars[n-1].Date.Format("2006-01-02"), cfg.AsOf.Format("2006-01-02"))
		}

		flowByDate := make(map[string]FlowPoint, len(flows))
		flowDates := make([]string, 0, len(flows))
		for _, f := range flows {
			flowByDate[f.Date] = f
			flowDates = append(flowDates, f.Date)
		}
		sort.Strings(flowDates)

		for i := range bars {
			t := bars[i].Date
			if t.Before(cfg.Start) || t.After(cfg.End) {
				continue
			}
			if i+cfg.ForwardDays >= len(bars) {
				continue // no forward truth available
			}
			closePrice := bars[i].Close
			if closePrice <= 0 {
				continue
			}
			fwd := bars[i+cfg.ForwardDays].Close
			forwardReturn := fwd/closePrice - 1

			prefix := bars[:i+1] // condition inputs: data with date <= t only
			for _, cond := range conds {
				if !cond.Eval(prefix, flowByDate, flowDates, t) {
					continue
				}
				out = append(out, SignalOutcome{
					Symbol:           symbol,
					TriggerDate:      t.Format("2006-01-02"),
					ForwardReturn:    forwardReturn,
					NetForwardReturn: forwardReturn - cfg.CostRate,
					Hit:              NetHit(forwardReturn, cfg.CostRate),
					CostRate:         cfg.CostRate,
					Source:           cfg.Source + "-" + cond.ID,
				})
			}
		}
	}
	return out, nil
}

// CoverageReport summarizes what a backtest run produced per condition,
// for the CLI's data-coverage output and PR verification.
type CoverageReport struct {
	AsOf          string         `json:"as_of"`
	Start         string         `json:"start"`
	End           string         `json:"end"`
	UniverseSize  int            `json:"universe_size"`
	TotalOutcomes int            `json:"total_outcomes"`
	BySource      map[string]int `json:"by_source"`
}

// BuildCoverage groups outcomes into a CoverageReport.
func BuildCoverage(cfg BacktestConfig, outcomes []SignalOutcome) CoverageReport {
	rep := CoverageReport{
		AsOf:         cfg.AsOf.Format("2006-01-02"),
		Start:        cfg.Start.Format("2006-01-02"),
		End:          cfg.End.Format("2006-01-02"),
		UniverseSize: len(cfg.Universe),
		BySource:     make(map[string]int),
	}
	for _, o := range outcomes {
		rep.TotalOutcomes++
		rep.BySource[o.Source]++
	}
	return rep
}

// DescribeConditions returns a human-readable line listing the demo
// condition IDs, used by the CLI and tests to prove fundamentals are
// excluded. The definitions of the configurable conditions themselves live
// in conditions.go (PR 2a).
func DescribeConditions() string {
	ids := make([]string, len(DemoConditions()))
	for i, id := range DemoConditions() {
		ids[i] = string(id)
	}
	return strings.Join(ids, ", ")
}
