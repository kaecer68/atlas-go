// Package strategy_techniques provides the investment techniques library —
// StrategyFrame-based rule engine for Taiwan stock market, built on a
// 5-layer framework (L1 Global Liquidity, L2 Foreign Capital Behavior,
// L3 Industry Catalysts, L4 FX & Chips, L5 Geopolitics) and 4 core leading
// indicators (foreign capital net, TSM ADR, NVDA, DXY).
//
// Self-correction: hybrid attribution (rule-based classifier + LLM annotation)
// to explain WHY a technique degraded, with regime labels and multi-timescale
// rolling validation.
//
// Seeds: data/seeds/strategy_techniques.json (9 production frames)
//
// Maturity: stable
package strategy_techniques
