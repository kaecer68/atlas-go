// Package stockpicker provides the stock-picking subsystem core:
// three-level win-rate math (signal / stock / strategy), the storage layer
// for per-symbol signal outcomes and win-rate aggregates, and the backtest
// aggregation job that feeds historical point-in-time data (price bars, T86
// institutional flows) through the win-rate formulas.
//
// winrate.go holds the pure math (sample win rate, Wilson score interval,
// calibration status, net-of-cost hit determination) with no I/O.
// signal_outcome_store.go and win_rate_store.go add SQLite/PostgreSQL-backed
// storage that reuses the internal/ledger/ SQLite pattern.
// conditions.go (PR 2a) is the configurable condition engine: an ordered
// registry of point-in-time conditions whose parameters (window, threshold)
// come from configs/parameters.json — the PR 1c demo conditions are the
// registry defaults; fundamentals conditions stay live_observe_only (P0-1).
// backtest.go replays a condition set over a PanelSource with strict
// point-in-time guarantees (P0-1: no lookahead, no fundamentals).
// aggregate.go groups outcomes into per-(symbol, source) win-rate summaries.
// cmd/run-stockpicker-backtest runs the end-to-end job after market close
// (-conditions selects which conditions run; -list-conditions lists them).
//
// Design documents:
//   - tasks/stock-picking-redesign-20260827-deepseek.md (§A k3 修正項)
//   - tasks/stock-picking-redesign-k3-review-20260827.md (§4 PR 1a 範圍)
//   - tasks/stock-picking-next-steps-k3-20260827.md (§2 B: PR 1c 範圍)
//
// Maturity: experimental
package stockpicker
