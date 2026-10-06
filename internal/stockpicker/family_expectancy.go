package stockpicker

import (
	"math"
	"sort"
	"strings"
	"time"
)

// ─── Signal-family net expectancy + regime stratification (read-only) ─────
//
// This file adds the FAMILY-level view of the persisted signal outcomes: one
// row per source ("family"), using exactly the canonical cost-aware caliber
// of SignalWinRate / ConditionWinRate / IndustryWinRate — hit =
// NetHit(forward_return, cost_rate), rate = WinRate, interval =
// WilsonScoreInterval, gate = CalibrationStatusFor — plus the two things
// those views do not answer:
//
//  1. net expectancy as a t statistic (mean net return divided by the
//     standard error of the mean), so a family can be read as "still alive"
//     or "not alive" instead of only "how often did it hit";
//  2. a trailing sub-window over the most recent TradingDays distinct
//     trigger dates OF THE READ. A family that stopped firing months ago
//     then reports trailing_n = 0 instead of looking healthy on its
//     full-period record — that is the whole point of the instrument.
//
// Read-only by construction: nothing here recomputes a backtest, writes a
// row, or changes an existing number.

// DefaultTrailingTradingDays is the trailing sub-window length used by the
// family-expectancy instrument when the caller does not override it.
const DefaultTrailingTradingDays = 60

// UnknownRegime is the stratum label for outcomes whose regime column is
// empty. It is never imputed backwards: a row stored before regime tagging
// stays "unknown" forever (the value is reported, never guessed).
const UnknownRegime = "unknown"

// triggerDateLayout is the only accepted trigger_date format (ISO date, the
// format RecordOutcomes persists and the format LoadOutcomes orders by).
const triggerDateLayout = "2006-01-02"

// FamilyRegimeStratum is one regime stratum of a family row. Regime values
// are taken verbatim from the stored column (whitespace trimmed); the empty
// string becomes UnknownRegime.
type FamilyRegimeStratum struct {
	Regime              string  `json:"regime"`
	N                   int     `json:"n"`
	NetHitRate          float64 `json:"net_hit_rate"`
	WilsonLower         float64 `json:"wilson_lower"`
	WilsonUpper         float64 `json:"wilson_upper"`
	AvgNetForwardReturn float64 `json:"avg_net_forward_return"`
	NetExpectancyT      float64 `json:"net_expectancy_t"`
}

// FamilyExpectancyRow is one family (source) row of the report.
//
// N / NetHitRate / WilsonLower / WilsonUpper / AvgNetForwardReturn /
// NetExpectancyT cover the whole read period (no rolling-window cutoff);
// the Trailing* fields repeat the same metrics over the trailing sub-window.
// Net returns are forward_return - cost_rate (the live parameter cost rate),
// identical to the industry-level caliber.
type FamilyExpectancyRow struct {
	Source              string  `json:"source"`
	ConditionID         string  `json:"condition_id"`
	Direction           string  `json:"direction"` // "buy" (default) or "avoid" (inverted semantics)
	N                   int     `json:"n"`
	Hits                int     `json:"hits"`
	NetHitRate          float64 `json:"net_hit_rate"`
	WilsonLower         float64 `json:"wilson_lower"`
	WilsonUpper         float64 `json:"wilson_upper"`
	Confidence          float64 `json:"confidence"`
	CalibrationStatus   string  `json:"calibration_status"`
	NetCostRate         float64 `json:"net_cost_rate"`
	Symbols             int     `json:"symbols"`
	AvgNetForwardReturn float64 `json:"avg_net_forward_return"`
	NetExpectancyT      float64 `json:"net_expectancy_t"`
	DataStart           string  `json:"data_start,omitempty"`
	DataEnd             string  `json:"data_end,omitempty"`
	// Trailing sub-window metrics. TrailingWindowStart/End bound the window
	// actually used (echoed even when TrailingN == 0, so a caller can tell
	// "no recent evidence" apart from "no window").
	TrailingN                   int                   `json:"trailing_n"`
	TrailingAvgNetForwardReturn float64               `json:"trailing_avg_net_forward_return"`
	TrailingNetExpectancyT      float64               `json:"trailing_net_expectancy_t"`
	TrailingWindowStart         string                `json:"trailing_window_start,omitempty"`
	TrailingWindowEnd           string                `json:"trailing_window_end,omitempty"`
	ByRegime                    []FamilyRegimeStratum `json:"by_regime"`
	GeneratedAt                 string                `json:"generated_at"`
}

