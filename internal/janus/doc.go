// Package janus implements the JANUS meta-layer for cross-cohort regime
// detection and dynamic weighting of PRISM training cohorts.
//
// JANUS sits above prism as a meta-layer: it observes the rolling performance
// of all PRISM cohorts across short/medium/long windows and dynamically
// re-weights them based on emergent regime classification.
//
// Core types:
//
//	RegimeClassification  — NOVEL_REGIME / HISTORICAL_REGIME / MIXED
//	PerformanceWindow     — short (~5D) / medium (~20D) / long (~60D)
//	CohortSnapshot         — Single performance observation per cohort
//	CohortPerformance      — Rolling-window aggregates per cohort
//	CohortWeight           — Computed JANUS weight per cohort
//	JANUSConfig            — Tunable parameters (DefaultJANUSConfig() sensible defaults)
//
// Regime classification logic (short vs long window divergence):
//   - NOVEL: short-window best cohort exceeds long-window best by NovelThreshold (default 0.15)
//   - HISTORICAL: long-window best exceeds short-window best by HistoricalThreshold (default 0.15)
//   - MIXED: in between
//
// Weight bounds: MinWeight=0.05 (floor, prevents total elimination), MaxWeight=0.60
// (ceiling). Cohorts with negative Sharpe get EpsilonWeight=0.02 when others
// are positive.
//
// Maturity: stable
package janus
