// Package strategy provides dynamic strategy selection and risk-parity allocation.
//
// Core components:
//
//	Registry          — Built-in strategies: all_weather, growth, value, defensive, momentum
//	Selector          — Regime-aware strategy picker with cooling-off period
//	ComparisonEngine  — Per-strategy Sharpe / drawdown / win-rate tracking
//	StrategyAllocator — Risk-parity weighting (inverse volatility) with [5%, 50%] bounds
//
// Selector behavior:
//   - shouldSwitch() enforces MinSwitchInterval to prevent churn
//   - No regime match → falls back to all_weather; missing all_weather → "fallback"
//   - ComparisonEngine scoring: Sharpe*0.4 + DailyReturn*30*0.3 + WinRate*0.3
//     (returns 0.5 when sample history < configured days)
//   - estimateVolatility() returns 0.20 (annualized default) when < 5 samples
//
// Maturity: evolving
package strategy