// FamilyExpectancyReport is the full answer: the trailing window definition,
// the data-quality counters, and one row per family.
type FamilyExpectancyReport struct {
	GeneratedAt         string `json:"generated_at"`
	TrailingTradingDays int    `json:"trailing_trading_days"`
	TrailingWindowStart string `json:"trailing_window_start,omitempty"`
	TrailingWindowEnd   string `json:"trailing_window_end,omitempty"`
	// TotalObservations counts every row read, including rows with an
	// unparsable trigger_date and rows with no source. Summing Families[].n
	// may therefore be smaller than TotalObservations; the two counters below
	// plus UnattributedObservations explain the difference (nothing is
	// dropped silently).
	TotalObservations        int                   `json:"total_observations"`
	InvalidTriggerDates      int                   `json:"invalid_trigger_dates"`
	NonTradingTriggerDates   int                   `json:"non_trading_trigger_dates"`
	UnattributedObservations int                   `json:"unattributed_observations"`
	Families                 []FamilyExpectancyRow `json:"families"`
}

// NetExpectancyT returns the t statistic of a net-return series:
// mean / (sampleStdDev / sqrt(n)).
//
// It is the single implementation of this statistic in the repo, so the
// whole-family view, the regime strata and the trailing window report
// comparable numbers.
//
// Sentinel: 0 is returned when the statistic is not defined — fewer than 2
// observations, zero dispersion (every value identical), or a non-finite
// result. 0 therefore means "not computable", NOT "no effect": read n and
// avg_net_forward_return together with it.
func NetExpectancyT(netReturns []float64) float64 {
	n := len(netReturns)
	if n < 2 {
		return 0
	}
	var sum float64
	for _, x := range netReturns {
		sum += x
	}
	mean := sum / float64(n)

	var sqDiff float64
	for _, x := range netReturns {
		d := x - mean
		sqDiff += d * d
	}
	variance := sqDiff / float64(n-1) // sample variance (n-1)
	if variance <= 0 {
		return 0
	}
	se := math.Sqrt(variance / float64(n))
	if se == 0 {
		return 0
	}
	t := mean / se
	if math.IsNaN(t) || math.IsInf(t, 0) {
		return 0
	}
	return t
}

// expectancyBlock is the shared metric bundle; the family row, the trailing
// sub-window and every regime stratum are filled from it, so all of them
// reuse one computation path (NetHit / WinRate / WilsonScoreInterval /
// NetExpectancyT) rather than a private copy.
type expectancyBlock struct {
	N                   int
	Hits                int
	NetHitRate          float64
	WilsonLower         float64
	WilsonUpper         float64
	AvgNetForwardReturn float64
	NetExpectancyT      float64
}

// computeExpectancyBlock aggregates outcomes with the shared helpers.
// Empty input yields the zero block (no division by zero, no panic).
func computeExpectancyBlock(outcomes []SignalOutcome, costRate, confidence float64) expectancyBlock {
	blk := expectancyBlock{N: len(outcomes)}
	netReturns := make([]float64, 0, len(outcomes))
	var netSum float64
	for _, o := range outcomes {
		if NetHit(o.ForwardReturn, costRate) {
			blk.Hits++
		}
		net := o.ForwardReturn - costRate
		netReturns = append(netReturns, net)
		netSum += net
	}
	blk.NetHitRate = WinRate(blk.Hits, blk.N)
	blk.WilsonLower, blk.WilsonUpper = WilsonScoreInterval(blk.Hits, blk.N, confidence)
	if blk.N > 0 {
		blk.AvgNetForwardReturn = netSum / float64(blk.N)
	}
	blk.NetExpectancyT = NetExpectancyT(netReturns)
	return blk
}

// preparedOutcome pairs a stored outcome with the result of validating its
// trigger_date once, so every later pass (data range, trailing window) reuses
// the same parse instead of re-validating.
type preparedOutcome struct {
	outcome   SignalOutcome
	validDate bool
	date      string // normalized YYYY-MM-DD, empty when !validDate
}

