// Package capitalflow provides a unified capital flow analysis engine for TAIEX.
//
// Core investment logic:
//
//	"全球資金流向決定方向，資金勢力共鳴決定強度，事件驅動資金流決定節奏"
//
// Forces tracked:
//   - Foreign: spot net buy/sell, futures open interest, TSM ADR premium
//   - Institutional: buy/sell net, decomposed into active vs passive (ETF)
//   - Government: branch-level identification from TWSE data (heuristic fallback)
//   - Retail: margin balance changes, day-trading ratio
//   - Dealer: proprietary + hedging net
//
// Output:
//   - Z-score per force (60-day rolling window)
//   - Resonance strength (1.5 when co-directional, 0.5 when adversarial)
//   - Capital quality score (foreign + institutional + dealer – retail)
//
// API endpoints:
//   - GET /api/capital-flow/daily — daily capital flow report
//   - GET /api/capital-flow/summary — capital flow summary
//
// Maturity: evolving
package capitalflow
