// Package charter provides per-run charter (investment methodology) options
// for A/B experiments: stepwise switches (PeriodOnly, StrategyFilter, MacroFlow,
// CashReserve, ConvictionFloor), period-based conviction floors, and paired
// statistics (paired t-test, BCa bootstrap) for comparing backtest arms.
//
// It is the experimental harness behind Phase C3 charter stepwise A/B; the
// production wiring lives in internal/orchestrator (CharterMode) and the
// methodology advisor in internal/methodology.
//
// Maturity: experimental
package charter