// prepareOutcomes validates trigger_date and tallies the data-quality
// counters. Nothing is dropped here: an unparsable date only removes the row
// from date-window computations (it cannot be placed in time), and the count
// is reported.
func prepareOutcomes(outcomes []SignalOutcome) ([]preparedOutcome, int, int) {
	prepared := make([]preparedOutcome, len(outcomes))
	invalid, nonTrading := 0, 0
	for i, o := range outcomes {
		prepared[i].outcome = o
		raw := strings.TrimSpace(o.TriggerDate)
		t, err := time.Parse(triggerDateLayout, raw)
		if err != nil {
			invalid++
			continue
		}
		prepared[i].validDate = true
		prepared[i].date = raw
		if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
			nonTrading++
		}
	}
	return prepared, invalid, nonTrading
}

// TrailingTriggerDates returns the trailing window as an ascending list of
// the most recent `days` distinct trigger dates present in the read
// (across every family). dates must already be distinct.
//
// The window is derived from the data, not from an exchange calendar: the
// read's own distinct trigger dates are the in-data trading-day proxy. days
// <= 0 or fewer distinct dates than days yield the empty window (and then
// every family reports trailing_n = 0).
func TrailingTriggerDates(dates []string, days int) []string {
	if days <= 0 || len(dates) == 0 {
		return []string{}
	}
	sorted := make([]string, len(dates))
	copy(sorted, dates)
	sort.Strings(sorted)
	if len(sorted) > days {
		sorted = sorted[len(sorted)-days:]
	}
	return sorted
}

// FamilyExpectancy aggregates raw signal outcomes into one row per family
// (source), with the net-expectancy t statistic, per-regime strata and a
// trailing sub-window.
//
// Parameters: costRate is the round-trip cost rate (forward_return - costRate
// is the net return), minSamples is the calibration gate, confidence is the
// Wilson confidence level (0.95 in production), trailingDays is the trailing
// window length in distinct trigger dates (<= 0 = no window), and
// generatedAt is stamped into the report and echoed on every row (the caller
// owns the clock, which keeps the pure function deterministic).
//
// Rows with an empty source cannot be attributed to a family: they are
// counted in UnattributedObservations and excluded from every row (the
// persisted store rejects empty sources, so this is only reachable from
// in-memory callers).
func FamilyExpectancy(outcomes []SignalOutcome, costRate float64, minSamples int, confidence float64, trailingDays int, generatedAt string) FamilyExpectancyReport {
	prepared, invalidDates, nonTradingDates := prepareOutcomes(outcomes)

	report := FamilyExpectancyReport{
		GeneratedAt:            generatedAt,
		TrailingTradingDays:    trailingDays,
		TotalObservations:      len(outcomes),
		InvalidTriggerDates:    invalidDates,
		NonTradingTriggerDates: nonTradingDates,
		Families:               []FamilyExpectancyRow{},
	}

	distinctDates := make(map[string]bool, len(prepared))
	for _, p := range prepared {
		if p.validDate {
			distinctDates[p.date] = true
		}
	}
	dateList := make([]string, 0, len(distinctDates))
	for d := range distinctDates {
		dateList = append(dateList, d)
	}
	window := TrailingTriggerDates(dateList, trailingDays)
	inWindow := make(map[string]bool, len(window))
	for _, d := range window {
		inWindow[d] = true
	}
	if len(window) > 0 {
		report.TrailingWindowStart = window[0]
		report.TrailingWindowEnd = window[len(window)-1]
	}

	bySource := make(map[string][]int)
	sources := make([]string, 0)
	for i := range prepared {
		src := strings.TrimSpace(prepared[i].outcome.Source)
		if src == "" {
			report.UnattributedObservations++
			continue
		}
		if _, seen := bySource[src]; !seen {
			sources = append(sources, src)
		}
		bySource[src] = append(bySource[src], i)
	}
	sort.Strings(sources)

	for _, src := range sources {
		idx := bySource[src]
		rows := make([]SignalOutcome, 0, len(idx))
		trailingRows := make([]SignalOutcome, 0, len(idx))
		regimeRows := make(map[string][]SignalOutcome)
		symbols := make(map[string]bool, len(idx))

		for _, i := range idx {
			p := prepared[i]
			rows = append(rows, p.outcome)
			if p.outcome.Symbol != "" {
				symbols[p.outcome.Symbol] = true
			}
			if p.validDate && inWindow[p.date] {
				trailingRows = append(trailingRows, p.outcome)
			}
			key := regimeKey(p.outcome.Regime)
			regimeRows[key] = append(regimeRows[key], p.outcome)
		}

		blk := computeExpectancyBlock(rows, costRate, confidence)
		trailing := computeExpectancyBlock(trailingRows, costRate, confidence)

		conditionID := strings.TrimPrefix(src, "stockpicker-")
		row := FamilyExpectancyRow{
			Source:                      src,
			ConditionID:                 conditionID,
			Direction:                   "buy",
			N:                           blk.N,
			Hits:                        blk.Hits,
			NetHitRate:                  blk.NetHitRate,
			WilsonLower:                 blk.WilsonLower,
			WilsonUpper:                 blk.WilsonUpper,
			Confidence:                  confidence,
			CalibrationStatus:           string(CalibrationStatusFor(blk.N, minSamples)),
			NetCostRate:                 costRate,
			Symbols:                     len(symbols),
			AvgNetForwardReturn:         blk.AvgNetForwardReturn,
			NetExpectancyT:              blk.NetExpectancyT,
			DataStart:                   minMaxDate(prepared, idx, true),
			DataEnd:                     minMaxDate(prepared, idx, false),
			TrailingN:                   trailing.N,
			TrailingAvgNetForwardReturn: trailing.AvgNetForwardReturn,
			TrailingNetExpectancyT:      trailing.NetExpectancyT,
			TrailingWindowStart:         report.TrailingWindowStart,
			TrailingWindowEnd:           report.TrailingWindowEnd,
			ByRegime:                    buildRegimeStrata(regimeRows, costRate, confidence),
			GeneratedAt:                 generatedAt,
		}
		if IsAvoidCondition(conditionID) {
			row.Direction = "avoid"
		}
		report.Families = append(report.Families, row)
	}
	return report
}

