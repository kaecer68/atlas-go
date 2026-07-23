// Package recommender provides tier-based investment recommendations.
//
// Three tiers of content:
//   - free (public): market regime light, capital flow summary, event reminders
//   - registered: strategy rankings, industry flow, stock event alerts
//   - premium: full strategy signals with entry/exit, deep backtest reports, MCP full access
//
// This module uses the ranking engine from internal/strategy_ranker/
// and the event-driven system from internal/eventdriven/.
//
// Maturity: evolving
package recommender
