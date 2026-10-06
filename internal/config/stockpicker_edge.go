package config

import (
	"sort"
	"strings"
)

// ── stockpicker edge policy: which signal families may reach advice ───────
//
// A stockpicker family carries "edge" only if its 5-trading-day net-cost
// expectancy is positive. Measured on the persisted outcome ledger
// (stock_signal_outcomes, 2026-10-06, n = 73,819 triggers, round-trip cost
// 0.585% — the same net_forward_return caliber the win-rate aggregate
// reports):
//
//	price-volume-bottom-divergence  n =  7,327   +0.618%   t = +7.7   kept
//	price-volume-top-divergence     n =  3,403   −0.888%   (AVOID semantics)  kept
//	foreign-3d-net-buy              n = 31,982   −0.517%   t = −11.4  DEMOTED
//	momentum-20d-positive           n = 31,107   −0.987%   t = −21.3  DEMOTED
//
// The two demoted families lose money after costs, so they may never back a
// user-visible recommendation, ranking or tilt: the executor that emitted
// BUY recommendations from them (StockpickerWinrateExecutor), the MCP
// stock_picker_scan ranking, and the sector-allocation industry hit-rate
// tilt chain all refuse them.
//
// Measurement is deliberately NOT affected: outcome recording, win-rate
// aggregation, and the read-only aggregates
// (GET /api/stock/win_rate, /api/stock/condition_winrate,
// /api/stock/industry_winrate and their MCP tools) keep reporting these
// families. Historical rows are never deleted — the demotion is about what
// the platform recommends, not about what it measures.
//
// This is deliberately code, not a configs/parameters.json knob: the list is
// a measurement verdict that must stay consistent with the evidence above,
// and a parameters.json change would also regenerate the derived
// valid_fields.json / field_types.ts artifacts. Re-qualifying a family means
// editing this list together with a fresh expectancy measurement.
const (
	// StockpickerSourcePrefix is the outcome-source prefix written by the
	// stockpicker backfill: "stockpicker-<condition-id>" (see
	// internal/stockpicker/win_rate_store.go and the MCP win-rate tools).
	StockpickerSourcePrefix = "stockpicker-"

	// StockpickerConditionForeign3DNetBuy and
	// StockpickerConditionMomentum20DPositive are the canonical condition
	// ids of the demoted families. They must match the ids registered by
	// internal/stockpicker (ConditionForeign3DNetBuy / ConditionMomentum20D),
	// which internal/stockpicker/demotion_test.go asserts.
	StockpickerConditionForeign3DNetBuy     = "foreign-3d-net-buy"
	StockpickerConditionMomentum20DPositive = "momentum-20d-positive"
)

// demotedStockpickerReasons maps each demoted condition id to the measurement
// that demoted it. The map is the single source of truth read by
// IsDemotedStockpickerCondition / IsDemotedStockpickerSource; keep it in sync
// with the table in the package comment above.
var demotedStockpickerReasons = map[string]string{
	StockpickerConditionForeign3DNetBuy:     "5-day net-cost expectancy -0.517% (t=-11.4, n=31,982): no edge after the 0.585% round-trip cost",
	StockpickerConditionMomentum20DPositive: "5-day net-cost expectancy -0.987% (t=-21.3, n=31,107): no edge after the 0.585% round-trip cost",
}

// DemotedStockpickerConditionIDs returns the demoted condition ids in
// ascending order (deterministic for logs, error messages and tests).
func DemotedStockpickerConditionIDs() []string {
	out := make([]string, 0, len(demotedStockpickerReasons))
	for id := range demotedStockpickerReasons {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// IsDemotedStockpickerCondition reports whether a bare condition id (e.g.
// "foreign-3d-net-buy") belongs to a family that lost its edge and must not
// back any user-visible recommendation. An unknown id is not demoted: this
// predicate adds a refusal, it never becomes a fallback allow-list.
func IsDemotedStockpickerCondition(conditionID string) bool {
	_, ok := demotedStockpickerReasons[strings.TrimPrefix(conditionID, StockpickerSourcePrefix)]
	return ok
}

// DemotedStockpickerConditionReason returns the measurement that demoted the
// condition, or "" when the condition is not demoted. Callers put it in
// operator-facing logs / no-data messages so a refusal explains itself.
func DemotedStockpickerConditionReason(conditionID string) string {
	return demotedStockpickerReasons[strings.TrimPrefix(conditionID, StockpickerSourcePrefix)]
}

// IsDemotedStockpickerSource reports whether an outcome source (e.g.
// "stockpicker-foreign-3d-net-buy") — or the equivalent bare condition id —
// names a demoted family. Callers that hold a source string (the win-rate
// ledger keys, the executor's source override) use this instead of trimming
// the prefix themselves.
func IsDemotedStockpickerSource(source string) bool {
	return IsDemotedStockpickerCondition(source)
}
