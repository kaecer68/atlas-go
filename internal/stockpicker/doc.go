// Package stockpicker provides the stock-picking subsystem core:
// three-level win-rate math (signal / stock / strategy), the storage layer
// for per-symbol signal outcomes and win-rate aggregates, and the PR 1c
// backtest aggregation job that feeds historical point-in-time data
// (price bars, T86 institutional flows) through the win-rate formulas.
//
// winrate.go holds the pure math (sample win rate, Wilson score interval,
// calibration status, net-of-cost hit determination) with no I/O.
// signal_outcome_store.go and win_rate_store.go add SQLite/PostgreSQL-backed
// storage that reuses the internal/ledger/ SQLite pattern.
// backtest.go replays a hardcoded demo condition set over a PanelSource with
// strict point-in-time guarantees (P0-1: no lookahead, no fundamentals).
// aggregate.go groups outcomes into per-(symbol, source) win-rate summaries.
// cmd/run-stockpicker-backtest runs the end-to-end job after market close.
//
// Design documents:
//   - tasks/stock-picking-redesign-20260827-deepseek.md (§A k3 修正項)
//   - tasks/stock-picking-redesign-k3-review-20260827.md (§4 PR 1a 範圍)
//   - tasks/stock-picking-next-steps-k3-20260827.md (§2 B: PR 1c 範圍)
//
// Maturity: experimental
package stockpicker
