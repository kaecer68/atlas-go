// Package stockpicker provides the stock-picking subsystem core:
// three-level win-rate math (signal / stock / strategy) plus the storage
// layer for per-symbol signal outcomes and win-rate aggregates.
//
// winrate.go holds the pure math (sample win rate, Wilson score interval,
// calibration status, net-of-cost hit determination) with no I/O.
// signal_outcome_store.go and win_rate_store.go add SQLite-backed storage
// that reuses the internal/ledger/ SQLite pattern.
//
// Design documents:
//   - tasks/stock-picking-redesign-20260827-deepseek.md (§A k3 修正項)
//   - tasks/stock-picking-redesign-k3-review-20260827.md (§4 PR 1a 範圍)
//   - tasks/stock-picking-next-steps-k3-20260827.md (§2 A: PR 1b 範圍)
//
// Maturity: experimental
package stockpicker