// regimeKey maps the stored regime value to its stratum label: the value
// verbatim (whitespace trimmed), or UnknownRegime when empty. No value is
// ever guessed or filled in.
func regimeKey(regime string) string {
	trimmed := strings.TrimSpace(regime)
	if trimmed == "" {
		return UnknownRegime
	}
	return trimmed
}

// buildRegimeStrata builds the by_regime list: most evidence first, then
// regime name ascending, so the output is deterministic.
func buildRegimeStrata(regimeRows map[string][]SignalOutcome, costRate, confidence float64) []FamilyRegimeStratum {
	strata := make([]FamilyRegimeStratum, 0, len(regimeRows))
	for regime, rows := range regimeRows {
		blk := computeExpectancyBlock(rows, costRate, confidence)
		strata = append(strata, FamilyRegimeStratum{
			Regime:              regime,
			N:                   blk.N,
			NetHitRate:          blk.NetHitRate,
			WilsonLower:         blk.WilsonLower,
			WilsonUpper:         blk.WilsonUpper,
			AvgNetForwardReturn: blk.AvgNetForwardReturn,
			NetExpectancyT:      blk.NetExpectancyT,
		})
	}
	sort.Slice(strata, func(i, j int) bool {
		if strata[i].N != strata[j].N {
			return strata[i].N > strata[j].N
		}
		return strata[i].Regime < strata[j].Regime
	})
	return strata
}

// minMaxDate returns the earliest (wantMin) or latest valid trigger date of
// the given family indices, or "" when none of them has a valid date.
func minMaxDate(prepared []preparedOutcome, idx []int, wantMin bool) string {
	out := ""
	for _, i := range idx {
		p := prepared[i]
		if !p.validDate {
			continue
		}
		if out == "" {
			out = p.date
			continue
		}
		if wantMin && p.date < out {
			out = p.date
		}
		if !wantMin && p.date > out {
			out = p.date
		}
	}
	return out
}
