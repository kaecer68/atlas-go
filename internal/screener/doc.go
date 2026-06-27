// Package screener provides declarative stock screening with composable
// filter primitives evaluated against domain.Quote snapshots.
//
// Built-in filter primitives:
//   - P/E ratio threshold (price-to-earnings)
//   - P/B ratio threshold (price-to-book)
//   - Dividend yield threshold (annualized)
//   - Momentum (price trend over configurable window)
//   - Volume (average daily volume threshold)
//
// The orchestrator loads screening rules from configs/agents.json's
// screening_criteria section. Rules are evaluated against the current
// universe before recommendation phase; symbols failing any filter are
// silently dropped (no Recommendations emitted for them).
//
// When adding a new filter primitive, also extend the FilterSpec schema
// in configs/agents.json validation; orchestrator silently drops symbols
// when primitive names don't match.
//
// Maturity: evolving
package screener
