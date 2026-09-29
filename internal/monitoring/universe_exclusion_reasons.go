// universe_exclusion_reasons.go — the auditable accounting of every symbol
// that leaves the universe pipeline between the candidate set (Step 3) and the
// ranked output (Step 4/5), keyed by the rule that removed it (issue #2019).
//
// Why a shared vocabulary instead of ad-hoc strings at each call site: the
// counts are persisted into data/state/universe_snapshot.json and read back by
// operators, the ops verifier and (later) dashboards. A typo in one producer
// would silently become a new series that nobody can join, and the whole point
// of the field is that the number is attributable WITHOUT re-running the
// pipeline.
//
// TWO IDENTITIES hold for a snapshot produced by BuildUniverse, and the tests
// assert both (they are the reason the vocabulary is closed rather than open):
//
//	symbols_filtered = symbols_ranked + Σ(screener_*) + concentration_cap
//	symbols_excluded = risk_total
//
// The first identity is why screener_topn_truncated is present even though a
// TopN cut is not an exclusion: without it the arithmetic would not close and a
// reader would have to guess whether the missing symbols were dropped or never
// existed. The second is why the risk keys are counted per RULE and not per
// symbol: a symbol that fails several risk rules increments every rule it
// failed, so Σ(named risk reasons) may exceed risk_total — a deliberate
// property (it answers "which rule is doing the excluding"), not a rounding
// error.
//
// NOT covered, on purpose:
//   - Step 2 industry filter (industry_filter/dropped): it shrinks the
//     candidate set BEFORE symbols_filtered is recorded, so it is not part of
//     either identity. Its own count is published by the
//     atlas_universe_symbols_filtered_total{result="industry_filter"} counter.
//   - The candidate set itself (symbols_built): where the population comes from
//     is a different question, answered by the coverage check.
package monitoring

// Canonical keys of UniverseBuildResult.SymbolsExcludedReasons.
//
// The risk_* keys mirror RiskExclusionResult.FailReasons verbatim (the risk
// filter already emits stable tokens) with an explicit risk_ prefix, so a
// reader can never confuse a risk rule with a screener rule.
const (
	// ExclusionReasonRiskVaRContribution counts symbols the Layer 2.5 VaR
	// contribution check excluded.
	ExclusionReasonRiskVaRContribution = "risk_var_contribution"
	// ExclusionReasonRiskVolatility counts symbols excluded by the 30-day
	// realized-volatility check.
	ExclusionReasonRiskVolatility = "risk_volatility"
	// ExclusionReasonRiskLiquidity counts symbols excluded by the daily traded
	// amount re-check (the check whose 張-vs-shares unit bug produced the
	// 130/150 exclusion that made this field necessary — issue #1987).
	ExclusionReasonRiskLiquidity = "risk_liquidity"
	// ExclusionReasonRiskUnspecified counts symbols the risk filter failed
	// WITHOUT naming a rule. Present so a failed symbol can never be counted as
	// "excluded for no recorded reason": either a rule names it or this key does.
	ExclusionReasonRiskUnspecified = "risk_unspecified"
	// ExclusionReasonRiskTotal counts the excluded symbols themselves (one per
	// symbol, regardless of how many rules it failed). Equals
	// UniverseBuildResult.SymbolsExcluded.
	ExclusionReasonRiskTotal = "risk_total"

	// ExclusionReasonScreenerNoQuote counts candidates the quote provider
	// returned no quote for (the 上櫃 part of the population lands here when the
	// provider only covers listed names).
	ExclusionReasonScreenerNoQuote = "screener_no_quote"
	// ExclusionReasonScreenerZeroVolume counts candidates whose quote carries
	// Volume == 0 (未成交 rows, or a provider that does not populate the field).
	ExclusionReasonScreenerZeroVolume = "screener_zero_volume"
	// ExclusionReasonScreenerBelowTurnoverFloor counts candidates whose
	// Volume*Last is under VolumeFloorTWD.
	ExclusionReasonScreenerBelowTurnoverFloor = "screener_below_turnover_floor"
	// ExclusionReasonScreenerBelowPriceFloor counts candidates priced under
	// PriceMin.
	ExclusionReasonScreenerBelowPriceFloor = "screener_below_price_floor"
	// ExclusionReasonScreenerBinaryRejected counts candidates dropped by the
	// injected binary screener (ScreenUniverse) after surviving the volume and
	// price filters.
	ExclusionReasonScreenerBinaryRejected = "screener_binary_rejected"
	// ExclusionReasonScreenerNoFactorScore counts survivors for which the factor
	// engine produced no scores at all — scoreAndRank drops them entirely
	// (SP4 §9). Before this key existed the drop was invisible.
	ExclusionReasonScreenerNoFactorScore = "screener_no_factor_score"
	// ExclusionReasonScreenerTopNTruncated counts the ranked symbols cut by the
	// TopN limit. NOT an exclusion — it is the ranking cut — but it is part of
	// the closed arithmetic (see the file header).
	ExclusionReasonScreenerTopNTruncated = "screener_topn_truncated"
	// ExclusionReasonScreenerTotal is every symbol the screener stage removed,
	// i.e. symbols_filtered - symbols_ranked. Equals the sum of the individual
	// screener_* keys (each symbol is attributed to exactly one of them).
	ExclusionReasonScreenerTotal = "screener_total"

	// ExclusionReasonConcentrationCap counts ranked symbols removed by
	// ApplyConcentrationCap (per-industry share above MaxIndustryConcentration).
	ExclusionReasonConcentrationCap = "concentration_cap"
)

// ExclusionReasonKeys is the canonical key vocabulary in stage order. Writers
// emit exactly these keys (zeros included) for every stage that ran, so a key
// that is present with value 0 means "the stage ran and excluded nobody", while
// an absent key means "the stage did not run at all" — the same
// present-zero/absent distinction the channel-health gauges use.
func ExclusionReasonKeys() []string {
	return []string{
		ExclusionReasonRiskVaRContribution,
		ExclusionReasonRiskVolatility,
		ExclusionReasonRiskLiquidity,
		ExclusionReasonRiskUnspecified,
		ExclusionReasonRiskTotal,
		ExclusionReasonScreenerNoQuote,
		ExclusionReasonScreenerZeroVolume,
		ExclusionReasonScreenerBelowTurnoverFloor,
		ExclusionReasonScreenerBelowPriceFloor,
		ExclusionReasonScreenerBinaryRejected,
		ExclusionReasonScreenerNoFactorScore,
		ExclusionReasonScreenerTopNTruncated,
		ExclusionReasonScreenerTotal,
		ExclusionReasonConcentrationCap,
	}
}

// mergeExclusionReasons adds every count of src into dst, creating dst when it
// is nil. Used to assemble one map from the independent stages.
func mergeExclusionReasons(dst, src map[string]int) map[string]int {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]int, len(src))
	}
	for key, count := range src {
		dst[key] += count
	}
	return dst
}
