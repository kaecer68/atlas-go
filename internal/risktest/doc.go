// Package risktest provides reusable stress-test scenario logic for risk
// management modules. It is used by cmd/stress-test for batch scenario runs.
//
// Scenario types:
//
//	MarketCrash          — Sharp single-day drawdown (e.g. -10%)
//	SustainedDrawdown    — Multi-week gradual drawdown (e.g. -20% over 30D)
//	LiquidityCrisis      — Wide bid-ask spreads + low volume
//	VolatilitySpike      — 2-3x normal intraday range
//	CorrelationBreakdown — All correlations → 1.0 (everything moves together)
//
// Each scenario defines:
//   - The market state perturbation (price shock, volume change, etc.)
//   - The expected portfolio response (drawdown, volatility)
//   - The risk management decision path (monitor / reduce / halt)
//
// Scenarios are pure functions (deterministic input → output) for
// reproducible testing. cmd/stress-test orchestrates the execution and
// produces comparison reports across scenarios.
//
// Maturity: utility
package risktest
