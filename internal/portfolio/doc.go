// Package portfolio provides portfolio intelligence: Darwinian weight management,
// multi-factor scoring, and position sizing optimization.
//
// Key components:
//
//	DarwinianWeightManager    — Performance-based agent weight adjustment [0.3, 2.5]
//	FactorEngine              — Multi-factor scoring (momentum, value, quality, agent)
//	Optimizer                 — Portfolio constraint optimization
//	VolatilityManager         — GARCH-based volatility forecasting
//
// Darwinian weights adjust agent influence based on rolling performance:
//   - Top 25% agents: +5% multiplier
//   - Bottom 25% agents: -5% multiplier
//   - Weights clamped to [0.3, 2.5] silently
//
// FactorEngine computes per-symbol scores using:
//   - Momentum: price trend, volume confirmation
//   - Value: P/E, P/B, dividend yield
//   - Quality: earnings stability, ROE consistency
//   - Agent: conviction-weighted agent consensus
package portfolio
