// Package charter provides per-run charter (investment methodology) options
// for A/B experiments: stepwise switches (PeriodOnly, StrategyFilter, MacroFlow,
// CashReserve, ConvictionFloor), period-based conviction floors, and paired
// statistics (paired t-test, BCa bootstrap) for comparing backtest arms.
//
// It is the experimental harness behind Phase C3 charter stepwise A/B; the
// production wiring lives in internal/orchestrator (CharterMode) and the
// methodology advisor in internal/methodology. Phase C4 adds the delta
// conversion (delta.go): each step report becomes a CharterDelta with a
// verdict + evidence for baseline policy writeback (internal/baseline).
//
// Maturity: experimental
package charter
