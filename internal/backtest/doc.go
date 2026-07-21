// Package backtest provides window-based backtesting with rolling-window
// cross-validation.
//
// Core entry points:
//
//	Runner.Run             — Single backtest run over a replay window
//	RollingWindowSplit     — IS/OOS split with configurable stride
//	BacktestPipeline       — Multi-window orchestration
//
// The replay window comes from data/replay/ (JSONL format, one event per
// line). RollingWindowSplit divides the full replay into IS (in-sample,
// first 2/3) and OOS (out-of-sample, last 1/3) by time, with a configurable
// stride for cross-fold validation.
//
// Results feed into ledger.BuildScorecards() for Sharpe / OOS ratio
// computation. See docs/specs/backtest-pipeline-spec.md for the canonical flow.
//
// Maturity: evolving
package backtest
