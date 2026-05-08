// Package sim provides the portfolio and execution simulation engine.
// It takes recommendations from the orchestrator, applies position sizing
// constraints, simulates order execution with slippage models, and produces
// position mutations and session outcomes.
//
// Key components:
//
//	Engine              — Main simulation loop: RunSymbol, ApplyRecommendations
//	SlippageModel       — Dynamic slippage based on volatility and volume
//	DynamicThreshold    — Adaptive conviction thresholds per regime
//
// The engine is deterministic: same input (quotes + recommendations + policy)
// always produces the same output, making it suitable for backtesting and
// experiment evaluation.
package sim
