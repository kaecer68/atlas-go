// Package prism provides regime-specific training queues for the 5 regime
// types used by atlas-go's simulation engine.
//
// PRISM is the layer below JANUS: it maintains separate training cohorts per
// regime, while JANUS observes cohort performance to detect regime emergence
// and re-weight cohorts dynamically.
//
// Architecture:
//
//	prism (cohort training queues)
//	  ↓ observed by
//	janus (meta-layer, regime detection + dynamic re-weighting)
//	  ↓ consumed by
//	simulation (regime-specific replay)
//
// The 5 regime types align with internal/realtime.RegimeType
// (calm, volatile, trending_up, trending_down, reversal). Each cohort
// accumulates training samples (MarketEvent + Outcome) for replay when the
// orchestrator detects the matching regime.
//
// Synthetic flag (added in PR #776): cohorts can be marked synthetic when
// generated from baseline policy rather than real market data; this prevents
// contamination of historical validation with synthetic samples.
//
// Maturity: evolving
package prism
