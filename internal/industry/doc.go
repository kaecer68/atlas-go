// Package industry provides the industry ecosystem for Taiwan equities:
// parameterized classification tree, cycle compass with per-industry thresholds,
// seasonal pattern engine, risk aggregation, supply-chain linkage graph,
// and an event calendar with automated lunar-to-solar date computation.
//
// Key types:
//
//   - ClassificationTree: hierarchical L1/L2 industry taxonomy loaded from
//     ParametersConfig (parameterized in PR-1, replacing 600+ lines of
//     hard-coded tree construction).
//
//   - CycleTracker: detects business-cycle phase (expansion/peak/contraction/
//     trough) per industry using configurable CycleThresholdConfig (PR-2).
//
//   - SeasonalEngine: month-of-year and day-of-year pattern detection with
//     corrected range-boundary logic (PR-2).
//
//   - RiskAssessment / GetAllRisksForIndustry: industry-level risk scoring;
//     "ALL" aggregates representative stocks across all L1 industries (PR-2).
//
//   - SupplyChainGraph / DefaultCorrelationMatrix: upstream/downstream linkage
//     and configurable cross-industry correlation matrix (PR-2).
//
//   - EventCalendar: Taiwan market calendar (dividend seasons, MSCI rebalances,
//     elections, etc.). Lunar holidays (Spring Festival, Dragon Boat, Mid-Autumn,
//     Qingming) are computed automatically via lunar-go for any year, removing
//     the previous 2023-2030 hard-coded lookup ceiling (PR-4, ST-8).
//
// Maturity: stable
package industry
