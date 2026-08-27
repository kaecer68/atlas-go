// Package stockpicker provides the pure-math core of the per-stock
// stock-picking subsystem (PR 1a of the stock-picking redesign).
//
// It holds the three-level win-rate math (signal / stock / strategy)
// shared across future layers: sample win rate, Wilson score interval,
// calibration status and net-of-cost hit determination. The package is
// intentionally pure (no I/O, no global state, stdlib only) so the math
// can be tested exhaustively before any storage or integration lands
// (storage reuses the internal/ledger/ SQLite pattern in later PRs).
//
// Design documents:
//   - tasks/stock-picking-redesign-20260827-deepseek.md (§A k3 修正項)
//   - tasks/stock-picking-redesign-k3-review-20260827.md (§4 PR 1a 範圍)
//
// Maturity: experimental
package stockpicker
