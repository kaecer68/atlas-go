package stockpicker

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ─── Industry-level win-rate aggregation (issue #1942) ─────────────────────
//
// This file adds the canonical INDUSTRY-level hit-rate aggregate. Its
// definition (single source of truth:
// docs/specs/industry-hitrate-metric-spec.md) reuses the only cost-aware
// caliber already in the repo — the stockpicker one:
//
//	hit = forward_return - cost_rate > 0   (fixed holding period, 5 trading
//	days in production; cost_rate = stockpicker.costs.round_trip_pct)
//
// aggregated with the industry (canonical L1 SectorID) as the additional key,
// so the row key is (industry_id, source, rolling_window). Every other
// "hit rate" in the repo is a DIFFERENT caliber and MUST NOT be presented as
// this metric (see the spec's caliber table).
//
// Nothing here changes an existing number: the math (WinRate /
// WilsonScoreInterval / CalibrationStatusFor / NetHit) is shared with
// SignalWinRate, ConditionWinRate and IsDegraded.

// SectorResolver maps a stock symbol to its canonical L1 industry id
// (industry.SectorID as a string). ok=false (or an empty id) means the symbol
// has NO canonical L1 mapping.
//
// Unmapped outcomes are counted and reported (IndustryCoverage.UnmappedSymbols)
// and excluded from every industry row. They are never dropped silently and
// never pooled into an "unknown" bucket: such a bucket would answer a
// different question and would hide the coverage gap (issue #1942).
type SectorResolver func(symbol string) (industryID string, ok bool)

// IndustryWinRateSummary is one (industry_id, source, rolling_window) row.
//
// CalibrationStatus / WilsonLower / WilsonUpper / WinRate reuse the exact
// helpers of the per-symbol caliber, so numbers are comparable across levels
// of aggregation (symbol -> condition -> industry).
//
// CoveragePct is this row's share of the source's outcomes in the window:
// observations / (mapped + unmapped observations). The denominator includes
// unmapped symbols on purpose, so the row can never overstate how much
// evidence it accounts for; summing CoveragePct over rows equals the report's
// Coverage.CoveragePct.
type IndustryWinRateSummary struct {
	IndustryID          string  `json:"industry_id"`
	IndustryNameZH      string  `json:"industry_name_zh,omitempty"`
	Source              string  `json:"source"`
	ConditionID         string  `json:"condition_id"`
	Direction           string  `json:"direction"` // "buy" (default) or "avoid" (inverted semantics)
	Window              string  `json:"rolling_window"`
	Observations        int     `json:"observations"`
	Symbols             int     `json:"symbols"` // distinct symbols contributing
	Hits                int     `json:"hits"`
	WinRate             float64 `json:"win_rate"`
	WilsonLower         float64 `json:"wilson_lower"`
	WilsonUpper         float64 `json:"wilson_upper"`
	Confidence          float64 `json:"confidence"`
	CalibrationStatus   string  `json:"calibration_status"`
	NetCostRate         float64 `json:"net_cost_rate"`
	AvgForwardReturn    float64 `json:"avg_forward_return"`
	AvgNetForwardReturn float64 `json:"avg_net_forward_return"`
	CoveragePct         float64 `json:"coverage_pct"`
	DataStart           string  `json:"data_start,omitempty"`
	DataEnd             string  `json:"data_end,omitempty"`
}

// IndustryUnmappedSymbol names one symbol with no canonical L1 mapping and
// how much evidence it carries, so a consumer can size the gap.
type IndustryUnmappedSymbol struct {
	Symbol       string `json:"symbol"`
	Observations int    `json:"observations"`
}

// IndustryCoverage reports how much of the source's evidence could be
// attributed to a canonical L1 industry. It is always populated — including
// when the filtered query finds no industry row — because the gap itself is
// the answer's most important caveat.
type IndustryCoverage struct {
	TotalObservations    int                      `json:"total_observations"`
	MappedObservations   int                      `json:"mapped_observations"`
	UnmappedObservations int                      `json:"unmapped_observations"`
	TotalSymbols         int                      `json:"total_symbols"`
	MappedSymbols        int                      `json:"mapped_symbols"`
	CoveragePct          float64                  `json:"coverage_pct"`        // by observations, 2 decimals
	SymbolCoveragePct    float64                  `json:"symbol_coverage_pct"` // by distinct symbols, 2 decimals
	UnmappedSymbols      []IndustryUnmappedSymbol `json:"unmapped_symbols"`    // observations desc, then symbol asc
}

// IndustryWinRateReport is the full answer for one source: every industry row
// plus the coverage block.
type IndustryWinRateReport struct {
	Source      string                   `json:"source"`
	ConditionID string                   `json:"condition_id"`
	Direction   string                   `json:"direction"`
	Window      string                   `json:"rolling_window"`
	Regime      string                   `json:"regime,omitempty"` // echo of the serving-layer filter ("" = all regimes)
	Industries  []IndustryWinRateSummary `json:"industries"`
	Coverage    IndustryCoverage         `json:"coverage"`
}

// IndustryWinRate aggregates outcomes by industry for a single source.
//
// Outcomes whose Source differs from source are skipped defensively (same
// discipline as ConditionWinRate: a miswired caller must not silently pool
// heterogeneous conditions) and do not enter the coverage counters. An empty
// input returns an empty report with zero coverage and no error.
//
// resolve must not be nil — industry attribution is the whole point of this
// aggregate, so a nil resolver is a wiring bug, not a "no data" case.
func IndustryWinRate(source string, outcomes []SignalOutcome, resolve SectorResolver, costRate float64, minSamples int, confidence float64) (IndustryWinRateReport, error) {
	if resolve == nil {
		return IndustryWinRateReport{}, fmt.Errorf("stockpicker: IndustryWinRate requires a non-nil SectorResolver")
	}

	condID := strings.TrimPrefix(source, "stockpicker-")
	report := IndustryWinRateReport{
		Source:      source,
		ConditionID: condID,
		Direction:   "buy",
		Industries:  []IndustryWinRateSummary{},
		Coverage: IndustryCoverage{
			UnmappedSymbols: []IndustryUnmappedSymbol{},
		},
	}
	if IsAvoidCondition(condID) {
		report.Direction = "avoid"
	}

	byIndustry := make(map[string][]SignalOutcome)
	symbolObservations := make(map[string]int)
	mappedSymbols := make(map[string]bool)
	unmappedObservations := make(map[string]int)

	for _, o := range outcomes {
		if o.Source != source {
			continue
		}
		report.Coverage.TotalObservations++
		symbolObservations[o.Symbol]++

		industryID, ok := resolve(o.Symbol)
		if !ok || industryID == "" {
			report.Coverage.UnmappedObservations++
			unmappedObservations[o.Symbol]++
			continue
		}
		mappedSymbols[o.Symbol] = true
		report.Coverage.MappedObservations++
		byIndustry[industryID] = append(byIndustry[industryID], o)
	}

	report.Coverage.TotalSymbols = len(symbolObservations)
	report.Coverage.MappedSymbols = len(mappedSymbols)
	report.Coverage.CoveragePct = percentage(report.Coverage.MappedObservations, report.Coverage.TotalObservations)
	report.Coverage.SymbolCoveragePct = percentage(report.Coverage.MappedSymbols, report.Coverage.TotalSymbols)
	report.Coverage.UnmappedSymbols = sortedUnmappedSymbols(unmappedObservations)

	industryIDs := make([]string, 0, len(byIndustry))
	for id := range byIndustry {
		industryIDs = append(industryIDs, id)
	}
	sort.Strings(industryIDs)

	total := report.Coverage.TotalObservations
	for _, id := range industryIDs {
		report.Industries = append(report.Industries, summarizeIndustry(
			id, source, condID, report.Direction, byIndustry[id],
			costRate, minSamples, confidence, total,
		))
	}
	return report, nil
}

// IndustryWinRateFor returns the single row for industryID and whether the
// report contains it. Used by the serving layer's industry_id filter so both
// the HTTP handler and the MCP tool share one lookup.
func IndustryWinRateFor(report IndustryWinRateReport, industryID string) (IndustryWinRateSummary, bool) {
	for _, row := range report.Industries {
		if row.IndustryID == industryID {
			return row, true
		}
	}
	return IndustryWinRateSummary{}, false
}

// summarizeIndustry computes one industry row. outcomes must be non-empty
// (callers only build rows from a non-empty group).
func summarizeIndustry(industryID, source, conditionID, direction string, outcomes []SignalOutcome, costRate float64, minSamples int, confidence float64, totalObservations int) IndustryWinRateSummary {
	summary := IndustryWinRateSummary{
		IndustryID:  industryID,
		Source:      source,
		ConditionID: conditionID,
		Direction:   direction,
		Confidence:  confidence,
		NetCostRate: costRate,
	}

	symbols := make(map[string]bool, len(outcomes))
	var grossSum, netSum float64
	for _, o := range outcomes {
		summary.Observations++
		symbols[o.Symbol] = true
		if NetHit(o.ForwardReturn, costRate) {
			summary.Hits++
		}
		grossSum += o.ForwardReturn
		netSum += o.ForwardReturn - costRate
		if o.TriggerDate == "" {
			continue
		}
		if summary.DataStart == "" || o.TriggerDate < summary.DataStart {
			summary.DataStart = o.TriggerDate
		}
		if o.TriggerDate > summary.DataEnd {
			summary.DataEnd = o.TriggerDate
		}
	}

	n := summary.Observations
	summary.Symbols = len(symbols)
	summary.WinRate = WinRate(summary.Hits, n)
	summary.WilsonLower, summary.WilsonUpper = WilsonScoreInterval(summary.Hits, n, confidence)
	summary.CalibrationStatus = string(CalibrationStatusFor(n, minSamples))
	summary.AvgForwardReturn = grossSum / float64(n)
	summary.AvgNetForwardReturn = netSum / float64(n)
	summary.CoveragePct = percentage(n, totalObservations)
	return summary
}

// percentage returns 100*part/whole rounded to 2 decimals, or 0 when whole
// is not positive. 2 decimals is enough for a coverage caveat and keeps the
// JSON stable for consumers and tests.
func percentage(part, whole int) float64 {
	if whole <= 0 {
		return 0
	}
	return math.Round(float64(part)/float64(whole)*10000) / 100
}

// sortedUnmappedSymbols flattens the per-symbol counts into a deterministic
// list: most evidence first, then symbol ascending.
func sortedUnmappedSymbols(counts map[string]int) []IndustryUnmappedSymbol {
	out := make([]IndustryUnmappedSymbol, 0, len(counts))
	for symbol, n := range counts {
		out = append(out, IndustryUnmappedSymbol{Symbol: symbol, Observations: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Observations != out[j].Observations {
			return out[i].Observations > out[j].Observations
		}
		return out[i].Symbol < out[j].Symbol
	})
	return out
}
