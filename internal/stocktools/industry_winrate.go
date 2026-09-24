package stocktools

import (
	"context"
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// ─── industry_winrate (issue #1942) ────────────────────────────────────────
//
// GET /api/stock/industry_winrate exposes the canonical INDUSTRY-level
// hit-rate aggregate defined in
// docs/specs/industry-hitrate-metric-spec.md: the cost-aware stockpicker
// caliber (hit = forward_return - cost_rate > 0, fixed holding period,
// min_samples + Wilson CI) re-keyed on the canonical L1 industry.
//
// Read-only by construction (the SQLite handle is opened with mode=ro) and
// additive: it recomputes nothing that is already persisted — it aggregates
// the raw stock_signal_outcomes rows on the fly, exactly like
// /api/stock/condition_winrate. No existing number changes.

// IndustryWinRateProvider is the minimal read-only interface the
// /api/stock/industry_winrate handler depends on: one industry-level
// aggregate read. Kept separate from WinRateProvider and
// ConditionWinRateProvider so the existing fakes stay valid (issue #1942).
type IndustryWinRateProvider interface {
	// LoadIndustryWinRate returns the industry aggregate for one source
	// ("stockpicker-<condition-id>") over the rolling window. regime
	// ("" = all regimes, or e.g. "RISK_ON") restricts the aggregate to
	// outcomes whose trigger-date regime matches.
	//
	// found=false when the query has no rows: either the source has zero
	// stored outcomes in the window (coverage empty), or the requested
	// regime stratum is empty while the source does have rows (coverage then
	// describes the unfiltered read, and Industries is empty).
	LoadIndustryWinRate(ctx context.Context, source, window, regime string) (stockpicker.IndustryWinRateReport, bool, error)
}

// LoadIndustryWinRate implements IndustryWinRateProvider.
//
// Symbol -> industry attribution uses the canonical L1 taxonomy
// (industry.ClassifyBySymbol / DefaultRepresentativeStocks, 20 L1 sectors).
// Symbols without a canonical mapping stay unmapped and are reported in
// coverage.unmapped_symbols — never dropped silently (issue #1942 §2, and
// the two-namespace problem tracked separately by #1943).
//
// Cost rate and min-samples come from the live parameters config, the same
// values the daily aggregation job uses, so industry numbers are comparable
// with the per-symbol and condition-level ones.
func (p *SQLiteWinRateProvider) LoadIndustryWinRate(ctx context.Context, source, window, regime string) (stockpicker.IndustryWinRateReport, bool, error) {
	all, err := stockpicker.LoadOutcomes(ctx, p.db, "", source, window)
	if err != nil {
		return stockpicker.IndustryWinRateReport{}, false, fmt.Errorf("stocktools: load industry outcomes %s: %w", source, err)
	}

	params := config.GetParametersConfig()
	costRate := params.Stockpicker.Costs.RoundTripPct.Value
	minSamples := params.Stockpicker.Calibration.MinSamples.Value

	if len(all) == 0 {
		// No outcomes at all: return an initialized empty report (empty,
		// non-nil slices and the right direction) so JSON consumers see
		// stable types instead of nulls.
		report, err := aggregateIndustryWinRate(source, nil, costRate, minSamples)
		if err != nil {
			return stockpicker.IndustryWinRateReport{}, false, err
		}
		return finishIndustryReport(report, window, regime), false, nil
	}

	considered := all
	stratumEmpty := false
	if regime != "" {
		considered = filterOutcomesByRegime(all, regime)
		if len(considered) == 0 {
			// Data exists, but none of it carries the requested regime tag.
			// Answer found=false while still reporting the UNFILTERED
			// coverage: the caller must be able to tell "no data" apart from
			// "not in this stratum", and the mapping gap of the read is the
			// useful caveat. Industries stays empty — returning other
			// regimes' rows would silently answer a different question.
			stratumEmpty = true
			considered = all
		}
	}

	report, err := aggregateIndustryWinRate(source, considered, costRate, minSamples)
	if err != nil {
		return stockpicker.IndustryWinRateReport{}, false, err
	}
	report = finishIndustryReport(report, window, regime)
	if stratumEmpty {
		report.Industries = []stockpicker.IndustryWinRateSummary{}
		return report, false, nil
	}
	return report, true, nil
}

// aggregateIndustryWinRate runs the pure canonical aggregation with the live
// parameters. It exists so the three read paths (no data / all regimes /
// stratum) share one call site.
func aggregateIndustryWinRate(source string, outcomes []stockpicker.SignalOutcome, costRate float64, minSamples int) (stockpicker.IndustryWinRateReport, error) {
	report, err := stockpicker.IndustryWinRate(source, outcomes, ResolveCanonicalL1Industry, costRate, minSamples, 0.95)
	if err != nil {
		return stockpicker.IndustryWinRateReport{}, fmt.Errorf("stocktools: aggregate industry win rate %s: %w", source, err)
	}
	return report, nil
}

// filterOutcomesByRegime keeps only the rows tagged with regime. Rows written
// before regime tagging (Regime == "") cannot be attributed to a stratum, so
// they are excluded rather than pooled (same rule as the condition-level
// path). A fresh slice is allocated: the caller still needs the unfiltered
// rows for coverage.
func filterOutcomesByRegime(outcomes []stockpicker.SignalOutcome, regime string) []stockpicker.SignalOutcome {
	filtered := make([]stockpicker.SignalOutcome, 0, len(outcomes))
	for _, o := range outcomes {
		if o.Regime == regime {
			filtered = append(filtered, o)
		}
	}
	return filtered
}

// finishIndustryReport stamps the serving-layer context (rolling window,
// regime echo, zh-TW industry labels) onto a report produced by the pure
// aggregator.
func finishIndustryReport(report stockpicker.IndustryWinRateReport, window, regime string) stockpicker.IndustryWinRateReport {
	report.Window = window
	report.Regime = regime
	annotateIndustryRows(&report)
	return report
}

// ResolveCanonicalL1Industry is the SectorResolver injected into
// stockpicker.IndustryWinRate. It maps a symbol to its canonical L1
// SectorID via industry.ClassifyBySymbol, which reads the canonical
// representative-stock table (internal/industry/representative_stocks.go).
//
// The L2 taxonomy is deliberately not consulted: the industry win-rate key is
// an L1 sector id, and L2 sub-industries would need a different (mutually
// exclusive) mapping that this issue does not define.
func ResolveCanonicalL1Industry(symbol string) (string, bool) {
	if symbol == "" {
		return "", false
	}
	id := industry.ClassifyBySymbol(symbol)
	if id == "" {
		return "", false
	}
	return string(id), true
}

// CanonicalL1IndustryID resolves a caller-supplied industry_id (canonical
// snake_case id, full Chinese label, or legacy Chinese alias) to a canonical
// L1 sector id. ok=false means the value is not a known L1 sector — including
// L2 sub-industries, which have no industry win-rate aggregate yet
// (answering with an empty found=false for those would look like "no data"
// instead of "not a valid key").
func CanonicalL1IndustryID(raw string) (string, bool) {
	id, ok := industry.SectorIDFromString(strings.TrimSpace(raw))
	if !ok || !id.IsL1() {
		return "", false
	}
	return string(id), true
}

// annotateIndustryRows stamps the serving-layer context onto every row (the
// pure aggregator returns rows without it): the rolling window of the read,
// and the zh-TW display label from the canonical DisplayZHTw table so MCP/UI
// consumers do not carry their own copy. Unknown ids keep an empty label.
func annotateIndustryRows(report *stockpicker.IndustryWinRateReport) {
	for i := range report.Industries {
		report.Industries[i].Window = report.Window
		if label, ok := industry.DisplayZHTw[industry.SectorID(report.Industries[i].IndustryID)]; ok {
			report.Industries[i].IndustryNameZH = label
		}
	}
}
