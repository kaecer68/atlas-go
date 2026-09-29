// Package recommender provides tier-based investment recommendations.
//
// Three tiers of content — the caller's access tier is decided by the **verified
// C-02 JWT claims** (internal/subscription), never by the local users table:
//   - free (public): market regime light, capital flow summary, event reminders
//   - basic (= legacy `registered`): strategy rankings, industry flow, stock event alerts
//   - pro (= legacy `premium`): full strategy signals with entry/exit, deep backtest reports, MCP full access
//
// Both vocabulary generations are accepted: go-member claims are mapped to
// basic/pro by subscription.mapMemberTier, while legacy self-signed HS256 tokens
// carry registered/premium. Same access level ⇒ same content (see tier_source.go).
//
// This module uses the ranking engine from internal/strategy_ranker/
// and the event-driven system from internal/eventdriven/.
//
// Maturity: evolving
package recommender
